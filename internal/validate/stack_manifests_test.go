package validate

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// declared runs the readers over a temp root holding one manifest and returns the
// dependency names it found, sorted. Asserting on the reader rather than on Stack's
// findings keeps these tests about the one thing that can be wrong here: what counts
// as a declared dependency.
func declared(t *testing.T, name, content string) []string {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, name), content)
	deps, err := dependencies(root)
	if err != nil {
		t.Fatalf("dependencies: %v", err)
	}
	var got []string
	for dep, from := range deps {
		if from != name {
			t.Errorf("%s was attributed to %q, want %q", dep, from, name)
		}
		got = append(got, dep)
	}
	sort.Strings(got)
	return got
}

func want(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("dependencies = %v, want %v", got, want)
	}
}

// A requirements.txt carries more non-requirements than requirements: options,
// comments, markers, extras, and direct URLs. Every one of them has to fall out, or the
// validator reports a finding about "-r" and is never trusted again.
func TestRequirementsTxt(t *testing.T) {
	got := declared(t, "requirements.txt", `# production
requests>=2.31.0
Flask-SQLAlchemy==3.1.1
uvicorn[standard]~=0.30      # extras are not part of the name
pytest ; python_version >= "3.9"
django \
    >= 5.0
-r dev-requirements.txt
-e .
--index-url https://example.invalid/simple
https://example.invalid/pkg.tar.gz
mypkg @ git+https://example.invalid/mypkg.git@v1
`)
	want(t, got, "Flask-SQLAlchemy", "django", "mypkg", "pytest", "requests", "uvicorn")
}

// PEP 621, PEP 735 and Poetry are three ways of saying the same thing, and a project
// uses whichever its tooling chose. Reading only one of them would leave a whole
// dependency list unchecked while reporting nothing, which is the worst shape of miss.
func TestPyprojectPEP621(t *testing.T) {
	got := declared(t, "pyproject.toml", `[build-system]
requires = ["hatchling"]

[project]
name = "app"
dependencies = [
    "httpx>=0.27",     # multi-line array
    "pydantic[email]>=2",
]

[project.optional-dependencies]
dev = ["pytest", "ruff"]

[dependency-groups]
docs = ["mkdocs"]
`)
	// hatchling is under [build-system], which builds the package rather than being a
	// dependency of it — and reading it would put every build backend in stack.md.
	want(t, got, "httpx", "mkdocs", "pydantic", "pytest", "ruff")
}

func TestPyprojectPoetry(t *testing.T) {
	got := declared(t, "pyproject.toml", `[tool.poetry]
name = "app"

[tool.poetry.dependencies]
python = "^3.12"
requests = "^2.31"
sqlalchemy = { version = "^2.0", extras = ["asyncio"] }

[tool.poetry.group.dev.dependencies]
pytest = "^8.0"

[tool.poetry.scripts]
app = "app.main:run"
`)
	// python constrains the interpreter and [tool.poetry.scripts] declares an entry
	// point; neither is a package anybody adopted.
	want(t, got, "pytest", "requests", "sqlalchemy")
}

func TestCargoToml(t *testing.T) {
	got := declared(t, "Cargo.toml", `[package]
name = "app"

[dependencies]
serde = { version = "1", features = ["derive"] }
tokio = "1"

[dev-dependencies]
criterion = "0.5"

[build-dependencies]
cc = "1"

[target.'cfg(unix)'.dependencies]
nix = "0.29"

[dependencies.reqwest]
version = "0.12"
features = ["json"]
`)
	want(t, got, "cc", "criterion", "nix", "reqwest", "serde", "tokio")
}

// The long form's sub-tables name the crate once, in the header. Reading their keys as
// crates would turn every feature list into a dependency.
func TestCargoLongFormSubTableIsNotACrate(t *testing.T) {
	got := declared(t, "Cargo.toml", `[dependencies.serde]
version = "1"

[dependencies.serde.features]
derive = true
`)
	want(t, got, "serde")
}

func TestComposerJSON(t *testing.T) {
	got := declared(t, "composer.json", `{
  "require": {
    "php": ">=8.2",
    "ext-json": "*",
    "monolog/monolog": "^3.0"
  },
  "require-dev": {"phpunit/phpunit": "^11.0"}
}`)
	// The interpreter and its extensions are what the code runs on, not decisions to
	// record — a finding demanding a stack.md entry for ext-json is noise.
	want(t, got, "monolog/monolog", "phpunit/phpunit")
}

// dependencyManagement pins versions for modules that may never depend on the artifact,
// and a profile's dependencies apply only when that profile is active. Reporting either
// would demand stack.md entries for technology the build does not necessarily use.
func TestPomXMLReadsOnlyTheDirectDependencies(t *testing.T) {
	got := declared(t, "pom.xml", `<project>
  <dependencyManagement>
    <dependencies>
      <dependency>
        <groupId>com.fasterxml.jackson</groupId>
        <artifactId>jackson-bom</artifactId>
      </dependency>
    </dependencies>
  </dependencyManagement>
  <dependencies>
    <dependency>
      <groupId>org.springframework</groupId>
      <artifactId>spring-core</artifactId>
    </dependency>
    <dependency>
      <groupId>org.junit.jupiter</groupId>
      <artifactId>junit-jupiter</artifactId>
      <scope>test</scope>
    </dependency>
    <dependency>
      <groupId>com.example</groupId>
      <artifactId>${unresolved.artifact}</artifactId>
    </dependency>
  </dependencies>
  <profiles>
    <profile>
      <dependencies>
        <dependency>
          <groupId>com.example</groupId>
          <artifactId>only-under-a-profile</artifactId>
        </dependency>
      </dependencies>
    </profile>
  </profiles>
</project>
`)
	// A test-scoped dependency stays: it is as much an adopted decision, and as much of
	// a supply-chain surface, as anything that ships.
	want(t, got, "junit-jupiter", "spring-core")
}

// Rule 3 of the readers, one case per shape: a file scc cannot read produces no
// dependencies rather than guessed ones.
func TestManifestsThatCannotBeReadProduceNothing(t *testing.T) {
	cases := map[string]string{
		"composer.json":  "{ not json",
		"pom.xml":        "<project><dependencies>",
		"pyproject.toml": "[project]\ndependencies = [\n  \"httpx\",\n", // array never closes
	}
	for name, content := range cases {
		if got := declared(t, name, content); len(got) != 0 {
			t.Errorf("%s: dependencies = %v, want none", name, got)
		}
	}
}

// The whole reason a language without a reader is safe: nothing is declared, so Stack
// returns before it ever asks whether docs/stack.md exists. A Ruby, Elixir or Swift
// project must pass `scc validate` untouched, not fail on a manifest scc never
// understood.
func TestAnEcosystemWithNoReaderIsNotChecked(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "Gemfile"), "source 'https://rubygems.org'\ngem 'rails', '~> 7.1'\n")
	write(t, filepath.Join(root, "mix.exs"), "defp deps do\n  [{:phoenix, \"~> 1.7\"}]\nend\n")
	write(t, filepath.Join(root, "build.gradle"), "dependencies { implementation 'com.google.guava:guava:33.0.0' }\n")

	if got := runValidator(t, Stack, root); len(got) != 0 {
		t.Errorf("rules = %v, want silence for an ecosystem with no reader", got)
	}
}

// A repo can hold two ecosystems, and each dependency has to be reported against the
// file that declared it — "requests is declared in package.json" would send the reader
// to the wrong file.
func TestSeveralManifestsInOneRepo(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "requirements.txt"), "requests\n")
	write(t, filepath.Join(root, "package.json"), `{"dependencies": {"react": "^18"}}`)
	deps, err := dependencies(root)
	if err != nil {
		t.Fatalf("dependencies: %v", err)
	}
	if deps["requests"] != "requirements.txt" || deps["react"] != "package.json" {
		t.Errorf("dependencies = %v, want each attributed to its own manifest", deps)
	}
}

// PyPI treats these as one project name, so a glossary of spellings is not the user's
// problem to solve before the validator will believe them.
func TestStackAcceptsEitherSeparatorSpelling(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "requirements.txt"), "Flask-SQLAlchemy==3.1.1\n")
	write(t, filepath.Join(root, "docs", "stack.md"), "# Stack\n\n- flask_sqlalchemy — the ORM binding\n")
	if got := runValidator(t, Stack, root); len(got) != 0 {
		t.Errorf("rules = %v, want the underscore spelling to count as documented", got)
	}
}

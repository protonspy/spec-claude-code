package jail

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A fake install tree: a home the sandbox would hide, and the two shapes a tool
// arrives in — a compiled binary, and a script symlinked onto PATH out of a
// package directory.
type tree struct {
	home string
}

func newTree(t *testing.T) tree {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return tree{home: home}
}

func (tr tree) write(t *testing.T, rel, content string) string {
	t.Helper()
	path := filepath.Join(tr.home, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func specs(maps []Mapping) []string {
	out := make([]string, 0, len(maps))
	for _, m := range maps {
		out = append(out, m.Spec())
	}
	return out
}

// The cargo-installed case, and the one that started this: rtk is a compiled
// binary sitting in a directory the private home takes away. One mount, itself,
// at the path PATH already knows — nothing about ~/.cargo/bin comes with it.
func TestNeedsMountsACompiledBinaryAndNothingElse(t *testing.T) {
	tr := newTree(t)
	bin := tr.write(t, ".cargo/bin/rtk", "\x7fELF fake")

	got := Needs(tr.home, bin, "")
	if len(got) != 1 || got[0].Spec() != bin {
		t.Errorf("Needs = %v, want just %s", specs(got), bin)
	}
}

// A tool outside the hidden region is already there. Mapping it would be scc
// asking the sandbox for something it does not need, which is how a launcher ends
// up owning somebody's policy.
func TestNeedsSkipsWhatTheSandboxKeeps(t *testing.T) {
	tr := newTree(t)
	other, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(other, "rtk")
	if err := os.WriteFile(bin, []byte("\x7fELF fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := Needs(tr.home, bin, ""); got != nil {
		t.Errorf("Needs = %v, want none for a path outside the hidden root", specs(got))
	}
}

// The npm distribution: `scc` on PATH is a node shim, and the binary that answers
// for it is the running process. Mapping the real one at the shim's name replaces
// a launcher with the thing it launches, so node never enters the sandbox at all.
func TestNeedsShowsTheRealBinaryAtTheNamePathKnows(t *testing.T) {
	tr := newTree(t)
	real := tr.write(t, ".npm/lib/node_modules/@protonspy/scc-linux-x64/bin/scc", "\x7fELF fake")
	entry := filepath.Join(tr.home, ".npm", "bin", "scc")

	got := Needs(tr.home, entry, real)
	if len(got) != 1 || got[0].Spec() != real+":"+entry {
		t.Errorf("Needs = %v, want %s", specs(got), real+":"+entry)
	}
}

// A script is the case where mounting the file alone starts it and then watches
// it fail: node resolves the real path of what it runs before looking for
// anything beside it, so the package tree has to be there too, and the
// interpreter has to be findable.
func TestNeedsCarriesAScriptsPackageAndInterpreter(t *testing.T) {
	tr := newTree(t)
	script := tr.write(t, "n/lib/node_modules/codegraph/bin/cli.js", "#!/usr/bin/env fakenode\nconsole.log(1)\n")
	node := tr.write(t, "n/bin/fakenode", "\x7fELF fake")
	// The real npm shape is a symlink from the bin directory into the package
	// tree. It is spelled as a resolved pair here rather than an os.Symlink,
	// because the two reach needs identically and creating one needs a privilege
	// the Windows CI job does not have.
	entry := filepath.Join(tr.home, "n", "bin", "codegraph")
	t.Setenv("PATH", filepath.Dir(node))

	got := specs(Needs(tr.home, entry, script))
	want := []string{
		script + ":" + entry, // runnable under the name PATH knows
		filepath.Dir(entry),  // the bin directory, symlinks intact
		filepath.Join(tr.home, "n", "lib", "node_modules"), // sibling packages resolve
	}
	for _, w := range want {
		if !contains(got, w) {
			t.Errorf("Needs = %v, missing %s", got, w)
		}
	}
	// The interpreter is a neighbour here, so the bin directory already covers it;
	// what matters is that the walk found it rather than stopping at the script.
	if runtime.GOOS != "windows" && !contains(got, filepath.Dir(node)) {
		t.Errorf("Needs = %v, does not reach the interpreter", got)
	}
}

func contains(all []string, want string) bool {
	for _, a := range all {
		if a == want {
			return true
		}
	}
	return false
}

// Two tools installed the same way name the same package root and the same
// interpreter. The command line says it once.
func TestComposeDropsRepeatsAndWhatADirectoryCovers(t *testing.T) {
	tr := newTree(t)
	dir := filepath.Join(tr.home, "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	inside := tr.write(t, "bin/rtk", "\x7fELF fake")

	got := specs(Compose([]Mapping{
		{Src: dir},
		{Src: inside},
		{Src: dir},
		{Src: inside, Dest: filepath.Join(tr.home, "other", "rtk")},
	}))
	want := []string{dir, inside + ":" + filepath.Join(tr.home, "other", "rtk")}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("Compose = %v, want %v", got, want)
	}
}

// Mounts apply in order, so a directory arriving after a file underneath it hides
// the file. Measured inside a real sandbox: an npm-installed scc mapped as "the
// real binary, at the shim's name" came back as the shim, because the bin
// directory was mounted on top of it a moment later. Directories first.
func TestComposePutsDirectoriesBeforeTheFilesInsideThem(t *testing.T) {
	tr := newTree(t)
	real := tr.write(t, "tools/scc", "\x7fELF fake")
	binDir := filepath.Join(tr.home, "n", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(binDir, "scc")

	// The order Needs produces: the specific mount first, the directory it lands
	// inside second.
	got := specs(Compose([]Mapping{
		{Src: real, Dest: shim},
		{Src: binDir},
	}))
	want := []string{binDir, real + ":" + shim}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("Compose = %v, want %v — the file mount has to land on top", got, want)
	}
}

// A build that does not advertise --map gets no substitute, the same answer
// Needed gives for the other two: a mount spelled by guesswork is a sandbox
// opened by a tool that was only trying to help.
func TestMapArgsRefusesToGuessTheSpelling(t *testing.T) {
	maps := []Mapping{{Src: "/home/u/.cargo/bin/rtk"}}

	args, ok := MapArgs(HelpFlags("  --map <PATH|SOURCE:DEST>  read-only mount\n"), maps)
	if !ok || strings.Join(args, " ") != "--map /home/u/.cargo/bin/rtk" {
		t.Errorf("MapArgs = %v (ok=%v)", args, ok)
	}

	if args, ok := MapArgs(HelpFlags("  --network  enable network\n"), maps); ok || args != nil {
		t.Errorf("MapArgs = %v (ok=%v), want nothing from a build with no %s", args, ok, FlagMap)
	}

	// Nothing to ask for is not a failure to ask.
	if _, ok := MapArgs(HelpFlags(""), nil); !ok {
		t.Error("MapArgs reported a failure for an empty toolchain")
	}
}

func TestHiddenIsTheRegionTheBackendTakesAway(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "home", "u")
	cases := []struct {
		root, path string
		want       bool
	}{
		{home, filepath.Join(home, ".cargo", "bin", "rtk"), true},
		{home, home, true},
		{home, filepath.Join(string(filepath.Separator), "usr", "bin", "git"), false},
		{home, filepath.Join(string(filepath.Separator), "home", "user2", "rtk"), false},
		// macOS hides everything outside the project, so its root is "/" and any
		// absolute path is inside it. Built from a real absolute path, since what
		// counts as one is a platform question.
		{string(filepath.Separator), filepath.Join(os.TempDir(), "git"), true},
		{"", filepath.Join(home, "rtk"), false},
	}
	for _, c := range cases {
		if got := Hidden(c.root, c.path); got != c.want {
			t.Errorf("Hidden(%q, %q) = %v, want %v", c.root, c.path, got, c.want)
		}
	}
}

func TestSpecPrefersThePathAToolAlreadyHas(t *testing.T) {
	if got := (Mapping{Src: "/a/rtk"}).Spec(); got != "/a/rtk" {
		t.Errorf("Spec = %q", got)
	}
	if got := (Mapping{Src: "/a/rtk", Dest: "/a/rtk"}).Spec(); got != "/a/rtk" {
		t.Errorf("Spec = %q, want no self-referential destination", got)
	}
	if got := (Mapping{Src: "/a/rtk", Dest: "/b/rtk"}).Spec(); got != "/a/rtk:/b/rtk" {
		t.Errorf("Spec = %q", got)
	}
}

func TestShebangNamesWhatPathHasToFind(t *testing.T) {
	tr := newTree(t)
	cases := []struct {
		name, content, want string
		script              bool
	}{
		{"env.js", "#!/usr/bin/env node\n", "node", true},
		{"envopts.js", "#!/usr/bin/env -S node --enable-source-maps\n", "node", true},
		{"envassign.js", "#!/usr/bin/env NODE_ENV=production node\n", "node", true},
		{"direct.sh", "#!/bin/sh\necho hi\n", "/bin/sh", true},
		{"binary", "\x7fELF fake", "", false},
	}
	for _, c := range cases {
		path := tr.write(t, c.name, c.content)
		got, script := shebang(path)
		if got != c.want || script != c.script {
			t.Errorf("shebang(%s) = %q, %v; want %q, %v", c.name, got, script, c.want, c.script)
		}
	}
}

// A colon is the map syntax's own separator, so a path carrying one cannot be
// asked for — and what ai-jail would parse instead is a mount of something else.
// The drive letter is discounted, since the suite runs on Windows and no sandbox
// does.
func TestSpellableRejectsWhatTheMapSyntaxWouldSplit(t *testing.T) {
	if spellable("/home/u/od:d/rtk") {
		t.Error("a path with a colon was called spellable")
	}
	if !spellable(filepath.Join(os.TempDir(), "rtk")) {
		t.Error("an ordinary absolute path was refused")
	}
}

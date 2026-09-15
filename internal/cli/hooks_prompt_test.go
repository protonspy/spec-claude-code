package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// promptWorkspace is a workspace with two artifacts to name.
func promptWorkspace(t *testing.T) string {
	t.Helper()
	root := initWorkspace(t)
	if err := os.MkdirAll(filepath.Join(root, "specs", "auth-flow"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	write(t, root, "specs/auth-flow/requirements.md", "# Requirements\n")
	write(t, root, "plans/migration.md", "# Migration\n\n## Tasks\n\n- [ ] 1.1 (Unit) start\n")
	return root
}

// TestThePromptStageIsSilentUnlessAnArtifactIsNamed is the property the whole
// stage rests on, and the reason it was safe to build at all.
//
// It fires on every prompt of every session — the worst cost profile of the three
// harness stages — so the only thing that makes it defensible is that it almost
// never speaks. ponytail can afford to re-assert on every prompt because its
// instruction is a disposition with no shape; scc's rules are routed by moment,
// so a stage that spoke unprompted would not be re-asserting anything, it would
// be interrupting.
//
// Every case here is a prompt a real session would send. If any of them starts
// drawing a line, the stage has become noise in every turn and the bound has
// quietly widened.
func TestThePromptStageIsSilentUnlessAnArtifactIsNamed(t *testing.T) {
	root := promptWorkspace(t)
	for _, prompt := range []string{
		"",
		"   ",
		"fix the failing test",
		"why is this returning nil?",
		"run the suite and tell me what broke",
		"add a flag to the launch command",
		// Names a *concept* the workspace happens to have an artifact about,
		// without naming the artifact. This is the case a looser matcher would
		// take, and taking it would fire on a large share of ordinary prompts.
		"how does auth work here?",
		"migrate the config format",
		// A requirement id names a requirement without naming a spec, and a real
		// workspace defines R1.2 in several of them. `scc map trace` answers that
		// question properly when it is actually asked.
		"what does R1.2 say?",
	} {
		if got := promptContext(root, prompt); len(got) != 0 {
			t.Errorf("prompt %q drew a line: %v", prompt, got)
		}
	}
}

// When the prompt does name an artifact, the stage says the one thing that is
// cheapest to say at that moment: how to read it without reading it.
func TestThePromptStageRoutesANamedArtifactThroughTheMap(t *testing.T) {
	root := promptWorkspace(t)
	for _, tc := range []struct {
		prompt string
		want   string
	}{
		{"start work on auth-flow", "specs/auth-flow"},
		{"what is left in the migration plan?", "plans/migration.md"},
		{"look at specs/auth-flow/requirements.md", "specs/auth-flow"},
	} {
		got := strings.Join(promptContext(root, tc.prompt), "\n")
		if got == "" {
			t.Errorf("prompt %q named an artifact and drew nothing", tc.prompt)
			continue
		}
		if !strings.Contains(got, tc.want) {
			t.Errorf("prompt %q: want %s in %q", tc.prompt, tc.want, got)
		}
		if !strings.Contains(got, "map") {
			t.Errorf("prompt %q names the artifact but not how to read it cheaply: %q", tc.prompt, got)
		}
	}
}

// TestAShortArtifactNameIsOnlyMatchedByItsPath is the guard on the failure that
// would make this stage worthless.
//
// A spec called `ui` matched in prose fires on "build the ui", on "guide", and on
// most of an ordinary session. One wrong line teaches the reader to skip every
// line, so a name below the floor is reachable only by its path — which nobody
// types by accident.
func TestAShortArtifactNameIsOnlyMatchedByItsPath(t *testing.T) {
	root := initWorkspace(t)
	if err := os.MkdirAll(filepath.Join(root, "specs", "ui"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	write(t, root, "specs/ui/requirements.md", "# Requirements\n")

	for _, prompt := range []string{"build the ui", "add a guide", "the ui is broken"} {
		if got := promptContext(root, prompt); len(got) != 0 {
			t.Errorf("a two-letter spec name fired on %q: %v", prompt, got)
		}
	}
	if got := promptContext(root, "open specs/ui/requirements.md"); len(got) == 0 {
		t.Error("the path form did not reach a short-named spec, so it is unreachable")
	}
}

// A prefix of a longer name is not that name. Treating `-` as a boundary would
// make `auth` match `auth-flow`, so naming one thing would cite another.
func TestAPrefixOfAnArtifactNameIsNotThatArtifact(t *testing.T) {
	root := promptWorkspace(t)
	if got := promptContext(root, "the auth module needs a fix"); len(got) != 0 {
		t.Errorf("`auth` matched the spec `auth-flow`: %v", got)
	}
}

// TestThePromptStageNeverBlocksATurn is the contract every harness stage holds,
// and the one this stage could most easily break.
//
// UserPromptSubmit is the only stage where a non-zero exit stops the request
// before the agent ever sees it — the user's prompt is simply discarded. A stage
// whose whole job is an optional hint must never be able to do that, whatever it
// is handed.
func TestThePromptStageNeverBlocksATurn(t *testing.T) {
	root := promptWorkspace(t)
	for _, stdin := range []string{
		`{"hook_event_name":"UserPromptSubmit","prompt":"work on auth-flow"}`,
		`{"hook_event_name":"UserPromptSubmit","prompt":"fix the test"}`,
		`{"prompt":""}`,
		`not json at all`,
		``,
	} {
		stdout, stderr, code := runStdin(t, stdin, "hooks", "run", "user-prompt", "--root", root)
		if code != ExitOK {
			t.Errorf("stdin %q: exit = %d, want %d — a non-zero exit here discards the user's prompt (stderr: %s)",
				stdin, code, ExitOK, stderr)
		}
		if strings.Contains(stdin, "auth-flow") {
			if ctx := additionalContext(t, stdout); !strings.Contains(ctx, "auth-flow") {
				t.Errorf("the routing never reached the agent: %q", ctx)
			}
			continue
		}
		if strings.TrimSpace(stdout) != "" {
			t.Errorf("stdin %q printed %q, want silence", stdin, stdout)
		}
	}
}

// A repository that is not an scc workspace has no artifacts to name, so the
// stage has nothing it could correctly say — and saying anything there would be
// telling a plain repository it is not a workspace, on every prompt.
func TestThePromptStageSaysNothingOutsideAWorkspace(t *testing.T) {
	if got := promptContext(t.TempDir(), "work on auth-flow"); len(got) != 0 {
		t.Errorf("a non-workspace drew a line: %v", got)
	}
}

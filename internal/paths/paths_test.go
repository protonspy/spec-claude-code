package paths

import (
	"path/filepath"
	"strings"
	"testing"
)

// Every path scc hands out has to land under the root it was given. The failure
// this guards is not a typo in a Join — it is a segment constant that grows a
// leading separator and silently turns a workspace-relative path into an absolute
// one, which no caller checks and every caller writes to.
func TestEveryPathStaysUnderTheRoot(t *testing.T) {
	root := filepath.Join("some", "workspace")

	got := map[string]string{
		"Specs":        Specs(root),
		"Spec":         Spec(root, "checkout"),
		"Requirements": Requirements(root, "checkout"),
		"Design":       Design(root, "checkout"),
		"Tasks":        Tasks(root, "checkout"),
		"Plans":        Plans(root),
		"Plan":         Plan(root, "revamp"),
		"Docs":         Docs(root),
		"Wiki":         Wiki(root),
		"WikiPages":    WikiPages(root),
		"ADR":          ADR(root),
		"Raw":          Raw(root),
		"Codewiki":     Codewiki(root),
		"Glossary":     Glossary(root),
		"Notes":        Notes(root),
		"Stack":        Stack(root),
	}
	for _, h := range Harnesses() {
		got[h.ID+".Config"] = h.Config(root)
		got[h.ID+".Agents"] = h.Agents(root)
		got[h.ID+".Skills"] = h.Skills(root)
		got[h.ID+".Rules"] = h.Rules(root)
		got[h.ID+".Rule"] = h.Rule(root, "delivery")
		got[h.ID+".Entry"] = h.Entry(root)
		got[h.ID+".Manifest"] = h.Manifest(root)
	}

	for name, p := range got {
		if !strings.HasPrefix(p, root+string(filepath.Separator)) {
			t.Errorf("%s = %q, which is not under %q", name, p, root)
		}
	}
}

// The named artifacts carry their own extension and segment, and a caller that
// joined the wrong one would write a file every validator then ignores.
func TestArtifactPathsCarryTheirOwnNames(t *testing.T) {
	root := "w"
	for name, want := range map[string]string{
		Requirements(root, "f"):       RequirementsSeg,
		Design(root, "f"):             DesignSeg,
		Tasks(root, "f"):              TasksSeg,
		Plan(root, "p"):               "p.md",
		Glossary(root):                GlossarySeg,
		Notes(root):                   NotesSeg,
		Stack(root):                   StackSeg,
		Claude.Rule(root, "delivery"): "delivery.md",
		Claude.Manifest(root):         ManifestSeg,
	} {
		if filepath.Base(name) != want {
			t.Errorf("%q ends in %q, want %q", name, filepath.Base(name), want)
		}
	}
}

// An empty segment is a capability this harness does not have, and the methods
// that carry one return "" rather than a path into the configuration directory
// itself — which is what a bare Join would produce, and what a caller would then
// write a settings file or a command over.
func TestAnAbsentCapabilityIsEmptyAndNotTheConfigDirectory(t *testing.T) {
	root := "w"
	for _, h := range Harnesses() {
		cmds, settings := h.Commands(root), h.Settings(root)
		if (h.CommandsSeg == "") != (cmds == "") {
			t.Errorf("%s: CommandsSeg %q but Commands() = %q", h.ID, h.CommandsSeg, cmds)
		}
		if (h.SettingsSeg == "") != (settings == "") {
			t.Errorf("%s: SettingsSeg %q but Settings() = %q", h.ID, h.SettingsSeg, settings)
		}
		if cmds != "" && cmds == h.Config(root) {
			t.Errorf("%s: Commands() resolved to the config directory itself", h.ID)
		}
		if settings != "" && settings == h.Config(root) {
			t.Errorf("%s: Settings() resolved to the config directory itself", h.ID)
		}
	}
}

func TestAgentExtFollowsTheFormat(t *testing.T) {
	for _, h := range Harnesses() {
		want := ".md"
		if h.AgentFormat == FormatTOML {
			want = ".toml"
		}
		if got := h.AgentExt(); got != want {
			t.Errorf("%s: AgentExt() = %q, want %q", h.ID, got, want)
		}
	}
}

func TestParseHarnessAcceptsEveryIDAndNothingElse(t *testing.T) {
	for _, want := range Harnesses() {
		got, err := ParseHarness(want.ID)
		if err != nil {
			t.Errorf("ParseHarness(%q): %v", want.ID, err)
			continue
		}
		if got.ID != want.ID {
			t.Errorf("ParseHarness(%q) returned %q", want.ID, got.ID)
		}
	}
	for _, id := range []string{"", "CLAUDE", "cursor", "claude "} {
		if _, err := ParseHarness(id); err == nil {
			t.Errorf("ParseHarness(%q) accepted an id scc does not support", id)
		} else if !strings.Contains(err.Error(), HarnessIDs()) {
			t.Errorf("ParseHarness(%q) failed without naming the choices: %v", id, err)
		}
	}
}

// The probe order is deliberate — it is the order workspace.Find stats the marker
// in — while HarnessIDs is for help text and sorts. Two different orders on
// purpose, and a test each so neither is "fixed" into the other.
func TestHarnessesProbeInOrderAndListAlphabetically(t *testing.T) {
	if got := Harnesses()[0].ID; got != Claude.ID {
		t.Errorf("the probe order starts at %q, not the default harness", got)
	}
	ids := strings.Split(HarnessIDs(), ", ")
	for i := 1; i < len(ids); i++ {
		if ids[i-1] > ids[i] {
			t.Errorf("HarnessIDs is not sorted: %q", HarnessIDs())
			break
		}
	}
	if len(ids) != len(Harnesses()) {
		t.Errorf("HarnessIDs lists %d of %d harnesses", len(ids), len(Harnesses()))
	}
}

// Skills are checked wherever they are: the Agent Skills format is one standard,
// and a workspace scaffolded for two harnesses has two copies of it to keep
// conforming. A validator searching one directory would pass a broken skill.
func TestSkillDirsCoversEveryHarness(t *testing.T) {
	dirs := SkillDirs("w")
	if len(dirs) != len(Harnesses()) {
		t.Fatalf("SkillDirs returned %d directories for %d harnesses", len(dirs), len(Harnesses()))
	}
	for i, h := range Harnesses() {
		if dirs[i] != h.Skills("w") {
			t.Errorf("SkillDirs[%d] = %q, want %q", i, dirs[i], h.Skills("w"))
		}
	}
}

// Each harness keeps its own directory, its own entry file and its own manifest.
// Two sharing either would make one workspace's scaffold overwrite the other's,
// and the manifest is also the marker, so a shared one would make the two
// indistinguishable to the workspace walk.
func TestHarnessesDoNotShareTheirOwnFiles(t *testing.T) {
	dirs, manifests := map[string]string{}, map[string]string{}
	for _, h := range Harnesses() {
		if prev, seen := dirs[h.Dir]; seen {
			t.Errorf("%s and %s share the directory %q", prev, h.ID, h.Dir)
		}
		dirs[h.Dir] = h.ID
		m := h.Manifest("w")
		if prev, seen := manifests[m]; seen {
			t.Errorf("%s and %s share the manifest %q", prev, h.ID, m)
		}
		manifests[m] = h.ID
		if h.ID == "" || h.Label == "" || h.Bin == "" || h.EntryFile == "" || h.RulesSeg == "" {
			t.Errorf("%+v is missing a field every harness needs", h)
		}
	}
}

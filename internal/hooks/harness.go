package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"strings"

	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/textutil"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// The harness hooks: the same idea as the git ones, one layer earlier.
//
// A git hook fires when work enters history. That is late for a finding — the
// commit is the second moment it exists, and the first is the end of the turn
// that wrote it, when the agent still has the file in mind and fixing it costs a
// sentence. Claude Code will run a command at that moment and read what it
// prints back into the next turn, so the check can arrive where it is cheapest to
// act on instead of where it is cheapest to enforce.
//
// **These never block.** A hook that can stop a turn can also loop one, and the
// agent is the reader here rather than the subject: findings come back as
// context, and what to do about them is the methodology's business. The git hooks
// are where refusal lives, because a commit is a decision with a natural place to
// stand in front of.
//
// **And they never publish.** The end of a turn is a plausible moment to `git
// push`, and scc does not: pushing from a hook sends work on an event nobody is
// watching. What it does instead is say the branch has commits that never left
// the machine, so the agent pushes in its next turn, in the open, where a person
// can still stop it.
type Event string

// The events scc registers, in the order a session meets them.
const (
	// SessionStart is where a workspace says what it is missing before any work
	// is done on it — one message, once, when it can still change the plan.
	SessionStart Event = "SessionStart"
	// Stop is the end of a turn, which is the earliest a finding can be reported
	// to the one who can fix it.
	Stop Event = "Stop"
)

// Events returns the set, in that order.
func Events() []Event { return []Event{SessionStart, Stop} }

// Stage is the name this event is called by on the command line: the hook entry
// runs `scc hooks run <stage>`, and the stage names stay kebab-case like git's.
func (e Event) Stage() Stage {
	switch e {
	case SessionStart:
		return StageSessionStart
	case Stop:
		return StageStop
	}
	return ""
}

// Why is the one line the report prints beside each event.
func (e Event) Why() string {
	switch e {
	case SessionStart:
		return "syncs the symbol graph and names what this workspace is missing, once"
	case Stop:
		return "syncs the graph, then reports findings and undelivered work to the agent"
	}
	return ""
}

// Timeout is how long the harness may wait, in seconds.
//
// Both stages sync the symbol graph before they report, so both need room for a
// subprocess rather than for reading two files. SessionStart stays the tighter of
// the two on purpose: a session that hangs on scc is worse than one that starts on
// a slightly stale index, and it is also the sync `scc launch` already did for
// every session started that way.
func (e Event) Timeout() int {
	if e == Stop {
		return 120
	}
	return 60
}

// HarnessStatus is one event's registration as it stands in a settings file.
type HarnessStatus struct {
	Harness string `json:"harness"`
	Event   Event  `json:"event"`
	Path    string `json:"path"`
	State   State  `json:"state"`
	Action  string `json:"action,omitempty"`
	Note    string `json:"note,omitempty"`
}

// command is what an entry scc wrote runs. It is also how scc recognizes its own
// entries on the way back in — there is no marker field to hide one in, and a
// schema scc does not own is not a place to invent keys.
func command(st Stage) string { return Prog + " hooks run " + string(st) }

// entry is the hook object scc writes, and the shape a look compares against: an
// entry that matches this byte for byte is current, and one that does not is from
// another build.
func entry(e Event) map[string]any {
	return map[string]any{
		"hooks": []any{map[string]any{
			"type":          "command",
			"command":       command(e.Stage()),
			"timeout":       e.Timeout(),
			"statusMessage": "scc",
		}},
	}
}

// LookHarness reports how every harness in this workspace is registered, without
// changing anything.
func LookHarness(root string) []HarnessStatus {
	return eachHarness(root, func(h paths.Harness, s *settings) []HarnessStatus {
		var out []HarnessStatus
		for _, e := range Events() {
			out = append(out, HarnessStatus{Harness: h.ID, Event: e,
				Path: h.Settings(root), State: s.state(e)})
		}
		return out
	})
}

// InstallHarness registers scc's hooks in every harness here that has a place for
// them, and reports what that took.
//
// The settings file belongs to the harness and to whoever else writes into it, so
// this splices rather than writes: scc's own entries are replaced, every other
// entry in the same event is left exactly where it is, and every key outside
// `hooks` survives untouched — including ones this build has never heard of.
func InstallHarness(root string) []HarnessStatus {
	return eachHarness(root, func(h paths.Harness, s *settings) []HarnessStatus {
		var out []HarnessStatus
		changed := false
		for _, e := range Events() {
			st := HarnessStatus{Harness: h.ID, Event: e, Path: h.Settings(root), State: s.state(e)}
			switch st.State {
			case Installed:
				st.Action = Present
			default:
				was := st.State
				s.set(e)
				changed = true
				st.State = Installed
				st.Action = Added
				if was == Stale {
					st.Action = Replaced
				}
			}
			out = append(out, st)
		}
		if changed {
			if err := s.save(); err != nil {
				for i := range out {
					out[i].Note, out[i].Action = err.Error(), ""
				}
			}
		}
		return out
	})
}

// RemoveHarness takes scc's entries back out and leaves the rest of the file as
// it found it — including an empty `hooks` object somebody else's tooling may be
// keeping, which is why only keys scc emptied are dropped.
func RemoveHarness(root string) []HarnessStatus {
	return eachHarness(root, func(h paths.Harness, s *settings) []HarnessStatus {
		var out []HarnessStatus
		changed := false
		for _, e := range Events() {
			st := HarnessStatus{Harness: h.ID, Event: e, Path: h.Settings(root), State: s.state(e)}
			if st.State == Missing {
				st.Action = Skipped
				out = append(out, st)
				continue
			}
			s.clear(e)
			changed = true
			st.State, st.Action = Missing, Removed
			out = append(out, st)
		}
		if changed {
			if err := s.save(); err != nil {
				for i := range out {
					out[i].Note, out[i].Action = err.Error(), ""
				}
			}
		}
		return out
	})
}

// HarnessOK reports whether every event that can be registered is.
func HarnessOK(all []HarnessStatus) bool {
	for _, s := range all {
		if s.State != Installed {
			return false
		}
	}
	return true
}

// eachHarness runs fn for every scaffolded harness that has a settings surface.
// A harness with none contributes nothing rather than a finding: Codex and
// opencode are not missing a file, they have no mechanism that would read one.
func eachHarness(root string, fn func(paths.Harness, *settings) []HarnessStatus) []HarnessStatus {
	var out []HarnessStatus
	for _, h := range workspace.Harnesses(root) {
		path := h.Settings(root)
		if path == "" {
			continue
		}
		s, err := loadSettings(path)
		if err != nil {
			out = append(out, HarnessStatus{Harness: h.ID, Path: path, State: Foreign, Note: err.Error()})
			continue
		}
		out = append(out, fn(h, s)...)
	}
	return out
}

// settings is one harness settings file, held as raw keys so everything scc does
// not understand survives a write.
//
// The same discipline internal/manifest keeps for unknown fields, for a stronger
// reason: that file is scc's and this one is not. A build that dropped a key it
// did not recognize would silently delete somebody's permissions, model choice or
// MCP server on the way past.
type settings struct {
	path string
	raw  map[string]json.RawMessage
}

func loadSettings(path string) (*settings, error) {
	s := &settings{path: path, raw: map[string]json.RawMessage{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	// An empty file is a file somebody created and never filled in, not a
	// syntax error to refuse over.
	if strings.TrimSpace(string(b)) == "" {
		return s, nil
	}
	if err := json.Unmarshal([]byte(textutil.NormalizeNewlines(string(b))), &s.raw); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON, so scc will not rewrite it: %w", filepath.Base(path), err)
	}
	return s, nil
}

// hooks returns the `hooks` object, event → entries.
func (s *settings) hooks() map[string][]json.RawMessage {
	out := map[string][]json.RawMessage{}
	raw, ok := s.raw["hooks"]
	if !ok {
		return out
	}
	byEvent := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &byEvent); err != nil {
		return out
	}
	for name, list := range byEvent {
		var entries []json.RawMessage
		if err := json.Unmarshal(list, &entries); err != nil {
			continue
		}
		out[name] = entries
	}
	return out
}

// state says what is registered for one event: this build's entry, another
// build's, or nothing.
func (s *settings) state(e Event) State {
	want, err := json.Marshal(entry(e))
	if err != nil {
		return Missing
	}
	for _, raw := range s.hooks()[string(e)] {
		if !mine(raw) {
			continue
		}
		if equalJSON(raw, want) {
			return Installed
		}
		return Stale
	}
	return Missing
}

// set replaces scc's entry for this event and leaves every other entry alone.
func (s *settings) set(e Event) {
	kept := notMine(s.hooks()[string(e)])
	next, err := json.Marshal(entry(e))
	if err != nil {
		return
	}
	s.put(e, append(kept, next))
}

// clear drops scc's entry and leaves the rest.
func (s *settings) clear(e Event) { s.put(e, notMine(s.hooks()[string(e)])) }

// put writes one event's entries back, dropping the event — and `hooks` itself —
// when scc emptied it. A key left behind holding an empty array is scc's litter
// in somebody else's file.
func (s *settings) put(e Event, entries []json.RawMessage) {
	all := s.hooks()
	if len(entries) == 0 {
		delete(all, string(e))
	} else {
		all[string(e)] = entries
	}
	if len(all) == 0 {
		delete(s.raw, "hooks")
		return
	}
	// Built from a map so encoding/json sorts the keys: the file is committed, and
	// a workspace that reordered it on every run would diff against itself.
	byEvent := map[string]json.RawMessage{}
	for name, list := range all {
		raw, err := json.Marshal(list)
		if err != nil {
			continue
		}
		byEvent[name] = raw
	}
	raw, err := json.Marshal(byEvent)
	if err != nil {
		return
	}
	s.raw["hooks"] = raw
}

// save writes the file the way scc writes every file: atomically, LF, two-space
// indent, one trailing newline — so a settings file scc touched still reviews as
// a diff of what changed.
func (s *settings) save() error {
	b, err := json.MarshalIndent(s.raw, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return workspace.AtomicWrite(s.path, []byte(textutil.NormalizeNewlines(string(b))+"\n"), 0o644)
}

// mine reports whether an entry is one scc wrote: any command hook in it running
// `scc hooks run`. Recognition by the command rather than by a marker field,
// because adding a key to a schema scc does not own is how a future validation
// error becomes scc's fault.
func mine(raw json.RawMessage) bool {
	var obj struct {
		Hooks []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return false
	}
	for _, h := range obj.Hooks {
		if strings.Contains(h.Command, Prog+" hooks run ") {
			return true
		}
	}
	return false
}

func notMine(entries []json.RawMessage) []json.RawMessage {
	var out []json.RawMessage
	for _, raw := range entries {
		if !mine(raw) {
			out = append(out, raw)
		}
	}
	return out
}

// equalJSON compares two documents by value rather than by bytes, so an entry
// that is current but spelled with different whitespace is not reported stale.
func equalJSON(a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	ax, err := json.Marshal(x)
	if err != nil {
		return false
	}
	by, err := json.Marshal(y)
	if err != nil {
		return false
	}
	return string(ax) == string(by)
}

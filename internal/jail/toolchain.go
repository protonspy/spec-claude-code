package jail

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// FlagMap is ai-jail's read-only extra mount: `--map PATH` shows a host path
// inside the sandbox at its own location, `--map SOURCE:DEST` at another one.
//
// It is the third flag scc asks for, and the last. The first two are what lets an
// agent run at all; this one is what lets it run *this workspace's* toolchain,
// which is the same argument one step further in. ai-jail binds the binary it was
// handed and says so plainly — "tools with needs beyond their install directory
// stay on the --map escape hatch" — so under the default private home an agent
// told by scc's own guidance to prefix every command with `rtk`, or to answer a
// question with `scc map tasks`, finds neither: they live in ~/.cargo/bin and the
// npm prefix, and $HOME inside the sandbox is a fresh tmpfs. PATH is then pruned
// to the directories that survived, so the entries naming them go too.
//
// A jail that starts an agent which cannot run the tools the file in front of it
// names is a jail that starts a useless agent — the same failure as one that
// cannot reach its model, arriving one step later.
const FlagMap = "--map"

// Mapping is one read-only mount scc asks the sandbox for. Dest empty means "at
// the path it already has", which is the form to prefer: a mount that moves a
// file is a mount somebody has to reason about later.
type Mapping struct {
	Src  string `json:"src"`
	Dest string `json:"dest,omitempty"`
}

// Spec is the mapping as ai-jail spells it: PATH, or SOURCE:DEST.
func (m Mapping) Spec() string {
	if m.Dest == "" || m.Dest == m.Src {
		return m.Src
	}
	return m.Src + ":" + m.Dest
}

// MapArgs turns mappings into ai-jail options, or reports that this build cannot
// take them.
//
// Read off the build's own help like the other two, and for the harder-learned
// half of the same reason: a flag guessed at here is a mount request the sandbox
// reads as something else. Nothing is substituted — a build with no --map starts
// the agent with a hidden toolchain, which is visible and diagnosable the moment
// the agent runs one command.
func MapArgs(flags map[string]bool, maps []Mapping) ([]string, bool) {
	if len(maps) == 0 {
		return nil, true
	}
	if !flags[FlagMap] {
		return nil, false
	}
	args := make([]string, 0, len(maps)*2)
	for _, m := range maps {
		// Two arguments rather than --map=SPEC: it is the spelling ai-jail's own
		// tests use, and a value carrying `=` cannot be misread in it.
		args = append(args, FlagMap, m.Spec())
	}
	return args, true
}

// HiddenRoot is the region this platform's backend takes away from the sandbox,
// and so the region a tool has to be mapped back out of.
//
// It is ai-jail's own choice of root, mirrored: on Linux the private home is a
// fresh tmpfs, so $HOME is what disappears; on macOS there is no mount namespace
// and the seatbelt profile denies by default everywhere, so everything outside
// the project does. Empty means nothing is hidden — the answer on a platform with
// no backend, where Supported is false and no launch gets this far.
func HiddenRoot() string {
	switch runtime.GOOS {
	case "linux":
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return home
	case "darwin":
		return string(filepath.Separator)
	default:
		return ""
	}
}

// Hidden reports whether path falls inside the region root names.
func Hidden(root, path string) bool {
	if root == "" || path == "" {
		return false
	}
	root, path = filepath.Clean(root), filepath.Clean(path)
	if root == string(filepath.Separator) {
		return filepath.IsAbs(path)
	}
	if path == root {
		return true
	}
	return strings.HasPrefix(path, root+string(filepath.Separator))
}

// maxInterpreterDepth bounds the shebang walk. A script run by a script run by a
// script is somebody's packaging accident, not a chain to follow forever.
const maxInterpreterDepth = 2

// Needs is the read-only mounts one tool needs to stay runnable when its install
// directory is hidden.
//
// entry is where PATH finds it — the name the agent will type. real is the file
// scc already knows sits behind that name, or empty to resolve the symlinks
// itself. Nothing is returned when neither is hidden: a tool in /usr/bin is
// already there.
//
// A compiled binary needs one mount, itself, shown at the name PATH knows. A
// script needs three things that are all the same fact — an interpreter, its own
// siblings, and the symlink structure its module resolution walks — so its bin
// directory and package root come with it. That is the npm-installed case, which
// is how both scc and codegraph are distributed.
func Needs(root, entry, real string) []Mapping {
	return needs(root, entry, real, 0)
}

func needs(root, entry, real string, depth int) []Mapping {
	if entry == "" || depth > maxInterpreterDepth {
		return nil
	}
	entry = filepath.Clean(entry)
	resolved := resolve(real)
	if resolved == "" {
		resolved = resolve(entry)
	}
	if resolved == "" {
		return nil
	}
	// ai-jail splits a map on its first colon, so a path containing one cannot be
	// expressed at all — and the request it would parse as is a mount of something
	// else. Nothing legitimate lands here; a mount scc cannot spell exactly is one
	// it does not ask for.
	if !spellable(entry) || !spellable(resolved) {
		return nil
	}
	if !Hidden(root, entry) && !Hidden(root, resolved) {
		return nil
	}

	var out []Mapping
	if resolved == entry {
		out = append(out, Mapping{Src: entry})
	} else {
		out = append(out, Mapping{Src: resolved, Dest: entry})
	}

	interp, script := shebang(resolved)
	if !script {
		// A compiled binary carries its dependencies or finds them under /usr,
		// which the sandbox mounts read-only anyway.
		return out
	}
	// The bin directory rather than the file: a global npm shim is a symlink into
	// the package tree, its interpreter is usually its neighbour, and node resolves
	// the real path of a script before looking for anything beside it. Mounting the
	// file alone at the name PATH knows would start it and then fail to find its
	// own package.
	if dir := filepath.Dir(entry); Hidden(root, dir) {
		out = append(out, Mapping{Src: dir})
	}
	if dir := payload(resolved); dir != "" && dir != filepath.Dir(entry) && Hidden(root, dir) {
		out = append(out, Mapping{Src: dir})
	}
	if interp != "" {
		if p, err := exec.LookPath(interp); err == nil {
			out = append(out, needs(root, p, "", depth+1)...)
		}
	}
	return out
}

// Dedupe drops repeats and anything a directory in the same set already covers,
// preserving order. Two tools installed the same way name the same interpreter
// and the same package root, and a command line that says so three times is one
// nobody reads.
func Dedupe(maps []Mapping) []Mapping {
	var dirs []string
	for _, m := range maps {
		if m.Dest == "" && isDir(m.Src) {
			dirs = append(dirs, m.Src)
		}
	}
	seen := map[string]bool{}
	out := make([]Mapping, 0, len(maps))
	for _, m := range maps {
		if seen[m.Spec()] {
			continue
		}
		if m.Dest == "" && under(m.Src, dirs) {
			continue
		}
		seen[m.Spec()] = true
		out = append(out, m)
	}
	return out
}

func under(path string, dirs []string) bool {
	for _, d := range dirs {
		if path != d && Hidden(d, path) {
			return true
		}
	}
	return false
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// spellable reports whether a path can be said in ai-jail's map syntax, which
// splits on its first colon. A drive letter is discounted: it is a Windows path,
// and no sandbox runs there — the suite does.
func spellable(path string) bool {
	return !strings.ContainsRune(path[len(filepath.VolumeName(path)):], ':')
}

// resolve is EvalSymlinks with the original kept when it fails, since a path that
// cannot be resolved is still a path worth mounting.
func resolve(path string) string {
	if path == "" {
		return ""
	}
	if p, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(p)
	}
	return filepath.Clean(path)
}

// payload is the directory a script's own resolution walks: the outermost
// node_modules above it when there is one, so a global npm package finds the
// sibling packages it depends on, and otherwise the directory it sits in.
func payload(path string) string {
	dir := filepath.Dir(path)
	parts := strings.Split(dir, string(filepath.Separator))
	for i, p := range parts {
		if p == "node_modules" {
			return strings.Join(parts[:i+1], string(filepath.Separator))
		}
	}
	return dir
}

// shebang reads the interpreter a script names, and reports whether the file is a
// script at all. `#!/usr/bin/env node` answers "node", because the interpreter
// that matters there is the one PATH has to find.
func shebang(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()

	r := bufio.NewReader(f)
	head, err := r.Peek(2)
	if err != nil || string(head) != "#!" {
		return "", false
	}
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		return "", true
	}
	fields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "#!"))
	for i, field := range fields {
		if i == 0 {
			if filepath.Base(field) != "env" {
				return field, true
			}
			continue
		}
		// `env -S node --flag` and `env NAME=value node` both put the command
		// after the options and the assignments.
		if strings.HasPrefix(field, "-") || strings.Contains(field, "=") {
			continue
		}
		return field, true
	}
	return "", true
}

// Package workspace resolves the scc workspace root and owns the primitives
// every writer goes through: name validation for hostile input, and atomic
// writes. It knows nothing about specs, wikis, or PRDs — only about the
// filesystem contract those features are built on.
package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/paths"
)

var kebabRe = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// ErrAlreadyExists signals a refusal to overwrite an existing artifact without --force.
var ErrAlreadyExists = errors.New("already exists")

// KebabCheck rejects names that are not kebab-case, returning a descriptive error.
// Every resource scc names on disk is kebab-case so a name maps to exactly one
// path on case-insensitive filesystems too.
func KebabCheck(name, kind string) error {
	if !kebabRe.MatchString(name) {
		return fmt.Errorf("%s name must be kebab-case (lowercase, hyphen-separated): got %q", kind, name)
	}
	return nil
}

// SafeName rejects a positional artifact name that could escape its parent
// directory when joined onto a workspace path. Every delete/show/generate lookup
// MUST call this before filepath.Join, because those names arrive from CLI args
// and may be malformed or hostile — without it, `scc spec delete .. --force`
// resolves to the workspace root and RemoveAll deletes the whole project. It
// rejects path separators, "." / "..", absolute paths, and (on Windows) reserved
// device names via filepath.IsLocal.
func SafeName(name, kind string) error {
	if name == "" {
		return fmt.Errorf("%s name is required", kind)
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." || !filepath.IsLocal(name) {
		return fmt.Errorf("invalid %s name %q: must be a single path segment with no separators or '..'", kind, name)
	}
	return nil
}

// AtomicWrite writes data to a temp file in the same directory as path and
// renames it into place, so a crash or a concurrent reader never observes a
// truncated or half-written file. The temp file is created in the destination
// directory so the rename stays on one filesystem.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op once the rename succeeds
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// Find walks upward from start looking for the workspace marker and returns the
// directory holding it, or start unchanged when there is none.
//
// The marker is the file .claude/scc-manifest.json, not the .claude/ directory.
// That distinction is the whole point, for two reasons: ~/.claude is Claude
// Code's own global configuration directory and exists on every machine that
// runs Claude Code, so a walk accepting the directory would resolve the root to
// $HOME for any command run outside a workspace — and every command would then
// read and write the user's global configuration. And .claude/ exists in every
// repo that merely uses Claude Code, where scc was never initialized. Only scc
// writes the manifest, so its presence answers exactly the right question.
func Find(start string) string {
	abs, err := filepath.Abs(start)
	if err != nil {
		return start
	}
	cur := abs
	for {
		if isFile(paths.Manifest(cur)) {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs
		}
		cur = parent
	}
}

// isFile reports whether path exists and is a regular file. A directory that
// happens to carry the manifest's name is not a marker.
func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// IsWorkspace reports whether root holds the marker — i.e. whether it is an
// initialized workspace rather than just the directory the walk fell back to.
func IsWorkspace(root string) bool { return isFile(paths.Manifest(root)) }

// Resolve normalizes the user-supplied --root flag. An empty arg falls back to
// Find(cwd). A non-empty arg must exist on disk, otherwise an error is returned.
func Resolve(arg string) (string, error) {
	if arg != "" {
		p, err := filepath.Abs(arg)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("--root path does not exist: %s", p)
		}
		return p, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return Find(cwd), nil
}

// SafeWrite writes content to path only when path is missing. Returns true when
// the file was created.
func SafeWrite(path, content string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	}
	if err := AtomicWrite(path, []byte(content), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// WriteFile writes content unconditionally unless the path exists and overwrite
// is false.
func WriteFile(path, content string, overwrite bool) error {
	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("refusing to overwrite existing file: %s (pass --force)", path)
		}
	}
	return AtomicWrite(path, []byte(content), 0o644)
}

// Relative reports the path of target relative to root, for friendly logging.
func Relative(root, target string) string {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return target
	}
	return rel
}

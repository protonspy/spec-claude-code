package hooks

import (
	"fmt"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/git"
)

// shebang is what a hook scc creates from nothing starts with. POSIX sh rather
// than bash: git ships one on Windows and every platform has one, and the block
// below uses nothing bash adds.
const shebang = "#!/bin/sh"

// Dir is where this repository runs its hooks from — `.git/hooks` unless
// core.hooksPath says otherwise, which git is asked rather than assumed.
func Dir(root string) (string, error) {
	if !git.IsRepo(root) {
		return "", fmt.Errorf("%s is not a git repository, so it has no hooks to install", root)
	}
	return git.HooksDir(root)
}

// script is the block scc owns inside one hook file.
//
// Three decisions are in these lines. It **calls back into scc** rather than
// deciding anything itself, so the gate is the same code `scc validate` runs and a
// fix ships by upgrading the binary instead of rewriting a file in somebody's
// `.git`. It **fails closed** when scc is not on PATH, because a hook that
// silently passes when its checker is missing is worse than no hook — it reports
// success it did not verify. And it **names the way past itself** in the message
// it prints: a gate with no documented escape is a gate people delete the first
// time it is wrong, and then nothing checks anything.
func script(st Stage) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s — written by `%s hooks install`; re-run it to update\n", Markers.Open, Version, Prog)
	fmt.Fprintf(&b, "# %s\n", st.Why())
	fmt.Fprintf(&b, "# Skip it with %s=1, or with git's --no-verify where that applies.\n", SkipEnv)
	fmt.Fprintf(&b, "if [ -z \"${%s:-}\" ]; then\n", SkipEnv)
	fmt.Fprintf(&b, "  if command -v %s >/dev/null 2>&1; then\n", Prog)
	fmt.Fprintf(&b, "    %s hooks run %s \"$@\"\n", Prog, st)
	b.WriteString("    code=$?\n")
	// Exit 1 is "could not run", and the likeliest cause by far is an scc older
	// than the hook it is being asked to run — an upgrade installs a block naming a
	// subcommand the binary on PATH may not have yet. Findings exit 2 and say what
	// they found; this one would otherwise be an unexplained refusal to commit.
	fmt.Fprintf(&b, "    if [ $code -eq 1 ]; then echo \"%s could not run this gate — if it is older than the hook, upgrade it and re-run \\`%s hooks install\\`\" >&2; fi\n", Prog, Prog)
	b.WriteString("    [ $code -eq 0 ] || exit $code\n")
	b.WriteString("  else\n")
	fmt.Fprintf(&b, "    echo \"%s: not on PATH — this repository gates %s on it (%s=1 skips)\" >&2\n", Prog, st, SkipEnv)
	b.WriteString("    exit 1\n")
	b.WriteString("  fi\n")
	b.WriteString("fi\n")
	b.WriteString(Markers.Close)
	return b.String()
}

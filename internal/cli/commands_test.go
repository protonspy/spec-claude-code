package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/assets"
	"github.com/protonspy/spec-claude-code/internal/paths"
)

// TestScaffoldedGuidanceOnlyNamesCommandsThatExist is the canary for the worst
// thing a rule can do: send the agent to a command that is not there.
//
// It is not hypothetical. `code-search.md` routed three questions to `scc graph
// impact`, `callers` and `callees`, none of which the dispatcher has ever had, and
// nothing caught it — the length budget is tested, the entry file's coverage of the
// rule set is tested, and what the prose actually tells the agent to type was
// tested by nobody. That is the same failure the CodeGraph usage block is withheld
// until launch to avoid, and the reasoning is written down there: guidance naming a
// command the machine cannot run costs the file its credibility, and an agent that
// has watched one line fail discounts the rest of it. A rule is only worth its
// standing place in the context window while every line of it is true.
//
// The dispatcher is read rather than run. Running these would mean starting an
// agent, installing a toolchain, or scaffolding a tree — `scc launch` and `scc
// update` are in this set — so the test asks the source what it dispatches and
// compares that against what the templates tell people to type.
func TestScaffoldedGuidanceOnlyNamesCommandsThatExist(t *testing.T) {
	resources, subs := dispatcher(t)

	checked := 0

	for _, h := range paths.Harnesses() {
		for _, f := range assets.Workspace(h) {
			raw, err := assets.Render(h, f)
			if err != nil {
				t.Fatalf("%s: %s: %v", h.ID, f.Name, err)
			}
			for _, c := range commandsIn(raw) {
				checked++
				handler, ok := resources[c.resource]
				if !ok {
					t.Errorf("%s: %s names `scc %s`, which is not a command",
						h.ID, f.Name, c.resource)
					continue
				}
				// A resource whose handler has no subcommand switch takes flags and
				// positionals, so the second word is an argument rather than a name
				// to check. `scc validate --checks` is the shape.
				known, dispatches := subs[handler]
				if c.sub == "" || !dispatches {
					continue
				}
				if !known[c.sub] {
					t.Errorf("%s: %s names `scc %s %s`, and %s dispatches only %s",
						h.ID, f.Name, c.resource, c.sub, c.resource, sorted(known))
				}
			}
		}
	}
	// A canary that stops matching is a canary that passes. The floor sits well under
	// what the templates hold today, so ordinary editing never trips it while the one
	// way this check could go quietly vacuous — an extraction that matches nothing —
	// fails loudly.
	if checked < 200 {
		t.Errorf("checked only %d command mentions across %d harnesses; the extraction is broken",
			checked, len(paths.Harnesses()))
	}
}

type command struct{ resource, sub string }

// codeRegion is the only place a command is looked for: a fenced block or an
// inline code span. Prose says "the spec command" and means no such thing, so
// reading the whole file would hand the check a stream of English to reject.
var (
	codeSpan    = regexp.MustCompile("`[^`\n]+`")
	commandText = regexp.MustCompile(`\bscc\s+([a-z][a-z-]*)(?:\s+([a-z][a-z-]*))?`)
)

// commandsIn pulls every `scc <resource> [<sub>]` out of a rendered template.
//
// The second word counts only when it is a bare lower-case word. A placeholder,
// a flag, a quoted string or an alternation (`query|explore`) is not a subcommand
// and is left alone — this is a canary rather than a parser, and a false finding
// on a line that was always fine costs more than the one it would catch.
func commandsIn(raw string) []command {
	var regions []string
	inFence := false
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			regions = append(regions, line)
			continue
		}
		for _, span := range codeSpan.FindAllString(line, -1) {
			regions = append(regions, strings.Trim(span, "`"))
		}
	}

	var out []command
	for _, r := range regions {
		for _, m := range commandText.FindAllStringSubmatch(r, -1) {
			out = append(out, command{resource: m[1], sub: m[2]})
		}
	}
	return out
}

// dispatcher reads this package's own command surface: which resources Run
// dispatches, and which subcommands each resource's handler dispatches in turn.
//
// Every dispatcher in the package is `switch args[0]`, which is what makes this
// readable without a registry. If that idiom ever changes, the switch is found by
// nothing, the resource resolves to no handler, and the test fails loudly — the
// one direction a canary is allowed to fail in.
func dispatcher(t *testing.T) (map[string]string, map[string]map[string]bool) {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	fset := token.NewFileSet()
	subs := map[string]map[string]bool{}
	resources := map[string]string{}

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			for _, sw := range argSwitches(fn) {
				for _, clause := range sw.Body.List {
					cc, ok := clause.(*ast.CaseClause)
					if !ok {
						continue
					}
					for _, name := range caseNames(cc) {
						if subs[fn.Name.Name] == nil {
							subs[fn.Name.Name] = map[string]bool{}
						}
						subs[fn.Name.Name][name] = true
						if fn.Name.Name == "Run" {
							if h := handlerIn(cc); h != "" {
								resources[name] = h
							}
						}
					}
				}
			}
		}
	}

	if len(resources) == 0 {
		t.Fatal("found no resources in Run's dispatch — the `switch args[0]` idiom this reads has changed")
	}
	return resources, subs
}

// argSwitches finds every `switch args[0]` in a function, nested ones included:
// `scc graph scope` dispatches from inside its own handler.
func argSwitches(fn *ast.FuncDecl) []*ast.SwitchStmt {
	var out []*ast.SwitchStmt
	ast.Inspect(fn, func(n ast.Node) bool {
		sw, ok := n.(*ast.SwitchStmt)
		if !ok {
			return true
		}
		idx, ok := sw.Tag.(*ast.IndexExpr)
		if !ok {
			return true
		}
		ident, ok := idx.X.(*ast.Ident)
		lit, litOK := idx.Index.(*ast.BasicLit)
		if ok && litOK && ident.Name == "args" && lit.Value == "0" {
			out = append(out, sw)
		}
		return true
	})
	return out
}

// caseNames is the string literals one case clause matches, unquoted.
func caseNames(cc *ast.CaseClause) []string {
	var out []string
	for _, expr := range cc.List {
		lit, ok := expr.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}
		v, err := strconv.Unquote(lit.Value)
		if err != nil {
			continue
		}
		out = append(out, v)
	}
	return out
}

// handlerIn is the run<Resource> a clause of Run's switch hands off to, which is
// the link between a resource's name and the subcommands it takes.
func handlerIn(cc *ast.CaseClause) string {
	var name string
	ast.Inspect(cc, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok && strings.HasPrefix(ident.Name, "run") {
			name = ident.Name
			return false
		}
		return true
	})
	return name
}

func sorted(set map[string]bool) string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return strings.Join(out, " | ")
}

package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/assets"
	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/notes"
	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/render"
	"github.com/protonspy/spec-claude-code/internal/validate"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// runNotes dispatches `scc notes <subcommand>`: the project's note log, written
// one line at a time and read one query at a time.
//
// The command exists because the file's whole contract is a format, and a format
// nobody can be made to type is a format that decays. `notes add` is the only
// writer scc ships, so every note it puts in the log is one `notes find` can
// return — and the validator is there for the lines that arrive some other way.
//
// It is also the answer to the obvious objection: if the file is greppable, why a
// CLI at all? Because grep answers "which lines contain this" and a reader is
// asking three other things — what tags exist before I coin a fourth, what does
// this project already know about this path, what is new since last week — and
// each of those is a filter over parsed fields rather than a substring. The line
// format stays greppable anyway, so nothing here is a gatekeeper: `grep '#gotcha'
// docs/notes.md` and `scc notes find --tag gotcha` return the same lines.
func runNotes(args []string) int {
	if len(args) == 0 {
		notesUsage()
		return ExitError
	}
	switch args[0] {
	case "help", "-h", "--help":
		notesUsage()
		return ExitOK
	case "add":
		return runNotesAdd(args[1:])
	case "find":
		return runNotesFind(args[1:])
	case "show":
		return runNotesShow(args[1:])
	case "tags":
		return runNotesIndex(args[1:], "tags")
	case "paths":
		return runNotesIndex(args[1:], "paths")
	case "rm":
		return runNotesRemove(args[1:])
	case "validate":
		return runNotesValidate(args[1:])
	default:
		render.Err(fmt.Sprintf("unknown notes subcommand %q", args[0]))
		fmt.Fprintf(os.Stderr, "run `%s notes help` for the available subcommands\n", prog())
		return ExitError
	}
}

// loadNotes reads and parses docs/notes.md. A missing file is an empty log rather
// than an error: every query has to work in a workspace where nobody has written a
// note yet, which is every workspace on its first day.
func loadNotes(root string) (*notes.File, string, bool) {
	abs := paths.Notes(root)
	rel := relPath(root, abs)
	b, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			f, _ := notes.Parse(rel, "")
			return f, rel, true
		}
		render.Err(err.Error())
		return nil, rel, false
	}
	f, err := notes.Parse(rel, string(b))
	if err != nil {
		render.Err(err.Error())
		return nil, rel, false
	}
	return f, rel, true
}

func runNotesAdd(args []string) int {
	fs := flag.NewFlagSet("notes add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	tags := fs.String("tag", "", "the note's tags, comma-separated `list` — at least one")
	scopes := fs.String("path", "", "what the note is about: repo-relative `path`s, comma-separated")
	date := fs.String("date", "", "override the note's `date` (YYYY-MM-DD); today by default")
	force := fs.Bool("force", false, "write even if the note introduces a validation finding")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	// The text is the positionals joined, so an unquoted sentence arrives as the
	// sentence it was typed as. `scc graph explore` does the same, for the same
	// reason: quoting discipline is a tax paid at the moment somebody is least
	// inclined to pay it.
	text, err2 := notes.CheckText(strings.Join(rest, " "))
	if err2 != nil {
		render.Err(err2.Error())
		render.Detail(fmt.Sprintf("  %s notes add \"the observation\" --tag gotcha --path internal/cli/notes.go", prog()))
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}

	n := notes.Note{Date: notes.Today(), Text: text}
	if *date != "" {
		if err := notes.CheckDate(*date); err != nil {
			render.Err(err.Error())
			return ExitError
		}
		n.Date = *date
	}
	for _, t := range splitList(*tags) {
		if err := notes.CheckTag(t); err != nil {
			render.Err(err.Error())
			return ExitError
		}
		n.Tags = append(n.Tags, t)
	}
	if len(n.Tags) == 0 {
		// Required rather than defaulted. A tag is the index this file is queried
		// by, and a default like "note" would be a tag on everything, which is a tag
		// on nothing — the drift the tag index exists to prevent, seeded by scc.
		render.Err("a note needs at least one --tag: it is what the log is queried by")
		render.Detail(fmt.Sprintf("  %s notes tags   lists the ones this project already uses", prog()))
		return ExitError
	}
	for _, p := range splitList(*scopes) {
		clean, err := notes.CheckPath(p)
		if err != nil {
			render.Err(err.Error())
			return ExitError
		}
		n.Paths = append(n.Paths, clean)
	}

	abs := paths.Notes(target)
	original, existed, ok := ensureNotesFile(abs)
	if !ok {
		return ExitError
	}
	f, err := notes.Parse(relPath(target, abs), original)
	if err != nil {
		render.Err(err.Error())
		return ExitError
	}
	n.ID = notes.ID(f.Next())
	content, err := f.Append(n)
	if err != nil {
		render.Err(err.Error())
		render.Detail(fmt.Sprintf("  add a `## %s` heading to %s, or delete the file and let this command reseed it",
			notes.Section, relPath(target, abs)))
		return ExitError
	}

	// The same contract `scc patch` writes under: the file is re-validated after the
	// write and the write is undone when it introduced a finding. It matters less
	// here than it does for a plan — a note is one appended line — and it is worth
	// having anyway, because the finding it catches is a hand-edited log this
	// command was about to append a duplicate id to.
	before, _ := validate.Notes(target)
	if err := workspace.AtomicWrite(abs, []byte(withTrailingNewline(content)), 0o644); err != nil {
		render.Err(err.Error())
		return ExitError
	}
	after, _ := validate.Notes(target)
	// A path that does not resolve is reported and never blocking, which is the one
	// place this command departs from `scc patch`. The stale check is about a log
	// aging past the code it describes; at the moment of writing, a note about a file
	// this branch has not created yet is exactly the note most worth having, and
	// refusing it would teach the user to stop passing --path at all.
	blocking, stale := split(newFindings(before.Sorted(), after.Sorted()), staleRule)
	verified := "clean"
	if len(blocking) > 0 {
		if *force {
			verified = "forced"
		} else {
			restore := []byte(original)
			if !existed {
				restore = nil
			}
			if err := undoNotesWrite(abs, restore); err != nil {
				render.Err("the note introduced findings and the rollback failed: " + err.Error())
				return ExitError
			}
			verified = "rolled-back"
		}
	}

	written := verified != "rolled-back"
	introduced := blocking
	if *jsonOut {
		code := ExitOK
		if len(introduced) > 0 && !written {
			code = ExitFindings
		}
		if emitJSON(struct {
			Note       notes.Note        `json:"note"`
			Line       string            `json:"line"`
			Path       string            `json:"path"`
			Written    bool              `json:"written"`
			Verified   string            `json:"verified"`
			Introduced []finding.Finding `json:"introduced,omitempty"`
			Warnings   []finding.Finding `json:"warnings,omitempty"`
		}{n, n.Format(), relPath(target, abs), written, verified, introduced, stale}) != ExitOK {
			return ExitError
		}
		return code
	}
	if !written {
		render.Err(fmt.Sprintf("not written: the note would introduce %d finding(s) in %s",
			len(introduced), relPath(target, abs)))
		for _, f := range introduced {
			render.Detail(fmt.Sprintf("  %s  %s", f.Rule, f.Message))
		}
		render.Detail("  fix the log first, or pass --force")
		return ExitFindings
	}
	for _, f := range stale {
		render.Warn(f.Message)
	}
	render.OK(n.Format())
	return ExitOK
}

// staleRule is the one finding `notes add` reports without acting on.
const staleRule = "notes.stale-path"

// split partitions findings by rule: everything else first, then the named rule.
func split(all []finding.Finding, rule string) (rest, named []finding.Finding) {
	for _, f := range all {
		if f.Rule == rule {
			named = append(named, f)
			continue
		}
		rest = append(rest, f)
	}
	return rest, named
}

// ensureNotesFile returns the current content of docs/notes.md, seeding it from the
// embedded anchor when it is not there.
//
// Seeding here rather than only at `scc init` is what makes the log reachable in a
// workspace scaffolded before this shipped: seeds are written once and tracked
// nowhere, so `scc update` will never deliver one. The first `notes add` is the
// other honest moment to create it — and the seed carries the format, so the file
// explains itself to whoever opens it next.
func ensureNotesFile(abs string) (content string, existed bool, ok bool) {
	b, err := os.ReadFile(abs)
	if err == nil {
		return string(b), true, true
	}
	if !os.IsNotExist(err) {
		render.Err(err.Error())
		return "", false, false
	}
	seed, err := assets.Content(notesSeed)
	if err != nil {
		render.Err(err.Error())
		return "", false, false
	}
	return seed, false, true
}

// notesSeed is the embedded anchor's name. Named here rather than inlined because
// assets.Seeds() names the same file, and the two must not drift.
const notesSeed = "docs/notes.md"

// undoNotesWrite restores what was there before, including "nothing at all" — a
// rollback that left a seeded file behind would report a note as not written and
// still change the workspace.
func undoNotesWrite(abs string, original []byte) error {
	if original == nil {
		return os.Remove(abs)
	}
	return workspace.AtomicWrite(abs, original, 0o644)
}

func runNotesFind(args []string) int {
	fs := flag.NewFlagSet("notes find", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	tags := fs.String("tag", "", "only notes carrying one of these tags, comma-separated `list`")
	scopes := fs.String("path", "", "only notes about one of these `path`s, or anything under them")
	since := fs.String("since", "", "only notes dated on or after this `date` (YYYY-MM-DD)")
	limit := fs.Int("limit", 0, "at most this many, newest first; 0 is all of them")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	if *since != "" {
		if err := notes.CheckDate(*since); err != nil {
			render.Err(err.Error())
			return ExitError
		}
	}
	f, rel, ok := loadNotes(target)
	if !ok {
		return ExitError
	}

	q := notes.Query{Tags: splitList(*tags), Paths: splitList(*scopes), Since: *since, Terms: rest}
	for i, p := range q.Paths {
		clean, err := notes.CheckPath(p)
		if err != nil {
			render.Err(err.Error())
			return ExitError
		}
		q.Paths[i] = clean
	}
	hits := f.Match(q)
	total := len(hits)
	if *limit > 0 && total > *limit {
		hits = hits[total-*limit:]
	}

	if *jsonOut {
		return emitJSON(struct {
			Path    string       `json:"path"`
			Notes   []notes.Note `json:"notes"`
			Count   int          `json:"count"`
			Matched int          `json:"matched"`
		}{rel, hits, len(hits), total})
	}
	if total == 0 {
		render.Info(fmt.Sprintf("no notes match — %d in %s", len(f.Notes), rel))
		return ExitOK
	}
	// Nothing but the notes on stdout, in the form the file holds them: the output
	// of this command and a grep over the file are deliberately the same text, so
	// neither one teaches a format the other contradicts.
	for _, n := range hits {
		fmt.Println(n.Format())
	}
	if len(hits) < total {
		// Never a silent cap. A truncated answer that looks complete is the one way
		// this command can mislead, so what was dropped is said out loud — on stderr,
		// so stdout stays the notes and nothing else.
		render.Warn(fmt.Sprintf("--limit %d: showing the %d newest of %d matches", *limit, len(hits), total))
	}
	return ExitOK
}

func runNotesShow(args []string) int {
	fs := flag.NewFlagSet("notes show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if len(rest) != 1 {
		render.Err(fmt.Sprintf("expected exactly one note id, got %d", len(rest)))
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	f, rel, ok := loadNotes(target)
	if !ok {
		return ExitError
	}
	n, found := f.Get(rest[0])
	if !found {
		// A removed note answers differently from one that never existed: the id was
		// spent, and a reader who followed a citation here deserves to be told that
		// rather than to doubt the citation.
		if line, was := f.Retired[rest[0]]; was {
			render.Err(fmt.Sprintf("%s was removed (%s:%d)", rest[0], rel, line))
			return ExitError
		}
		render.Err(fmt.Sprintf("no note %q in %s", rest[0], rel))
		return ExitError
	}
	if *jsonOut {
		return emitJSON(struct {
			Path string     `json:"path"`
			Note notes.Note `json:"note"`
			Line string     `json:"line"`
		}{rel, n, n.Format()})
	}
	fmt.Println(n.Format())
	return ExitOK
}

// runNotesIndex is `notes tags` and `notes paths`: the two questions asked before
// writing rather than after. One function because they are one report over two
// fields, and two commands because "which tags exist" and "what do we know about
// this area" are asked at different moments.
func runNotesIndex(args []string, which string) int {
	fs := flag.NewFlagSet("notes "+which, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if !noPositionals(rest, "notes "+which) {
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	f, rel, ok := loadNotes(target)
	if !ok {
		return ExitError
	}
	index := f.Tags()
	if which == "paths" {
		index = f.Paths()
	}
	if *jsonOut {
		return emitJSON(struct {
			Path  string        `json:"path"`
			Index []notes.Count `json:"index"`
			Kind  string        `json:"kind"`
			Count int           `json:"count"`
		}{rel, index, which, len(index)})
	}
	if len(index) == 0 {
		render.Info(fmt.Sprintf("no %s yet — %d notes in %s", which, len(f.Notes), rel))
		return ExitOK
	}
	for _, e := range index {
		fmt.Printf("%4d  %s\n", e.Count, e.Name)
	}
	return ExitOK
}

func runNotesRemove(args []string) int {
	fs := flag.NewFlagSet("notes rm", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if len(rest) != 1 {
		render.Err(fmt.Sprintf("expected exactly one note id, got %d", len(rest)))
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	abs := paths.Notes(target)
	f, rel, ok := loadNotes(target)
	if !ok {
		return ExitError
	}
	content, n, err := f.Remove(rest[0])
	if err != nil {
		render.Err(err.Error())
		return ExitError
	}
	if err := workspace.AtomicWrite(abs, []byte(withTrailingNewline(content)), 0o644); err != nil {
		render.Err(err.Error())
		return ExitError
	}
	if *jsonOut {
		return emitJSON(struct {
			Path    string     `json:"path"`
			Removed notes.Note `json:"removed"`
			Line    string     `json:"line"`
		}{rel, n, n.Format()})
	}
	// What was removed, in full: the number is spent for good — a tombstone stays
	// where the note stood so it is never handed out again — and this line is the
	// only copy the caller still has outside git.
	render.OK("removed " + n.ID)
	render.Detail("  " + n.Format())
	return ExitOK
}

func runNotesValidate(args []string) int {
	return runValidation("notes", args, func(root string, rest []string) (*finding.Set, error) {
		if !noPositionals(rest, "notes validate") {
			return nil, errUsage
		}
		return validate.Notes(root)
	})
}

func notesUsage() {
	fmt.Fprintf(os.Stderr, `%s notes — the project's note log: %s

Usage:
  %s notes <subcommand> [flags]

Subcommands:
  add       Append a note: add "<text>" --tag <t>[,<t>] [--path <p>[,<p>]] [--date]
  find      Query it: [terms…] [--tag] [--path] [--since] [--limit]
  show      One note by id
  tags      The tags in use, most used first — read this before coining one
  paths     The paths the log knows something about
  rm        Remove one note by id (the number is never reused)
  validate  Check the log's shape; exit 2 on findings

A note is one line, and that is the contract: a match is a whole note, so

  grep ' #gotcha ' %s

answers the same question as "notes find --tag gotcha". Anything needing a second
line is a wiki page, an ADR, or a task — never a longer note.

  - %s0001 2026-08-27 #gotcha @internal/cli/notes.go — one line, index fields first
`, render.Bold(prog()), paths.DocsSeg+"/"+paths.NotesSeg, prog(),
		paths.DocsSeg+"/"+paths.NotesSeg, notes.IDPrefix)
}

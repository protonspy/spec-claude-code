// Package textutil holds tiny, dependency-free text helpers shared across the
// codebase. It exists so line-ending and BOM handling is defined in exactly one
// place: scc runs on Windows/WSL where the working tree is frequently CRLF, and
// every content comparison (manifest hashes, frontmatter parsing, Markdown
// regexes) must treat "\r\n" and "\n" identically or the tool misbehaves.
package textutil

import "strings"

// bom is U+FEFF, which editors like Windows Notepad prepend to files. It is not
// Unicode whitespace, so it must be stripped explicitly. Built from its code
// point so the byte sequence never appears literally in this source file.
var bom = string(rune(0xFEFF))

// NormalizeNewlines converts Windows (\r\n) and classic-Mac (lone \r) line
// endings to Unix (\n) and strips a leading UTF-8 BOM. The result is what every
// parser and content hash in scc should operate on, so behavior is identical
// regardless of how git checked the files out.
func NormalizeNewlines(s string) string {
	s = strings.TrimPrefix(s, bom)
	if strings.IndexByte(s, '\r') == -1 {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

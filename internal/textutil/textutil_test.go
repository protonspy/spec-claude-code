package textutil

import "testing"

func TestNormalizeNewlines(t *testing.T) {
	bomPrefix := string(rune(0xFEFF))
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"already lf", "a\nb\n", "a\nb\n"},
		{"crlf", "a\r\nb\r\n", "a\nb\n"},
		{"lone cr", "a\rb\r", "a\nb\n"},
		{"mixed", "a\r\nb\rc\n", "a\nb\nc\n"},
		{"bom stripped", bomPrefix + "a\n", "a\n"},
		{"bom with crlf", bomPrefix + "a\r\n", "a\n"},
		{"bom only at start", "a" + bomPrefix + "b", "a" + bomPrefix + "b"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NormalizeNewlines(c.in); got != c.want {
				t.Errorf("NormalizeNewlines(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// Normalizing is idempotent — content hashes depend on it, so a second pass must
// never change the result.
func TestNormalizeNewlinesIdempotent(t *testing.T) {
	in := string(rune(0xFEFF)) + "a\r\nb\rc\n"
	once := NormalizeNewlines(in)
	if twice := NormalizeNewlines(once); twice != once {
		t.Errorf("not idempotent: %q then %q", once, twice)
	}
}

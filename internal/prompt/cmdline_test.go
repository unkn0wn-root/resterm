package prompt

import "testing"

func TestQuoteRoundTrips(t *testing.T) {
	for _, value := range []string{
		"plain.http",
		"api/",
		"space dir/",
		"path with spaces.http",
		`fixtures/a"b.http`,
		`fixtures/a\"b.http`,
		"single'quote.http",
		`C:\api\users.http`,
		`C:\Program Files\`,
		`Program Files\`,
		`weird\`,
		`\`,
		`\\`,
		`a"b\`,
		`a b\\`,
	} {
		t.Run(value, func(t *testing.T) {
			encoded := Quote(value)
			line := Lex(encoded)
			if line.Unclosed != 0 {
				t.Fatalf("Quote(%q) = %q, which lexes with an unclosed %q", value, encoded, line.Unclosed)
			}
			if len(line.Tokens) != 1 {
				t.Fatalf("Quote(%q) = %q, which lexes as %d tokens", value, encoded, len(line.Tokens))
			}
			if got := line.Tokens[0].Value; got != value {
				t.Fatalf("Quote(%q) = %q, which decodes to %q", value, encoded, got)
			}
		})
	}
}

// A completed path is written into a command line next to other arguments, so
// it has to survive being lexed back out of one.
func TestQuotedPathSurvivesACommandLine(t *testing.T) {
	for _, dir := range []string{`Program Files\`, "space dir/", `C:\Program Files\`} {
		line := Lex("mock start --source " + Quote(dir))
		if line.Unclosed != 0 {
			t.Fatalf("--source %q lexes with an unclosed %q", dir, line.Unclosed)
		}
		values := line.Values()
		if len(values) != 4 || values[3] != dir {
			t.Fatalf("--source %q lexes as %q", dir, values)
		}
	}
}

func TestQuoteLeavesPlainValuesAlone(t *testing.T) {
	for _, value := range []string{"api/users.http", `C:\api\users.http`, `weird\`} {
		if got := Quote(value); got != value {
			t.Fatalf("Quote(%q) = %q, want it left alone", value, got)
		}
	}
}

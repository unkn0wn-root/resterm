package directive

import "testing"

func TestCutName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		word string
		rest string
	}{
		{name: "bare", in: "Create using=First", word: "Create", rest: " using=First"},
		{name: "only word", in: "Create", word: "Create"},
		{
			name: "quoted holds spaces",
			in:   `"Create Account" using=First`,
			word: "Create Account",
			rest: " using=First",
		},
		{
			name: "quoted holds an equals sign",
			in:   `"a=b" using=First`,
			word: "a=b",
			rest: " using=First",
		},
		{
			name: "quoted holds a quote",
			in:   `"Say \"hi\"" using=First`,
			word: `Say "hi"`,
			rest: " using=First",
		},
		{
			name: "quoted holds a backslash",
			in:   `"Path\\Name" using=First`,
			word: `Path\Name`,
			rest: " using=First",
		},
		// A bare word is not escaped text, so the backslash is part of the name.
		{name: "bare keeps a backslash", in: `Path\Name x=1`, word: `Path\Name`, rest: " x=1"},
		{name: "single quotes group too", in: `'Create Account' x=1`, word: "Create Account", rest: " x=1"},
		// An option is never a name, otherwise a step written without one would
		// take its first option as the alias.
		{name: "leading option", in: "using=First x=1", rest: "using=First x=1"},
		{name: "leading comparison is not an option", in: "a==b x=1", word: "a==b", rest: " x=1"},
		// A stray quote must not swallow the options behind it.
		{name: "unterminated quote", in: `"Create using=First`, word: `"Create`, rest: " using=First"},
		{name: "empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			word, rest := CutName(tt.in)
			if word != tt.word || rest != tt.rest {
				t.Fatalf("CutName(%q) = (%q, %q), want (%q, %q)", tt.in, word, rest, tt.word, tt.rest)
			}
		})
	}
}

// Quote and CutName have to be exact inverses or a rendered document stops
// reading back as what was written.
func TestQuoteRoundTripsThroughCutName(t *testing.T) {
	t.Parallel()

	values := []string{
		"Create",
		"Create Account",
		`Say "hi"`,
		`Path\Name`,
		"a=b",
		"'Quoted'",
		"Tab\tName",
		"a==b",
		"  padded  ",
		"héllo wörld",
	}
	for _, want := range values {
		line := Quote(want) + " using=First"
		got, rest := CutName(line)
		if got != want {
			t.Fatalf("CutName(Quote(%q)) = %q, rendered as %q", want, got, line)
		}
		if opts := ParseOptions(rest); opts["using"] != "First" {
			t.Fatalf("options after %q = %v, want using=First", line, opts)
		}
	}
}

func TestQuoteLeavesSafeWordsBare(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"", "Create", "expect.status", "local-port", "200"} {
		if got := Quote(in); got != in {
			t.Fatalf("Quote(%q) = %q, want it left alone", in, got)
		}
	}
}

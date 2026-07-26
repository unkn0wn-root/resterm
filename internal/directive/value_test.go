package directive

import (
	"reflect"
	"testing"
)

func TestParseBool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  string
		want bool
		ok   bool
	}{
		{raw: "yes", want: true, ok: true},
		{raw: "OFF", want: false, ok: true},
		{raw: " 1 ", want: true, ok: true},
		{raw: "maybe", want: false, ok: false},
		{raw: "", want: false, ok: false},
	}
	for _, tt := range tests {
		got, ok := ParseBool(tt.raw)
		if got != tt.want || ok != tt.ok {
			t.Fatalf("ParseBool(%q) = (%t, %t), want (%t, %t)",
				tt.raw, got, ok, tt.want, tt.ok)
		}
	}
}

// Every spelling that parses as false has to read as off, otherwise "@sse no"
// would quietly leave the feature enabled.
func TestIsOffMatchesParseBool(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"false", "f", "0", "no", "off", "FALSE", " no "} {
		if !IsOff(raw) {
			t.Fatalf("IsOff(%q) = false, want true", raw)
		}
	}
	for _, raw := range []string{"true", "t", "1", "yes", "on"} {
		if IsOff(raw) {
			t.Fatalf("IsOff(%q) = true, want false", raw)
		}
	}
	// Not boolean spellings, but feature directives still accept them.
	for _, raw := range []string{"disable", "DISABLED"} {
		if !IsOff(raw) {
			t.Fatalf("IsOff(%q) = false, want true", raw)
		}
	}
	for _, raw := range []string{"", "maybe"} {
		if IsOff(raw) {
			t.Fatalf("IsOff(%q) = true, want false", raw)
		}
	}
}

func TestParseNonNegativeInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  string
		want int
		err  bool
	}{
		{raw: "5", want: 5},
		{raw: " 0 ", want: 0},
		{raw: "-1", err: true},
		{raw: "abc", err: true},
		{raw: "", err: true},
	}
	for _, tt := range tests {
		got, err := ParseNonNegativeInt(tt.raw)
		if (err != nil) != tt.err {
			t.Fatalf("ParseNonNegativeInt(%q) err = %v, want err = %t", tt.raw, err, tt.err)
		}
		if err == nil && got != tt.want {
			t.Fatalf("ParseNonNegativeInt(%q) = %d, want %d", tt.raw, got, tt.want)
		}
	}
}

func TestSplitCSV(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  []string
	}{
		{input: "a, b ,c", want: []string{"a", "b", "c"}},
		{input: "a,,b", want: []string{"a", "b"}},
		{input: " , "},
		{input: ""},
	}
	for _, tt := range tests {
		if got := SplitCSV(tt.input); !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("SplitCSV(%q) = %#v, want %#v", tt.input, got, tt.want)
		}
	}
}

func TestTrimQuotes(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		`"quoted"`:  "quoted",
		`'quoted'`:  "quoted",
		`"mixed'`:   `"mixed'`,
		`"`:         `"`,
		`""`:        "",
		`unquoted`:  "unquoted",
		`say "hi"`:  `say "hi"`,
		`"a" + "b"`: `a" + "b`,
	}
	for input, want := range tests {
		if got := TrimQuotes(input); got != want {
			t.Fatalf("TrimQuotes(%q) = %q, want %q", input, got, want)
		}
	}
}

// The last two cases are where TrimQuotes gives a different answer: it cannot
// see the escapes, and it strips quotes off text that was never one token.
func TestUnquoteToken(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		`"Create Account"`:      "Create Account",
		`'Create Account'`:      "Create Account",
		`Create`:                "Create",
		`Create Account`:        "Create Account",
		``:                      "",
		`"Create \"Big\" Acct"`: `Create "Big" Acct`,
		`"a" + "b"`:             `"a" + "b"`,
	}
	for input, want := range tests {
		if got := UnquoteToken(input); got != want {
			t.Fatalf("UnquoteToken(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestIsIdent(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"_x1":   true,
		"Token": true,
		"1x":    false,
		"a b":   false,
		"a-b":   false,
		"héllo": false,
		" ":     false,
		"":      false,
	}
	for input, want := range tests {
		if got := IsIdent(input); got != want {
			t.Fatalf("IsIdent(%q) = %t, want %t", input, got, want)
		}
	}
}

// Keys accept the separators identifiers reject, and nothing else.
func TestIsKeyRune(t *testing.T) {
	t.Parallel()

	for _, r := range "abzABZ09_-." {
		if !IsKeyRune(r) {
			t.Fatalf("IsKeyRune(%q) = false, want true", r)
		}
	}
	for _, r := range " /=:\"é" {
		if IsKeyRune(r) {
			t.Fatalf("IsKeyRune(%q) = true, want false", r)
		}
	}
	for _, r := range "-." {
		if IsIdentRune(r) {
			t.Fatalf("IsIdentRune(%q) = true, want false", r)
		}
	}
}

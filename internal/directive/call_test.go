package directive

import (
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want Call
		ok   bool
	}{
		{
			name: "canonical",
			text: "@assert:   response.status == 200",
			want: Call{
				Name:      Assert,
				Spelling:  Assert,
				Args:      "response.status == 200",
				ArgOffset: 11,
			},
			ok: true,
		},
		{
			name: "semantic alias",
			text: "@skip-if value",
			want: Call{Name: When, Spelling: SkipIf, Args: "value", ArgOffset: 9},
			ok:   true,
		},
		{
			name: "protocol alias",
			text: "@graphql-query query GetUser",
			want: Call{
				Name:      Query,
				Spelling:  GraphQLQuery,
				Args:      "query GetUser",
				ArgOffset: 15,
			},
			ok: true,
		},
		{
			name: "space after marker",
			text: "@  ASSERT: \tvalue",
			want: Call{
				Name:      Assert,
				Spelling:  Assert,
				Args:      "value",
				ArgOffset: 12,
			},
			ok: true,
		},
		{
			name: "colon without a space",
			text: "@assert:x",
			want: Call{Name: Assert, Spelling: Assert, Args: "x", ArgOffset: 8},
			ok:   true,
		},
		{
			name: "repeated colons",
			text: "@assert::x",
			want: Call{Name: Assert, Spelling: Assert, Args: "x", ArgOffset: 9},
			ok:   true,
		},
		{
			name: "no argument",
			text: "@no-log",
			want: Call{Name: NoLog, Spelling: NoLog, ArgOffset: 7},
			ok:   true,
		},
		{
			name: "unknown",
			text: "@future value",
			want: Call{Name: "future", Spelling: "future", Args: "value", ArgOffset: 8},
			ok:   true,
		},
		{name: "missing marker", text: "assert value"},
		{name: "missing name", text: "@   "},
		{name: "empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := Parse(tt.text)
			if ok != tt.ok {
				t.Fatalf("Parse(%q) ok = %t, want %t", tt.text, ok, tt.ok)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Parse(%q) = %#v, want %#v", tt.text, got, tt.want)
			}
		})
	}
}

// The editor turns ArgOffset into a source column, so it has to be the byte
// index where Args begins.
func TestParseArgOffsetLandsOnArgs(t *testing.T) {
	t.Parallel()

	texts := []string{
		"@assert:   response.status == 200",
		"@name héllo wörld",
		"@  desc: \t multi word text",
		"@when x",
	}
	for _, text := range texts {
		call, ok := Parse(text)
		if !ok {
			t.Fatalf("Parse(%q) ok = false", text)
		}
		if got := text[call.ArgOffset:]; got != call.Args {
			t.Fatalf("Parse(%q) text[%d:] = %q, want %q", text, call.ArgOffset, got, call.Args)
		}
	}
}

// The editor relies on this matching what Parse skips between the name and Args.
func TestIsArgSep(t *testing.T) {
	t.Parallel()

	//   is a non-breaking space and 　 an ideographic space. Both are
	// easy to paste in by accident and both have to read as a separator.
	for _, r := range ": \t\r\n 　" {
		if !IsArgSep(r) {
			t.Fatalf("IsArgSep(%q) = false, want true", r)
		}
	}
	for _, r := range "a0-_.=\"" {
		if IsArgSep(r) {
			t.Fatalf("IsArgSep(%q) = true, want false", r)
		}
	}
}

// A pasted non-breaking space used to end up glued to the front of the value.
func TestParseTreatsUnicodeSpaceAsSeparator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
	}{
		{name: "non-breaking space", text: "@auth bearer tok"},
		{name: "ideographic space", text: "@auth　bearer tok"},
		{name: "colon then non-breaking space", text: "@auth: bearer tok"},
		{name: "leading and trailing", text: "@auth  bearer tok "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := Parse(tt.text)
			if !ok {
				t.Fatalf("Parse(%q) ok = false", tt.text)
			}
			if got.Name != Auth {
				t.Fatalf("Parse(%q) name = %q, want %q", tt.text, got.Name, Auth)
			}
			if got.Args != "bearer tok" {
				t.Fatalf("Parse(%q) args = %q, want %q", tt.text, got.Args, "bearer tok")
			}
			if tail := tt.text[got.ArgOffset:]; !strings.HasPrefix(tail, "bearer") {
				t.Fatalf("Parse(%q) offset %d lands on %q", tt.text, got.ArgOffset, tail)
			}
		})
	}
}

func TestCutToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		tok   string
		rest  string
	}{
		{name: "two tokens", input: "  File Office  ", tok: "File", rest: "Office"},
		{name: "single token", input: "office", tok: "office"},
		{name: "case and colon kept", input: "Name: value", tok: "Name:", rest: "value"},
		{name: "empty"},
		{name: "blank", input: " \t "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tok, rest := CutToken(tt.input)
			if tok != tt.tok || rest != tt.rest {
				t.Fatalf("CutToken(%q) = (%q, %q), want (%q, %q)",
					tt.input, tok, rest, tt.tok, tt.rest)
			}
		})
	}
}

func TestCutKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		key   string
		rest  string
	}{
		{name: "lowercased", input: "TIMEOUT 5s", key: "timeout", rest: "5s"},
		{name: "trailing colon dropped", input: "timeout: 5s", key: "timeout", rest: "5s"},
		{name: "key only", input: "  Persist ", key: "persist"},
		{name: "empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			key, rest := CutKey(tt.input)
			if key != tt.key || rest != tt.rest {
				t.Fatalf("CutKey(%q) = (%q, %q), want (%q, %q)",
					tt.input, key, rest, tt.key, tt.rest)
			}
		})
	}
}

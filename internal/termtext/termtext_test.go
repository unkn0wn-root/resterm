package termtext

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func TestRow(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "ordinary text is unchanged",
			value: `fixtures/a b\"quote/users.http`,
			want:  `fixtures/a b\"quote/users.http`,
		},
		{
			name:  "zero width joiner keeps emoji sequences whole",
			value: "\U0001f468\u200d\U0001f469\u200d\U0001f467.http",
			want:  "\U0001f468\u200d\U0001f469\u200d\U0001f467.http",
		},
		{
			name:  "zero width non joiner keeps persian words readable",
			value: "mi\u200cravad.http",
			want:  "mi\u200cravad.http",
		},
		{
			name:  "private use glyph from a patched font survives",
			value: "\ue0b0icons.http",
			want:  "\ue0b0icons.http",
		},
		{
			name:  "combining marks stay with their base letter",
			value: "cafe\u0301 \u0915\u094d\u0937",
			want:  "cafe\u0301 \u0915\u094d\u0937",
		},
		{
			name:  "line controls preserve surrounding graphics",
			value: "a\\\"\r\n\tb/\U0001f468\u200d\U0001f469",
			want:  `a\"\r\n\tb/` + "\U0001f468\u200d\U0001f469",
		},
		{name: "terminal escape", value: "a\x1b[2Jb", want: `a\u001b[2Jb`},
		{name: "C1 control", value: "a\u009bJb", want: `a\u009bJb`},
		{name: "delete", value: "a\x7fb", want: `a\u007fb`},
		{name: "line separator", value: "a\u2028b", want: `a\u2028b`},
		{name: "paragraph separator", value: "a\u2029b", want: `a\u2029b`},
		{name: "bidi override", value: "a\u202eb", want: `a\u202eb`},
		{name: "bidi isolate", value: "a\u2066b", want: `a\u2066b`},
		{name: "arabic letter mark", value: "a\u061cb", want: `a\u061cb`},
		{name: "left to right mark", value: "a\u200eb", want: `a\u200eb`},
		{name: "right to left mark", value: "a\u200fb", want: `a\u200fb`},
		{name: "soft hyphen", value: "a\u00adb", want: `a\u00adb`},
		{name: "zero width space", value: "a\u200bb", want: `a\u200bb`},
		{name: "word joiner", value: "a\u2060b", want: `a\u2060b`},
		{name: "byte order mark", value: "a\ufeffb", want: `a\ufeffb`},
		{name: "mongolian vowel separator", value: "a\u180eb", want: `a\u180eb`},
		{name: "interlinear annotation", value: "a\ufff9b", want: `a\ufff9b`},
		{name: "language tag", value: "a\U000E0001b", want: `a\udb40\udc01b`},
		{name: "invalid UTF-8", value: string([]byte{'a', 0xff, 'b'}), want: `a\xffb`},
		{name: "invalid UTF-8 after a prepended sign", value: "\u0600\xff", want: `\u0600\xff`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Row(tt.value); got != tt.want {
				t.Fatalf("Row(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestBlockNormalizesLineBreaksAndTabs(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "line feed", value: "first\nsecond", want: "first\nsecond"},
		{name: "CRLF", value: "first\r\nsecond", want: "first\nsecond"},
		{name: "tab", value: "name\tvalue", want: "name    value"},
		{name: "carriage return", value: "first\rsecond", want: `first\rsecond`},
		{name: "backspace", value: "ab\bc", want: `ab\bc`},
		{name: "vertical tab", value: "a\vb", want: `a\u000bb`},
		{name: "nul", value: "a\x00b", want: `a\u0000b`},
		{
			name:  "utf-8 read as latin-1 leaves a soft hyphen",
			value: "tecnolog\u00c3\u00ada",
			want:  "tecnolog\u00c3" + `\u00ad` + "a",
		},
		{
			name:  "emoji sequence",
			value: "\U0001f469\u200d\U0001f4bb",
			want:  "\U0001f469\u200d\U0001f4bb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Block(tt.value); got != tt.want {
				t.Fatalf("Block(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestKeepsAttachedRunes(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		// Scotland: a waving black flag and the tag characters spelling gbsct.
		{"emoji tag sequence", "\U0001f3f4\U000e0067\U000e0062\U000e0073\U000e0063\U000e0074\U000e007f"},
		{"variation selector", "\u2699\ufe0f"},
		{"arabic number sign", "\u0600\u0661\u0662"},
		{"zero width joiner", "\U0001f469\u200d\U0001f4bb"},
		{"combining acute", "cafe\u0301"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Row(tt.value); got != tt.value {
				t.Errorf("Row quoted an attached rune: %q", got)
			}
			if got := Block(tt.value); got != tt.value {
				t.Errorf("Block quoted an attached rune: %q", got)
			}
		})
	}
}

func FuzzEscape(f *testing.F) {
	for _, seed := range []string{
		"ordinary text", "\u00ad", "\r\n", "\u0600\xff", "\x1b[2J", "👩‍💻", "a\tb",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		for _, body := range []bool{false, true} {
			format := Row
			if body {
				format = Block
			}
			out := format(input)
			if !utf8.ValidString(out) {
				t.Fatalf("body=%v: invalid UTF-8 remains in %q", body, out)
			}
			if strings.ContainsFunc(out, func(r rune) bool {
				return unicode.IsControl(r) && (!body || r != '\n')
			}) {
				t.Fatalf("body=%v: terminal control remains in %q", body, out)
			}
			if again := format(out); again != out {
				t.Fatalf("body=%v: escaping is not idempotent: %q -> %q", body, out, again)
			}
		}
	})
}

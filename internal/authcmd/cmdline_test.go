package authcmd

import (
	"slices"
	"strings"
	"testing"
)

func TestSplitCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		line string
		want []string
	}{
		{name: "words", line: "gh auth token", want: []string{"gh", "auth", "token"}},
		{name: "extra whitespace", line: " gh \t auth\ntoken ", want: []string{"gh", "auth", "token"}},
		{name: "single quotes", line: `mycli --name 'a b "c"'`, want: []string{"mycli", "--name", `a b "c"`}},
		{name: "double quotes", line: `mycli "a 'b' c"`, want: []string{"mycli", "a 'b' c"}},
		{name: "escaped double quote", line: `mycli "say \"hi\""`, want: []string{"mycli", `say "hi"`}},
		{name: "quote joins word", line: `mycli --flag="a b"`, want: []string{"mycli", "--flag=a b"}},
		{name: "empty quoted argument", line: `mycli '' x`, want: []string{"mycli", "", "x"}},
		{name: "escaped space", line: `mycli a\ b`, want: []string{"mycli", "a b"}},
		{name: "single quotes keep backslash", line: `mycli 'a\"b'`, want: []string{"mycli", `a\"b`}},
		{name: "windows path", line: `C:\tools\gh.exe auth token`, want: []string{`C:\tools\gh.exe`, "auth", "token"}},
		{name: "trailing backslash", line: `mycli a\`, want: []string{"mycli", `a\`}},
		{
			name: "shell syntax stays literal",
			line: `mycli 'a; rm x' $(id) | tee *`,
			want: []string{"mycli", "a; rm x", "$(id)", "|", "tee", "*"},
		},
		{
			name: "template with spaces",
			line: `gcloud --project {{ gcp.project }} --account={{= vars.get("acct") }}`,
			want: []string{"gcloud", "--project", "{{ gcp.project }}", `--account={{= vars.get("acct") }}`},
		},
		{name: "template inside quotes", line: `mycli "{{ a }} b"`, want: []string{"mycli", "{{ a }} b"}},
		{name: "unclosed template", line: `mycli {{ a b`, want: []string{"mycli", "{{", "a", "b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := splitCommand(tt.line)
			if err != nil {
				t.Fatalf("splitCommand(%q) error = %v", tt.line, err)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("splitCommand(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestSplitCommandRejectsUnterminatedQuote(t *testing.T) {
	t.Parallel()

	for _, line := range []string{`gh "auth`, `gh 'auth`, `gh "a\"`} {
		_, err := splitCommand(line)
		if err == nil || !strings.Contains(err.Error(), "unterminated") {
			t.Fatalf("splitCommand(%q) error = %v, want unterminated quote", line, err)
		}
	}
}

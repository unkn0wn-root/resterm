package cli

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/parser"
)

func TestParseRunDocMasksDeclaredSecrets(t *testing.T) {
	const bad = "# @apply {query: {k: \"d4-s3c\"}\n"
	tests := []struct {
		name   string
		src    string
		masked bool
	}{
		{"request secret", "### A\n# @request-secret tok d4-s3c\n" + bad + "GET https://api.example.com/x\n", true},
		{"file secret", "# @file-secret fs d4-s3c\n\n### A\n" + bad + "GET https://api.example.com/x\n", true},
		{"global secret", "# @global-secret gs d4-s3c\n\n### A\n" + bad + "GET https://api.example.com/x\n", true},
		{
			"other request",
			"### A\n" + bad + "GET https://api.example.com/x\n\n### B\n# @request-secret tok d4-s3c\nGET https://api.example.com/y\n",
			true,
		},
		{"not declared", "### A\n" + bad + "GET https://api.example.com/x\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRunDoc(RunSource{Path: "d4.http", Data: []byte(tt.src)})
			if err == nil {
				t.Fatal("ParseRunDoc returned no error")
			}
			shown := err.Error() + "\n" + diag.Render(err)
			if tt.masked == strings.Contains(shown, "d4-s3c") || tt.masked != strings.Contains(shown, "•••") {
				t.Errorf("masked = %t, error:\n%s", tt.masked, shown)
			}
			if !strings.Contains(shown, "@apply is missing a closing") {
				t.Errorf("error lost its message:\n%s", shown)
			}
		})
	}
}

func TestBuildRunRequestChoicesMasksDeclaredSecrets(t *testing.T) {
	src := "### A\n# @request-secret tok d4-s3c\nGET https://api.example.com/x?k=d4-s3c\n\n" +
		"### B\n# @name b d4-s3c\nGET https://api.example.com/y\n\n" +
		"### C\nGET https://api.example.com/plain\n"
	choices := BuildRunRequestChoices(parser.Parse("d4.http", []byte(src)))
	if len(choices) != 3 {
		t.Fatalf("choices = %+v, want three", choices)
	}
	for _, ch := range choices[:2] {
		if text := ch.Name + " " + ch.Target + " " + ch.Label; strings.Contains(text, "d4-s3c") ||
			!strings.Contains(text, "•••") {
			t.Errorf("choice %+v shows the secret", ch)
		}
	}
	if want := "GET https://api.example.com/plain"; choices[2].Label != want {
		t.Errorf("label = %q, want %q", choices[2].Label, want)
	}
}

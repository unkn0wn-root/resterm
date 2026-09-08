package termtext_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/termtext"
)

func TestBlockKeepsJSONParseable(t *testing.T) {
	for _, value := range []string{
		"del \u007f here",
		"soft \u00ad hyphen",
		"zero \u200b width",
		"tag \U000E0001 character",
		"bidi \u202e override",
		"c1 \u009b control",
	} {
		body := `{"d":"` + value + `"}`
		if !json.Valid([]byte(body)) {
			t.Fatalf("fixture is not valid JSON to begin with: %q", body)
		}
		escaped := termtext.Block(body)
		if !json.Valid([]byte(escaped)) {
			t.Errorf("escaping broke JSON: %q -> %q", body, escaped)
		}
		if strings.ContainsAny(escaped, "\u007f\u00ad\u200b\U000E0001\u202e\u009b") {
			t.Errorf("escaped text still holds an unprintable rune: %q", escaped)
		}
		var out struct{ D string }
		if err := json.Unmarshal([]byte(escaped), &out); err != nil {
			t.Errorf("escaped body does not decode: %v", err)
			continue
		}
		if out.D != value {
			t.Errorf("escaping changed the value: %q -> %q", value, out.D)
		}
	}
}

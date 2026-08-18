package ui

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestCompareHistoryResultMasksRuntimeSecrets(t *testing.T) {
	const secret = "ui-compare-runtime-secret"

	model := newTestModelWithDoc("### probe\n# @name probe\nGET http://example.test\n")
	req := &restfile.Request{
		Method:   "GET",
		URL:      "http://example.test",
		Metadata: restfile.RequestMetadata{Name: "probe"},
	}
	result := compareResult{
		Environment:    "dev",
		Request:        req,
		RequestText:    "GET http://example.test\nX-Token: " + secret,
		RuntimeSecrets: []string{secret},
		Response: &httpx.Response{
			StatusCode: 200,
			Body:       []byte(`{"echo":"` + secret + `"}`),
		},
	}

	entry := model.buildCompareHistoryResult(result)
	for field, text := range map[string]string{
		"request text": entry.RequestText,
		"body snippet": entry.BodySnippet,
	} {
		if strings.Contains(text, secret) {
			t.Fatalf("%s contains the plaintext secret: %s", field, text)
		}
	}
}

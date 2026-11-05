package request

import (
	"strings"
	"testing"

	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/redact"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// Nothing else stops a secret variable from reaching the explain output, so
// check every field the report carries, not just the obvious headers.
func TestRedactExplainReportMasksSecretsAndSensitiveHeaders(t *testing.T) {
	t.Parallel()

	eng := newTestEngine()
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com?token=top-secret",
		Variables: []restfile.Variable{
			{Name: "token", Value: "top-secret", Secret: true},
		},
	}
	rep := &xplain.Report{
		Name:     "top-secret request",
		Method:   "GET",
		URL:      "https://example.com?token=top-secret",
		Decision: "using top-secret",
		Failure:  "top-secret failed",
		Vars: []xplain.Var{
			{Name: "token", Source: "request", Value: "top-secret"},
		},
		Stages: []xplain.Stage{
			{
				Name:    "auth",
				Status:  xplain.StageOK,
				Summary: "set top-secret auth",
				Changes: []xplain.Change{
					{Field: "header.authorization", Before: "", After: "Bearer top-secret"},
				},
				Notes: []string{"note top-secret"},
			},
		},
		Final: &xplain.Final{
			Method: "GET",
			URL:    "https://example.com?token=top-secret",
			Headers: []xplain.Header{
				{Name: "Authorization", Value: "Bearer top-secret"},
				{Name: "X-Trace", Value: "top-secret"},
			},
			Body:     "top-secret",
			BodyNote: "body top-secret",
			Settings: []xplain.Pair{
				{Key: "cert", Value: "top-secret"},
			},
			Route: &xplain.Route{
				Kind:    "ssh",
				Summary: "top-secret host",
				Notes:   []string{"note top-secret"},
			},
		},
		Warnings: []string{"warn top-secret"},
	}

	got := eng.redactExplainReport(rep, nil, req, testEnv("").Resolve(), vars.Globals{})
	if got.Final == nil {
		t.Fatal("expected final report section")
	}
	// The authorization header is masked because of its name, X-Trace because
	// its value matches a secret variable.
	if got.Final.Headers[0].Value != redact.Mask {
		t.Fatalf("authorization header = %q, want %q", got.Final.Headers[0].Value, redact.Mask)
	}
	if got.Final.Headers[1].Value != redact.Mask {
		t.Fatalf("secret header value = %q, want %q", got.Final.Headers[1].Value, redact.Mask)
	}

	for _, field := range []string{
		got.Name, got.Method, got.URL, got.Decision, got.Failure,
		got.Vars[0].Value,
		got.Stages[0].Summary, got.Stages[0].Changes[0].After, got.Stages[0].Notes[0],
		got.Final.URL, got.Final.Body, got.Final.BodyNote,
		got.Final.Settings[0].Value,
		got.Final.Route.Summary, got.Final.Route.Notes[0],
		got.Warnings[0],
	} {
		if strings.Contains(field, "top-secret") {
			t.Fatalf("secret survived redaction in %q", field)
		}
	}
}

func TestRedactExplainReportMasksJSONBody(t *testing.T) {
	req := &restfile.Request{Variables: []restfile.Variable{{Name: "token", Value: "a/b9", Secret: true}}}
	tests := []struct {
		body string
		want string
	}{
		{`{"token":"a\/b9","note":"keep / this"}`, `{"token":"•••","note":"keep / this"}`},
		{`{"token":"other","n":1}`, `{"token":"other","n":1}`},
		{"token=a%2Fb9&n=1", "token=•••&n=1"},
	}
	for _, tt := range tests {
		rep := &xplain.Report{Final: &xplain.Final{Body: tt.body}}
		got := newTestEngine().redactExplainReport(rep, nil, req, testEnv("").Resolve(), vars.Globals{})
		if got.Final.Body != tt.want {
			t.Errorf("body %q = %q, want %q", tt.body, got.Final.Body, tt.want)
		}
	}
}

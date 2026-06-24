package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// Directive evaluation is delegated to the request engine, so a @when error must
// carry the same source diagnostics in the TUI as it does headless/CLI.
func TestEvalConditionErrorCarriesSource(t *testing.T) {
	model := New(Config{})
	doc := &restfile.Document{
		Path: "sample.http",
		Raw:  []byte("### Req\n# @when missing.value\nGET https://example.com\n"),
	}
	req := &restfile.Request{}
	spec := &restfile.ConditionSpec{Expression: "missing.value", Line: 2}

	_, _, err := model.evalCondition(context.Background(), doc, req, "", "", spec, nil, nil)
	if err == nil {
		t.Fatalf("expected @when condition error")
	}

	out := diag.Render(err)
	checks := []string{
		`undefined name "missing"`,
		"--> sample.http:2",
		"2 | # @when missing.value", // source snippet, attached via the engine
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Fatalf("expected rendered @when error to contain %q:\n%s", want, out)
		}
	}
}

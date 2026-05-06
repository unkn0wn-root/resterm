package request

import (
	"context"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
	engcfg "github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/parser"
)

func TestRunRTSPreRequestErrorRendersInlineSource(t *testing.T) {
	eng := New(engcfg.Config{}, nil)
	src := `### RTS
# @rts pre-request
> request.setHeader("X", missing.value)
GET https://example.com
`
	doc := parser.Parse("sample.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}

	_, err := eng.runRTSPreRequest(
		context.Background(),
		doc,
		doc.Requests[0],
		"",
		"",
		nil,
		nil,
	)
	if err == nil {
		t.Fatalf("expected rts error")
	}

	out := diag.Render(diag.WrapAs(diag.ClassScript, err, "pre-request rts script"))
	checks := []string{
		`error[script]: undefined name "missing"`,
		"--> sample.http:3:26",
		`   3 | > request.setHeader("X", missing.value)`,
		"Stack:",
		"  at sample.http:3:1 in @script pre-request",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Fatalf("expected rendered error to contain %q:\n%s", want, out)
		}
	}
}

package request

import (
	"context"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
	engcfg "github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/mock"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

type testMockInspector struct{}

func (testMockInspector) Count(context.Context, mock.RequestPattern) (uint64, error) {
	return 0, nil
}

func TestRTSExtraClonesCallerValues(t *testing.T) {
	e := New(engcfg.Config{MockInspector: testMockInspector{}}, nil)
	src := map[string]rts.Value{"custom": rts.Str("original")}

	got := e.rtsExtra(src)

	if value := got["custom"].S; value != "original" {
		t.Fatalf("custom value = %q, want %q", value, "original")
	}
	if _, ok := got["mock"]; !ok {
		t.Fatal("mock value is missing")
	}
	if _, ok := src["mock"]; ok {
		t.Fatal("rtsExtra mutated its source map")
	}
	got["custom"] = rts.Str("changed")
	if value := src["custom"].S; value != "original" {
		t.Fatalf("source value = %q after output mutation, want %q", value, "original")
	}

	if got := e.rtsExtra(nil); len(got) != 1 {
		t.Fatalf("rtsExtra(nil) has %d values, want the mock value only", len(got))
	}
}

func TestRTSExtraPreservesCallerMockValue(t *testing.T) {
	e := New(engcfg.Config{MockInspector: testMockInspector{}}, nil)
	src := map[string]rts.Value{"mock": rts.Str("loop item")}

	got := e.rtsExtra(src)

	if value := got["mock"]; value.K != rts.VStr || value.S != "loop item" {
		t.Fatalf("mock value = %+v, want caller value", value)
	}
	got["mock"] = rts.Str("changed")
	if value := src["mock"].S; value != "loop item" {
		t.Fatalf("source mock value = %q after output mutation, want %q", value, "loop item")
	}
}

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

func TestEvalForEachErrorCarriesSource(t *testing.T) {
	eng := New(engcfg.Config{SourceDiagnostics: true}, nil)
	src := `### Req
# @for-each item in missing.value
GET https://example.com
`
	doc := parser.Parse("sample.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}

	_, err := eng.EvalForEachItems(
		context.Background(),
		doc,
		doc.Requests[0],
		"",
		"",
		ForEachSpec{Expr: "missing.value", Line: 2},
		nil,
		nil,
	)
	if err == nil {
		t.Fatalf("expected for-each error")
	}

	out := diag.Render(err)
	checks := []string{
		`error[script]: undefined name "missing"`,
		"--> sample.http:2:1",
		"   2 | # @for-each item in missing.value", // source snippet, attached via rtsErr
		"in @for-each missing.value",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Fatalf("expected rendered for-each error to contain %q:\n%s", want, out)
		}
	}
}

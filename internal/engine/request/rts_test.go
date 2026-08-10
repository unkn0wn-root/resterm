package request

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
	engcfg "github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/mock"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"google.golang.org/grpc/codes"
)

type testMockInspector struct{}

func (testMockInspector) Count(context.Context, mock.RequestPattern) (uint64, error) {
	return 0, nil
}

func TestRTSExtensionsBindMockAndYieldToLocals(t *testing.T) {
	e := New(engcfg.Config{MockInspector: testMockInspector{}}, nil)
	pos := rts.Pos{Path: "test", Line: 1, Col: 1}

	bound, err := e.re.Eval(context.Background(), rts.RT{Extensions: e.rtsExtensions()}, "mock", pos)
	if err != nil {
		t.Fatalf("eval mock: %v", err)
	}
	if bound.K != rts.VObj {
		t.Fatalf("mock = %+v, want the inspector object", bound)
	}

	rt := rts.RT{Extensions: e.rtsExtensions(), Locals: rts.Local("mock", rts.Str("loop item"))}
	shadowed, err := e.re.Eval(context.Background(), rt, "mock", pos)
	if err != nil {
		t.Fatalf("eval shadowed mock: %v", err)
	}
	if shadowed.K != rts.VStr || shadowed.S != "loop item" {
		t.Fatalf("mock = %+v, want the loop item to win", shadowed)
	}
}

func TestRTSExtensionsAreEmptyWithoutAnInspector(t *testing.T) {
	e := New(engcfg.Config{}, nil)
	pos := rts.Pos{Path: "test", Line: 1, Col: 1}

	_, err := e.re.Eval(context.Background(), rts.RT{Extensions: e.rtsExtensions()}, "mock", pos)
	if err == nil || !strings.Contains(err.Error(), `undefined name "mock"`) {
		t.Fatalf("eval mock without an inspector: error = %v, want it undefined", err)
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
		testEnv(""),
		"",
		nil,
		rts.Locals{},
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

func TestRunRTSPreRequestRejectsResponse(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "member",
			body: `request.setHeader("X", response.statusCode)`,
		},
		{
			name: "member behind try",
			body: `request.setHeader("X", (try response.statusCode).value ?? "none")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := "### RTS\n# @rts pre-request\n> " + tt.body + "\nGET https://example.com\n"
			doc := parser.Parse("sample.http", []byte(src))
			if len(doc.Errors) != 0 {
				t.Fatalf("parse errors: %+v", doc.Errors)
			}

			eng := New(engcfg.Config{}, nil)
			_, err := eng.runRTSPreRequest(
				context.Background(),
				doc,
				doc.Requests[0],
				testEnv(""),
				"",
				nil,
				rts.Locals{},
				nil,
			)
			if err == nil {
				t.Fatal("expected response to be rejected before the request")
			}
			if !strings.Contains(err.Error(), "use last for the previous response") {
				t.Fatalf("error = %v, want it to name last", err)
			}
		})
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
		testEnv(""),
		"",
		ForEachSpec{Expr: "missing.value", Line: 2},
		nil,
		rts.Locals{},
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

func TestRTSGRPCExposesEncodedTrailers(t *testing.T) {
	raw := string([]byte{0x00, 0x01, 0xff})
	resp := &grpcx.Response{
		StatusCode:    codes.InvalidArgument,
		StatusMessage: "invalid page size",
		Headers:       map[string][]string{"x-shared": {"header"}},
		Trailers: map[string][]string{
			"x-shared":  {"trailer"},
			"trace-bin": {raw},
		},
	}

	h := rtsGRPC(resp).H
	if got := h["x-shared"]; len(got) != 1 || got[0] != "header" {
		t.Fatalf("x-shared = %q, want the header value to survive the trailer", got)
	}
	if got := h["Grpc-Trailer-x-shared"]; len(got) != 1 || got[0] != "trailer" {
		t.Fatalf("Grpc-Trailer-x-shared = %q, want the trailer under its prefix", got)
	}
	want := base64.RawStdEncoding.EncodeToString([]byte(raw))
	if got := h["Grpc-Trailer-trace-bin"]; len(got) != 1 || got[0] != want {
		t.Fatalf("Grpc-Trailer-trace-bin = %q, want %q", got, want)
	}
}

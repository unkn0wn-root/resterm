package prerequest

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestNormalize(t *testing.T) {
	body := "body"
	out := Output{
		Headers: http.Header{},
		Query:   map[string]*string{},
		Body:    &body,
	}

	Normalize(&out)
	if out.Headers != nil || out.Query != nil {
		t.Fatalf("expected empty collections to be nil: %#v", out)
	}
	if out.Body == nil || *out.Body != body {
		t.Fatalf("expected body to be preserved: %#v", out.Body)
	}
}

func TestApplyRemovesDeclaredHeaders(t *testing.T) {
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com",
		Headers: http.Header{
			"X-Declared": {"declared"},
			"X-Replaced": {"declared"},
		},
	}

	var out Output
	out.DelHeader("X-Declared")
	out.SetHeader(diag.Pos{}, "X-Replaced", "script")
	out.DelHeader("X-Replaced")
	out.SetHeader(diag.Pos{}, "X-Replaced", "final")

	if err := Apply(req, out); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got, ok := req.Headers["X-Declared"]; ok {
		t.Fatalf("expected the removed header to be gone, got %#v", got)
	}
	if got := req.Headers.Get("X-Replaced"); got != "final" {
		t.Fatalf("X-Replaced = %q, want %q", got, "final")
	}
}

func TestApplyPreservesTemplatedURL(t *testing.T) {
	req := &restfile.Request{
		Method: "GET",
		URL:    "{{base_url}}/anything",
	}

	mode, pre := "debug", "1"
	err := Apply(req, Output{Query: map[string]*string{"mode": &mode, "pre": &pre}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !strings.Contains(req.URL, "{{base_url}}") {
		t.Fatalf("expected templated base_url preserved, got %q", req.URL)
	}
	if strings.Contains(req.URL, "%7B%7B") || strings.Contains(req.URL, "%7D%7D") {
		t.Fatalf("expected template braces to remain unescaped, got %q", req.URL)
	}
	if !strings.Contains(req.URL, "mode=debug") || !strings.Contains(req.URL, "pre=1") {
		t.Fatalf("expected merged query params, got %q", req.URL)
	}
}

func TestApplyMarksRuntimeWritesAsData(t *testing.T) {
	req := &restfile.Request{
		Method:  "GET",
		URL:     "https://example.com",
		Headers: http.Header{"X-Declared": {"{{token}}"}},
	}
	url, body := "https://example.com/{{token}}", "{{token}}"
	out := Output{URL: &url, Body: &body}
	out.SetHeader(diag.Pos{}, "X-Script", "{{token}}-{{$randomInt(7, 7)}}")

	if err := Apply(req, out); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !req.Written.URL || !req.Written.Body {
		t.Fatalf("written = %+v, want the URL and body marked", req.Written)
	}
	if got := req.Headers.Get("X-Script"); got != "{{token}}-7" {
		t.Fatalf("X-Script = %q, want helpers rendered and the name left as written", got)
	}
	if !req.Written.Header("X-Script", "{{token}}-7") || req.Written.Header("X-Declared", "{{token}}") {
		t.Fatalf("written headers = %v, want only the script header", req.Written.Headers)
	}
}

func TestOutputWarnings(t *testing.T) {
	at := func(line int) diag.Pos { return diag.Pos{Path: "a.http", Line: line, Col: 3} }
	var out Output
	out.SetURL(at(1), "{{base}}/users?id={{$uuid}}")
	out.SetHeader(at(2), "Authorization", "Bearer {{old}}")
	out.SetHeader(at(3), "authorization", "Bearer {{api.token}}")
	out.AddHeader(at(4), "Accept", "{{accept}}")
	out.SetHeader(at(5), "X-Gone", "{{gone}}")
	out.DelHeader("X-Gone")
	out.SetQuery(at(6), "page", "{{page}}")
	out.SetHeader(at(7), "X-Id", "{{$uuuid}}-{{$randomInt(9, 1)}}")
	out.SetBody(diag.Pos{}, `{"n": {{= 1 + 1 }}}`)

	want := []string{
		`a.http:1: Script sends {{base}} in the URL as written. Use vars.get("base") or vars.interpolate().`,
		`a.http:3: Script sends {{api.token}} in header Authorization as written. Use vars.get("api.token") or vars.interpolate().`,
		`a.http:4: Script sends {{accept}} in header Accept as written. Use vars.get("accept") or vars.interpolate().`,
		`a.http:6: Script sends {{page}} in query param page as written. Use vars.get("page") or vars.interpolate().`,
		`a.http:7: Script sends {{$uuuid}} in header X-Id as written. Check the helper name, or use vars.get("$uuuid").`,
		"Script sends {{= 1 + 1 }} in the body as written. Write the expression without {{= }}.",
	}
	if got := out.Warnings(); !slices.Equal(got, want) {
		t.Fatalf("Warnings =\n%q\nwant\n%q", got, want)
	}
}

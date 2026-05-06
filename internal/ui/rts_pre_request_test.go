package ui

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/prerequest"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestRunRTSPreRequestMutations(t *testing.T) {
	model := New(Config{})
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/path?seed=1",
		Headers: http.Header{
			"X-Base": []string{"A"},
		},
		LineRange: restfile.LineRange{Start: 1, End: 6},
		Metadata: restfile.RequestMetadata{
			Scripts: []restfile.ScriptBlock{{
				Kind: "pre-request",
				Lang: "rts",
				Body: `request.setHeader("X-Test", "1")
request.addHeader("X-Test", "2")
request.setQueryParam("user", "alice")
request.setBody("payload")
vars.set("token", "abc")
request.setHeader("X-Secret", vars.global.get("secret"))
vars.global.set("newglobal", "ng", false)
vars.global.delete("old")`,
			}},
		},
	}
	vars := map[string]string{"seed": "value"}
	globals := map[string]prerequest.GlobalValue{
		"secret": {Name: "Secret", Value: "top", Secret: true},
		"old":    {Name: "old", Value: "gone"},
	}

	out, err := model.runRTSPreRequest(context.Background(), nil, req, "", "", vars, globals)
	if err != nil {
		t.Fatalf("runRTSPreRequest: %v", err)
	}

	if got := out.Headers.Values("X-Test"); len(got) != 2 || got[0] != "1" || got[1] != "2" {
		t.Fatalf("expected X-Test header values [1 2], got %#v", got)
	}
	if got := out.Headers.Get("X-Secret"); got != "top" {
		t.Fatalf("expected X-Secret header to use secret global, got %q", got)
	}
	if got := out.Query["user"]; got != "alice" {
		t.Fatalf("expected query user=alice, got %q", got)
	}
	if out.Body == nil || *out.Body != "payload" {
		t.Fatalf("expected body payload, got %#v", out.Body)
	}
	if got := out.Variables["token"]; got != "abc" {
		t.Fatalf("expected output vars token=abc, got %q", got)
	}
	if got := vars["token"]; got != "abc" {
		t.Fatalf("expected vars map token=abc, got %q", got)
	}
	if gv, ok := out.Globals["newglobal"]; !ok || gv.Value != "ng" || gv.Secret {
		t.Fatalf("expected newglobal=ng (non-secret), got %#v", gv)
	}
	if gv, ok := out.Globals["old"]; !ok || !gv.Delete {
		t.Fatalf("expected old to be marked deleted, got %#v", gv)
	}
}

func TestRunRTSPreRequestPreservesTemplatedURL(t *testing.T) {
	model := New(Config{})
	req := &restfile.Request{
		Method:    "POST",
		URL:       "{{base_url}}/anything",
		LineRange: restfile.LineRange{Start: 1, End: 5},
		Metadata: restfile.RequestMetadata{
			Scripts: []restfile.ScriptBlock{{
				Kind: "pre-request",
				Lang: "rts",
				Body: `request.setURL(query.merge(request.url, {mode: "debug", pre: "1"}))
request.setQueryParam("mutated", "true")`,
			}},
		},
	}

	out, err := model.runRTSPreRequest(context.Background(), nil, req, "", "", nil, nil)
	if err != nil {
		t.Fatalf("runRTSPreRequest: %v", err)
	}
	if err := prerequest.Apply(req, out); err != nil {
		t.Fatalf("applyPreRequestOutput: %v", err)
	}

	if !strings.Contains(req.URL, "{{base_url}}") {
		t.Fatalf("expected templated base_url preserved, got %q", req.URL)
	}
	if strings.Contains(req.URL, "%7B%7B") || strings.Contains(req.URL, "%7D%7D") {
		t.Fatalf("expected template braces to remain unescaped, got %q", req.URL)
	}
	if !strings.Contains(req.URL, "mode=debug") || !strings.Contains(req.URL, "mutated=true") {
		t.Fatalf("expected merged query params, got %q", req.URL)
	}
}

func TestRunRTSPreRequestErrorRendersInlineSource(t *testing.T) {
	model := New(Config{})
	src := `### RTS
# @rts pre-request
> request.setHeader("X", missing.value)
GET https://example.com
`
	doc := parser.Parse("sample.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}

	_, err := model.runRTSPreRequest(
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
		"                         ^",
		"Stack:",
		"  at sample.http:3:1 in @script pre-request",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Fatalf("expected rendered error to contain %q:\n%s", want, out)
		}
	}
}

func TestRunRTSPreRequestErrorRendersIncludedSource(t *testing.T) {
	model := New(Config{})
	dir := t.TempDir()
	path := filepath.Join(dir, "pre.rts")
	if err := os.WriteFile(
		path,
		[]byte("request.setHeader(\"X\", missing.value)\n"),
		0o600,
	); err != nil {
		t.Fatalf("write rts file: %v", err)
	}
	req := &restfile.Request{
		Method:    "GET",
		URL:       "https://example.com",
		LineRange: restfile.LineRange{Start: 1, End: 3},
		Metadata: restfile.RequestMetadata{
			Scripts: []restfile.ScriptBlock{{
				Kind:     "pre-request",
				Lang:     "rts",
				FilePath: "pre.rts",
			}},
		},
	}

	_, err := model.runRTSPreRequest(context.Background(), nil, req, "", dir, nil, nil)
	if err == nil {
		t.Fatalf("expected rts error")
	}

	out := diag.Render(diag.WrapAs(diag.ClassScript, err, "pre-request rts script"))
	checks := []string{
		`error[script]: undefined name "missing"`,
		"--> " + path + ":1:24",
		`   1 | request.setHeader("X", missing.value)`,
		"  at " + path + ":1:1 in @script pre-request",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Fatalf("expected rendered error to contain %q:\n%s", want, out)
		}
	}
}

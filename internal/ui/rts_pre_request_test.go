package ui

import (
	"context"
	"net/http"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
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
	globals := map[string]scripts.GlobalValue{
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

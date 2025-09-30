package scripts

import (
	"net/http"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestRunPreRequestScripts(t *testing.T) {
	runner := NewRunner()
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/api",
		Headers: http.Header{
			"User-Agent": {"resterm"},
		},
	}

	scripts := []restfile.ScriptBlock{
		{Kind: "pre-request", Body: `request.setHeader("X-Test", "1"); request.setQueryParam("user", "alice"); vars.set("token", "abc");`},
	}

	out, err := runner.RunPreRequest(scripts, PreRequestInput{Request: req, Variables: map[string]string{"seed": "value"}})
	if err != nil {
		t.Fatalf("pre-request runner: %v", err)
	}
	if out.Headers.Get("X-Test") != "1" {
		t.Fatalf("expected header to be set")
	}
	if out.Query["user"] != "alice" {
		t.Fatalf("expected query param to be set")
	}
	if out.Variables["token"] != "abc" {
		t.Fatalf("expected script variable to be returned")
	}
}

func TestRunTestsScripts(t *testing.T) {
	runner := NewRunner()
	response := &httpclient.Response{
		Status:       "200 OK",
		StatusCode:   200,
		EffectiveURL: "https://example.com/api",
		Duration:     125 * time.Millisecond,
		Headers: http.Header{
			"Content-Type": {"application/json"},
		},
		Body: []byte(`{"ok":true}`),
	}

	scripts := []restfile.ScriptBlock{
		{Kind: "test", Body: `client.test("status", function() { tests.assert(response.statusCode === 200, "status code"); });`},
	}

	results, err := runner.RunTests(scripts, TestInput{Response: response, Variables: map[string]string{}})
	if err != nil {
		t.Fatalf("run tests: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected two test results, got %d", len(results))
	}
	for _, r := range results {
		if !r.Passed {
			t.Fatalf("expected all tests to pass, got %#v", results)
		}
	}
}

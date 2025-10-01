package scripts

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestRunPreRequestScripts(t *testing.T) {
	runner := NewRunner(nil)
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

func TestRunScriptsFromFile(t *testing.T) {
	dir := t.TempDir()
	preScript := "request.setHeader(\"X-File\", \"1\");\nvars.set(\"fromFile\", \"yes\");"
	if err := os.WriteFile(filepath.Join(dir, "pre.js"), []byte(preScript), 0o600); err != nil {
		t.Fatalf("write pre script: %v", err)
	}
	testScript := `client.test("file status", function () {
	  tests.assert(response.statusCode === 201, "status code");
});
client.test("vars carried", function () {
	  tests.assert(vars.get("fromFile") === "yes", "vars should be visible");
});`
	if err := os.WriteFile(filepath.Join(dir, "test.js"), []byte(testScript), 0o600); err != nil {
		t.Fatalf("write test script: %v", err)
	}

	runner := NewRunner(nil)
	req := &restfile.Request{Method: "POST", URL: "https://example.com/api"}
	preBlocks := []restfile.ScriptBlock{{Kind: "pre-request", FilePath: "pre.js"}}
	preResult, err := runner.RunPreRequest(preBlocks, PreRequestInput{Request: req, Variables: map[string]string{}, BaseDir: dir})
	if err != nil {
		t.Fatalf("pre-request file script: %v", err)
	}
	if preResult.Headers.Get("X-File") != "1" {
		t.Fatalf("expected header from file script")
	}
	if preResult.Variables["fromFile"] != "yes" {
		t.Fatalf("expected variable from file script")
	}

	response := &httpclient.Response{
		Status:     "201 Created",
		StatusCode: 201,
		Body:       []byte(`{"ok":true}`),
	}
	testBlocks := []restfile.ScriptBlock{{Kind: "test", FilePath: "test.js"}}
	results, err := runner.RunTests(testBlocks, TestInput{Response: response, Variables: preResult.Variables, BaseDir: dir})
	if err != nil {
		t.Fatalf("test file script: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected four results, got %d", len(results))
	}
	for _, res := range results {
		if !res.Passed {
			t.Fatalf("expected results to pass: %+v", results)
		}
	}
}

func TestRunTestsScripts(t *testing.T) {
	runner := NewRunner(nil)
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

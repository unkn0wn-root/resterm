package scripts

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

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

	response := &Response{
		Kind:   ResponseKindHTTP,
		Status: "201 Created",
		Code:   201,
		Body:   []byte(`{"ok":true}`),
	}
	testBlocks := []restfile.ScriptBlock{{Kind: "test", FilePath: "test.js"}}
	results, globals, err := runner.RunTests(testBlocks, TestInput{Response: response, Variables: preResult.Variables, BaseDir: dir})
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
	if globals != nil {
		t.Fatalf("expected no global changes, got %+v", globals)
	}
}

func TestRunTestsScripts(t *testing.T) {
	runner := NewRunner(nil)
	response := &Response{
		Kind:   ResponseKindHTTP,
		Status: "200 OK",
		Code:   200,
		URL:    "https://example.com/api",
		Time:   125 * time.Millisecond,
		Header: http.Header{
			"Content-Type": {"application/json"},
		},
		Body: []byte(`{"ok":true}`),
	}

	scripts := []restfile.ScriptBlock{
		{Kind: "test", Body: `client.test("status", function() { tests.assert(response.statusCode === 200, "status code"); });`},
	}

	results, globals, err := runner.RunTests(scripts, TestInput{Response: response, Variables: map[string]string{}})
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
	if globals != nil {
		t.Fatalf("expected no global changes, got %+v", globals)
	}
}

func TestRunTestsScriptsStream(t *testing.T) {
	runner := NewRunner(nil)
	response := &Response{
		Kind:   ResponseKindHTTP,
		Status: "101 Switching Protocols",
		Code:   101,
		Header: http.Header{"Content-Type": {"application/json"}},
		Body:   []byte(`{"events":[],"summary":{}}`),
		URL:    "wss://example.com/socket",
	}
	streamInfo := &StreamInfo{
		Kind: "websocket",
		Summary: map[string]interface{}{
			"sentCount":     1,
			"receivedCount": 1,
			"duration":      int64(time.Second),
			"closedBy":      "client",
		},
		Events: []map[string]interface{}{
			{
				"step":      "1:send",
				"direction": "send",
				"type":      "text",
				"text":      "hello",
				"timestamp": "2024-01-01T00:00:00Z",
			},
			{
				"step":      "2:receive",
				"direction": "receive",
				"type":      "text",
				"text":      "hi",
				"timestamp": "2024-01-01T00:00:01Z",
			},
		},
	}

	script := `client.test("stream summary", function () {
	const summary = stream.summary();
	tests.assert(stream.enabled() === true, "stream enabled");
	tests.assert(summary.sentCount === 1, "sent count");
	const events = stream.events();
	tests.assert(events.length === 2, "event length");
});

client.test("stream callbacks", function () {
	let seen = 0;
	stream.onEvent(function (evt) {
		seen += 1;
		tests.assert(evt.type === "text", "event type");
	});
	stream.onClose(function (summary) {
		tests.assert(summary.closedBy === "client", "closed by client");
		tests.assert(seen === 2, "all events replayed");
	});
});

client.test("response stream access", function () {
	const info = response.stream();
	tests.assert(info.enabled === true, "response stream enabled");
	tests.assert(info.summary.sentCount === 1, "response summary count");
});`

	results, globals, err := runner.RunTests([]restfile.ScriptBlock{{Kind: "test", Body: script}}, TestInput{
		Response:  response,
		Variables: map[string]string{},
		Stream:    streamInfo,
	})
	if err != nil {
		t.Fatalf("run stream tests: %v", err)
	}
	if len(results) != 12 {
		t.Fatalf("expected twelve results, got %d", len(results))
	}
	for _, res := range results {
		if !res.Passed {
			t.Fatalf("expected all stream tests to pass, got %+v", results)
		}
	}
	if globals != nil {
		t.Fatalf("expected no global changes, got %+v", globals)
	}
}

func TestPreRequestGlobalSetAndDelete(t *testing.T) {
	runner := NewRunner(nil)
	blocks := []restfile.ScriptBlock{{
		Kind: "pre-request",
		Body: `if (vars.global.get("token") !== "seed") { throw new Error("existing global not visible"); }
vars.global.set("token", "updated", {secret: true});
vars.global.delete("removeMe");`,
	}}
	input := PreRequestInput{
		Request:   &restfile.Request{Method: "GET", URL: "https://example.com"},
		Variables: map[string]string{},
		Globals: map[string]GlobalValue{
			"token":    {Name: "token", Value: "seed"},
			"removeMe": {Name: "removeMe", Value: "gone"},
		},
	}
	out, err := runner.RunPreRequest(blocks, input)
	if err != nil {
		t.Fatalf("pre-request globals: %v", err)
	}
	if out.Globals == nil {
		t.Fatalf("expected globals map to be populated")
	}
	assertGlobal := func(name string, expectDelete bool, expectSecret bool, expectValue string) {
		found := false
		for _, entry := range out.Globals {
			if entry.Name == name {
				found = true
				if entry.Delete != expectDelete {
					t.Fatalf("expected delete=%v for %s, got %v", expectDelete, name, entry.Delete)
				}
				if entry.Secret != expectSecret {
					t.Fatalf("expected secret=%v for %s, got %v", expectSecret, name, entry.Secret)
				}
				if !expectDelete && entry.Value != expectValue {
					t.Fatalf("expected value %q for %s, got %q", expectValue, name, entry.Value)
				}
			}
		}
		if !found {
			t.Fatalf("global %s not found", name)
		}
	}
	assertGlobal("token", false, true, "updated")
	assertGlobal("removeMe", true, false, "")
}

func TestTestScriptsGlobalMutation(t *testing.T) {
	runner := NewRunner(nil)
	resp := &Response{Kind: ResponseKindHTTP, Status: "204", Code: 204}
	scripts := []restfile.ScriptBlock{{
		Kind: "test",
		Body: `client.test("update global", function () {
  tests.assert(vars.global.get("token") === "seed", "seed should be visible");
  vars.global.set("token", "after");
});`,
	}}
	results, globals, err := runner.RunTests(scripts, TestInput{
		Response:  resp,
		Variables: map[string]string{},
		Globals: map[string]GlobalValue{
			"token": {Name: "token", Value: "seed"},
		},
	})
	if err != nil {
		t.Fatalf("test globals: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected two results, got %d", len(results))
	}
	if globals == nil {
		t.Fatalf("expected globals to be returned")
	}
	var updated GlobalValue
	found := false
	for _, entry := range globals {
		if entry.Name == "token" {
			updated = entry
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected updated token entry")
	}
	if updated.Value != "after" {
		t.Fatalf("expected token to be updated, got %q", updated.Value)
	}
	if updated.Secret {
		t.Fatalf("did not expect secret flag")
	}
	if updated.Delete {
		t.Fatalf("did not expect delete flag")
	}
}

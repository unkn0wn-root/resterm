package scripts

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
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
		{
			Kind: "pre-request",
			Body: `request.setHeader("X-Test", "1"); request.setQueryParam("user", "alice"); vars.set("token", "abc");`,
		},
	}

	out, err := runner.RunPreRequest(
		scripts,
		PreRequestInput{Request: req, Variables: map[string]string{"seed": "value"}},
	)
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

func TestRunPreRequestSkipsRTS(t *testing.T) {
	runner := NewRunner(nil)
	req := &restfile.Request{}
	blocks := []restfile.ScriptBlock{{
		Kind: "pre-request",
		Lang: "rts",
		Body: `request.setHeader("X-Test", "1");`,
	}}
	out, err := runner.RunPreRequest(
		blocks,
		PreRequestInput{Request: req, Variables: map[string]string{}},
	)
	if err != nil {
		t.Fatalf("pre-request runner: %v", err)
	}
	if len(out.Headers) != 0 {
		t.Fatalf("expected rts scripts to be skipped, got headers: %#v", out.Headers)
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
	preResult, err := runner.RunPreRequest(
		preBlocks,
		PreRequestInput{Request: req, Variables: map[string]string{}, BaseDir: dir},
	)
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
	results, globals, err := runner.RunTests(
		testBlocks,
		TestInput{Response: response, Variables: preResult.Variables, BaseDir: dir},
	)
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
		{
			Kind: "test",
			Body: `client.test("status", function() { tests.assert(response.statusCode === 200, "status code"); });`,
		},
	}

	results, globals, err := runner.RunTests(
		scripts,
		TestInput{Response: response, Variables: map[string]string{}},
	)
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

	results, globals, err := runner.RunTests(
		[]restfile.ScriptBlock{{Kind: "test", Body: script}},
		TestInput{
			Response:  response,
			Variables: map[string]string{},
			Stream:    streamInfo,
		},
	)
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

func TestResponseAPIUsesWireForBinary(t *testing.T) {
	runner := NewRunner(nil)
	response := &Response{
		Kind:            ResponseKindGRPC,
		Status:          "0 OK",
		Code:            0,
		Header:          http.Header{"Content-Type": {"application/json"}},
		Body:            []byte(`{"ok":true}`),
		Wire:            []byte{0x00, 0xFF},
		WireContentType: "application/grpc+proto",
	}

	script := `client.test("wire preferred", function () {
  tests.assert(response.base64() === "AP8=", "base64 uses wire");
  tests.assert(response.bytes().length === 2, "bytes length");
  tests.assert(response.isBinary === true, "binary flag");
  const js = response.json();
  tests.assert(js.ok === true, "json parsed from body");
});`

	results, globals, err := runner.RunTests(
		[]restfile.ScriptBlock{{Kind: "test", Body: script}},
		TestInput{
			Response:  response,
			Variables: map[string]string{},
		},
	)
	if err != nil {
		t.Fatalf("run tests: %v", err)
	}
	if globals != nil {
		t.Fatalf("expected no globals, got %+v", globals)
	}
	if len(results) != 5 {
		t.Fatalf("expected five results (four asserts + wrapper), got %d", len(results))
	}
	for _, res := range results {
		if !res.Passed {
			t.Fatalf("expected tests to pass, got %+v", results)
		}
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

func TestPreRequestCancellationInterruptsScript(t *testing.T) {
	runner := NewRunner(nil)
	blocks := []restfile.ScriptBlock{{
		Kind: "pre-request",
		Body: `while (true) {}`,
	}}

	ctx, cancel := context.WithCancel(context.Background())
	input := PreRequestInput{
		Request: &restfile.Request{Method: "GET", URL: "https://example.com"},
		Context: ctx,
	}

	done := make(chan struct{})
	var err error
	go func() {
		defer close(done)
		_, err = runner.RunPreRequest(blocks, input)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pre-request cancellation did not return")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
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

func TestTraceBindingProvidesTimeline(t *testing.T) {
	runner := NewRunner(nil)
	t0 := time.Unix(0, 0)
	timeline := &nettrace.Timeline{
		Started:   t0,
		Completed: t0.Add(75 * time.Millisecond),
		Duration:  75 * time.Millisecond,
		Phases: []nettrace.Phase{
			{
				Kind:     nettrace.PhaseDNS,
				Start:    t0,
				End:      t0.Add(5 * time.Millisecond),
				Duration: 5 * time.Millisecond,
				Meta:     nettrace.PhaseMeta{Addr: "example.com", Cached: true},
			},
			{
				Kind:     nettrace.PhaseConnect,
				Start:    t0.Add(5 * time.Millisecond),
				End:      t0.Add(30 * time.Millisecond),
				Duration: 25 * time.Millisecond,
				Meta:     nettrace.PhaseMeta{Addr: "93.184.216.34:443"},
			},
			{
				Kind:     nettrace.PhaseTLS,
				Start:    t0.Add(30 * time.Millisecond),
				End:      t0.Add(45 * time.Millisecond),
				Duration: 15 * time.Millisecond,
			},
			{
				Kind:     nettrace.PhaseReqHdrs,
				Start:    t0.Add(45 * time.Millisecond),
				End:      t0.Add(46 * time.Millisecond),
				Duration: 1 * time.Millisecond,
			},
			{
				Kind:     nettrace.PhaseReqBody,
				Start:    t0.Add(46 * time.Millisecond),
				End:      t0.Add(48 * time.Millisecond),
				Duration: 2 * time.Millisecond,
			},
			{
				Kind:     nettrace.PhaseTTFB,
				Start:    t0.Add(48 * time.Millisecond),
				End:      t0.Add(55 * time.Millisecond),
				Duration: 7 * time.Millisecond,
			},
			{
				Kind:     nettrace.PhaseTransfer,
				Start:    t0.Add(55 * time.Millisecond),
				End:      t0.Add(75 * time.Millisecond),
				Duration: 20 * time.Millisecond,
			},
		},
	}
	spec := &restfile.TraceSpec{
		Enabled: true,
		Budgets: restfile.TraceBudget{
			Total:     60 * time.Millisecond,
			Tolerance: 5 * time.Millisecond,
			Phases: map[string]time.Duration{
				"dns":     5 * time.Millisecond,
				"connect": 15 * time.Millisecond,
			},
		},
	}
	traceInput := NewTraceInput(timeline, spec)
	response := &Response{Kind: ResponseKindHTTP, Status: "200 OK", Code: 200}
	script := `client.test("trace basics", function () {
  tests.assert(trace.enabled() === true, "enabled");
  tests.assert(Math.round(trace.durationMs()) === 75, "duration ms");
  tests.assert(typeof trace.started() === "string", "started string");
  const phases = trace.phases();
  tests.assert(phases.length === 7, "phase count");
  tests.assert(phases[0].meta.cached === true, "dns cached meta");
  const dns = trace.getPhase("dns");
  tests.assert(dns.count === 1, "dns count");
  tests.assert(Math.round(dns.durationMs) === 5, "dns duration");
  const names = trace.phaseNames();
  tests.assert(names.indexOf("dns") !== -1, "names include dns");
  const budgets = trace.budgets();
  tests.assert(budgets.enabled === true, "budgets enabled");
  tests.assert(Math.round(budgets.phases.connect) === 15, "connect budget");
  const breaches = trace.breaches();
  tests.assert(breaches.length === 2, "breaches count");
  tests.assert(trace.withinBudget() === false, "not within budget");
});`

	results, globals, err := runner.RunTests(
		[]restfile.ScriptBlock{{Kind: "test", Body: script}},
		TestInput{
			Response:  response,
			Variables: map[string]string{},
			Trace:     traceInput,
		},
	)
	if err != nil {
		t.Fatalf("trace test script: %v", err)
	}
	if globals != nil {
		t.Fatalf("expected no globals, got %+v", globals)
	}
	if len(results) == 0 {
		t.Fatalf("expected trace script results")
	}
	for _, res := range results {
		if !res.Passed {
			t.Fatalf("trace script failure: %+v", res)
		}
	}
}

func TestTraceBindingDisabled(t *testing.T) {
	runner := NewRunner(nil)
	resp := &Response{Kind: ResponseKindHTTP, Status: "200 OK", Code: 200}
	script := `client.test("trace disabled", function () {
  tests.assert(trace.enabled() === false, "trace disabled");
  tests.assert(trace.breaches().length === 0, "no breaches when disabled");
  tests.assert(trace.withinBudget() === true, "within budget default");
});`
	results, _, err := runner.RunTests(
		[]restfile.ScriptBlock{{Kind: "test", Body: script}},
		TestInput{Response: resp},
	)
	if err != nil {
		t.Fatalf("trace disabled script: %v", err)
	}
	for _, res := range results {
		if !res.Passed {
			t.Fatalf("trace disabled assertion failed: %+v", res)
		}
	}
}

func TestResponseAPIExposesBinaryHelpers(t *testing.T) {
	runner := NewRunner(nil)
	body := []byte{0x00, 0x01, 0x02, 0x03}
	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "body.bin")
	expectedB64 := base64.StdEncoding.EncodeToString(body)

	script := fmt.Sprintf(`client.test("binary helpers", function () {
  tests.assert(response.isBinary === true, "binary flag");
  tests.assert(response.base64() === "%s", "base64 value");
  const bytes = response.bytes();
  tests.assert(bytes.length === 4, "byte length");
  tests.assert(bytes[1] === 1, "byte copy");
  const name = response.filename();
  tests.assert(name && name.length > 0, "filename hint");
  tests.assert(response.saveBody("%s") === true, "save body");
});`, expectedB64, savePath)

	resp := &Response{
		Kind:   ResponseKindHTTP,
		Status: "200 OK",
		Code:   200,
		URL:    "https://example.com/download/file.bin",
		Header: http.Header{
			"Content-Type":        {"application/octet-stream"},
			"Content-Disposition": {`attachment; filename="file.bin"`},
		},
		Body: body,
	}

	results, _, err := runner.RunTests(
		[]restfile.ScriptBlock{{Kind: "test", Body: script}},
		TestInput{Response: resp, Variables: map[string]string{}},
	)
	if err != nil {
		t.Fatalf("binary helpers script: %v", err)
	}
	for _, res := range results {
		if !res.Passed {
			t.Fatalf("binary helpers assertion failed: %+v", res)
		}
	}
	data, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatalf("expected saved file, got error: %v", err)
	}
	if !bytes.Equal(data, body) {
		t.Fatalf("saved body mismatch, got %v want %v", data, body)
	}
}

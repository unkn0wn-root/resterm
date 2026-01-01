package parser

import (
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestParseAuthAndSettings(t *testing.T) {
	src := `# @name Sample
# @auth bearer token-123
# @setting timeout 5s
# @tag smoke critical
# @capture global authToken {{response.json.token}}
GET https://example.com/api
> {% tests.assert(true, "status ok") %}
`

	doc := Parse("sample.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}

	req := doc.Requests[0]
	if req.Metadata.Auth == nil {
		t.Fatalf("expected auth metadata")
	}
	if req.Metadata.Auth.Type != "bearer" {
		t.Fatalf("expected bearer auth, got %s", req.Metadata.Auth.Type)
	}
	if req.Metadata.Auth.Params["token"] != "token-123" {
		t.Fatalf("unexpected bearer token: %q", req.Metadata.Auth.Params["token"])
	}

	if req.Settings["timeout"] != "5s" {
		t.Fatalf("expected timeout setting 5s, got %q", req.Settings["timeout"])
	}

	if len(req.Metadata.Tags) != 2 {
		t.Fatalf("expected two tags, got %v", req.Metadata.Tags)
	}

	if len(req.Metadata.Scripts) != 1 {
		t.Fatalf("expected one script block, got %d", len(req.Metadata.Scripts))
	}
	script := req.Metadata.Scripts[0]
	if script.Kind != "test" {
		t.Fatalf("expected test script, got %s", script.Kind)
	}
	if script.Body == "" {
		t.Fatalf("expected script body to be captured")
	}

	if len(req.Metadata.Captures) != 1 {
		t.Fatalf("expected one capture, got %d", len(req.Metadata.Captures))
	}
	capture := req.Metadata.Captures[0]
	if capture.Scope != restfile.CaptureScopeGlobal {
		t.Fatalf("expected global capture scope, got %v", capture.Scope)
	}
	if capture.Name != "authToken" {
		t.Fatalf("unexpected capture name: %s", capture.Name)
	}
	if capture.Expression != "{{response.json.token}}" {
		t.Fatalf("unexpected capture expression: %q", capture.Expression)
	}
}

func TestParseAssertDirective(t *testing.T) {
	src := `# @assert status == 200
# @assert contains(header("Content-Type"), "json") => "content type"
GET https://example.com/api
`
	doc := Parse("assert.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if len(req.Metadata.Asserts) != 2 {
		t.Fatalf("expected 2 asserts, got %d", len(req.Metadata.Asserts))
	}
	first := req.Metadata.Asserts[0]
	if first.Expression != "status == 200" {
		t.Fatalf("unexpected assert expression: %q", first.Expression)
	}
	if first.Line != 1 {
		t.Fatalf("expected line 1, got %d", first.Line)
	}
	second := req.Metadata.Asserts[1]
	if second.Expression != `contains(header("Content-Type"), "json")` {
		t.Fatalf("unexpected assert expression: %q", second.Expression)
	}
	if second.Message != "content type" {
		t.Fatalf("unexpected assert message: %q", second.Message)
	}
	if second.Line != 2 {
		t.Fatalf("expected line 2, got %d", second.Line)
	}
}

func TestSplitAssertEscapes(t *testing.T) {
	expr, msg := splitAssert(`contains(body, "a=>b") => "ok"`)
	if expr != `contains(body, "a=>b")` {
		t.Fatalf("unexpected expr: %q", expr)
	}
	if msg != "ok" {
		t.Fatalf("unexpected msg: %q", msg)
	}

	expr, msg = splitAssert(`contains(body, "a\"=>b") => "ok"`)
	if expr != `contains(body, "a\"=>b")` {
		t.Fatalf("unexpected escaped expr: %q", expr)
	}
	if msg != "ok" {
		t.Fatalf("unexpected escaped msg: %q", msg)
	}
}

func TestParseScriptLang(t *testing.T) {
	src := `# @script pre-request lang=rts
> request.setHeader("X-Test", "1")
GET https://example.com
`
	doc := Parse("script-lang.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if len(req.Metadata.Scripts) != 1 {
		t.Fatalf("expected 1 script block, got %d", len(req.Metadata.Scripts))
	}
	script := req.Metadata.Scripts[0]
	if script.Kind != "pre-request" {
		t.Fatalf("expected pre-request script, got %s", script.Kind)
	}
	if script.Lang != "rts" {
		t.Fatalf("expected rts lang, got %q", script.Lang)
	}
	if script.Body == "" {
		t.Fatalf("expected script body to be captured")
	}
}

func TestParseApplyDirective(t *testing.T) {
	src := `# @apply {headers: {"X-Test": "1"}}
GET https://example.com
`
	doc := Parse("apply.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if len(req.Metadata.Applies) != 1 {
		t.Fatalf("expected 1 apply directive, got %d", len(req.Metadata.Applies))
	}
	spec := req.Metadata.Applies[0]
	if spec.Expression != `{headers: {"X-Test": "1"}}` {
		t.Fatalf("unexpected apply expression: %q", spec.Expression)
	}
	if spec.Line != 1 {
		t.Fatalf("expected line 1, got %d", spec.Line)
	}
	if spec.Col != 1 {
		t.Fatalf("expected col 1, got %d", spec.Col)
	}
}

func TestFileLevelSettingsCaptured(t *testing.T) {
	src := `# @setting timeout 10s
# @setting http-insecure true

### First
GET https://example.com
`
	doc := Parse("file-settings.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request")
	}
	if doc.Settings == nil {
		t.Fatalf("expected file-level settings to be captured")
	}
	if doc.Settings["timeout"] != "10s" {
		t.Fatalf("expected timeout=10s, got %q", doc.Settings["timeout"])
	}
	if doc.Settings["http-insecure"] != "true" {
		t.Fatalf("expected http-insecure=true, got %q", doc.Settings["http-insecure"])
	}
}

func TestSettingsDirectiveMultiAssignFileScope(t *testing.T) {
	src := `# @settings timeout=10s http-insecure=true
# @settings proxy=http://one

### First
GET https://example.com
`
	doc := Parse("file-settings.http", []byte(src))
	if doc.Settings == nil {
		t.Fatalf("expected settings to be captured")
	}
	if doc.Settings["timeout"] != "10s" {
		t.Fatalf("expected timeout=10s, got %q", doc.Settings["timeout"])
	}
	if doc.Settings["http-insecure"] != "true" {
		t.Fatalf("expected http-insecure=true, got %q", doc.Settings["http-insecure"])
	}
	if doc.Settings["proxy"] != "http://one" {
		t.Fatalf("expected proxy=http://one, got %q", doc.Settings["proxy"])
	}
}

func TestSettingsDirectiveRequestScopeOverride(t *testing.T) {
	src := `# @settings timeout=9s followredirects=true

### First
# @name req1
# @setting proxy http://legacy
# @settings timeout=3s proxy=http://req followredirects=false
GET https://example.com
`
	doc := Parse("request-settings.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if req.Settings["timeout"] != "3s" {
		t.Fatalf("expected request timeout=3s, got %q", req.Settings["timeout"])
	}
	if req.Settings["proxy"] != "http://req" {
		t.Fatalf("expected request proxy override, got %q", req.Settings["proxy"])
	}
	if req.Settings["followredirects"] != "false" {
		t.Fatalf("expected request followredirects=false, got %q", req.Settings["followredirects"])
	}
	if doc.Settings["timeout"] != "9s" {
		t.Fatalf("expected file timeout=9s, got %q", doc.Settings["timeout"])
	}
	if doc.Settings["followredirects"] != "true" {
		t.Fatalf("expected file followredirects=true, got %q", doc.Settings["followredirects"])
	}
}

func TestParseGlobalDirectiveWhitespaceValue(t *testing.T) {
	src := `# @global base_url https://httpbin.org
# @global alt_url: https://alt.example.com
GET https://example.com
`

	doc := Parse("globals.http", []byte(src))

	if len(doc.Globals) != 2 {
		t.Fatalf("expected 2 globals, got %d", len(doc.Globals))
	}

	values := make(map[string]string)
	for _, gv := range doc.Globals {
		values[gv.Name] = gv.Value
	}

	if values["base_url"] != "https://httpbin.org" {
		t.Fatalf("expected base_url to be https://httpbin.org, got %q", values["base_url"])
	}

	if values["alt_url"] != "https://alt.example.com" {
		t.Fatalf("expected alt_url to be https://alt.example.com, got %q", values["alt_url"])
	}
}

func TestParseScopedVariableCommentAliases(t *testing.T) {
	src := `# @file api.base https://example.com
# @file-secret api.token secret-file

### WithRequest
# @request request.id 42
# @request-secret request.token secret-req
GET https://example.com
`

	doc := Parse("scopes-aliases.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}

	fileVars := make(map[string]restfile.Variable)
	for _, v := range doc.Variables {
		fileVars[v.Name] = v
	}
	if v, ok := fileVars["api.base"]; !ok || v.Scope != restfile.ScopeFile || v.Secret {
		t.Fatalf("expected file api.base as non-secret file var, got %#v", v)
	}
	if v, ok := fileVars["api.token"]; !ok || v.Scope != restfile.ScopeFile || !v.Secret {
		t.Fatalf("expected file-secret api.token, got %#v", v)
	}

	req := doc.Requests[0]
	reqVars := make(map[string]restfile.Variable)
	for _, v := range req.Variables {
		reqVars[v.Name] = v
	}
	if v, ok := reqVars["request.id"]; !ok || v.Scope != restfile.ScopeRequest || v.Secret {
		t.Fatalf("expected request id as non-secret request var, got %#v", v)
	}
	if v, ok := reqVars["request.token"]; !ok || v.Scope != restfile.ScopeRequest || !v.Secret {
		t.Fatalf("expected request-secret token, got %#v", v)
	}
}

func TestParseShorthandSecretVariables(t *testing.T) {
	src := `@global-secret auth.token super-secret
@file-secret base.url https://secret.example.com

### InlineRequest
@request-secret trace.id {{$uuid}}
GET https://example.com
`

	doc := Parse("shorthand-secret.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}

	if len(doc.Globals) != 1 || doc.Globals[0].Name != "auth.token" || !doc.Globals[0].Secret {
		t.Fatalf("expected one secret global, got %#v", doc.Globals)
	}

	if len(doc.Variables) != 1 || doc.Variables[0].Name != "base.url" || !doc.Variables[0].Secret ||
		doc.Variables[0].Scope != restfile.ScopeFile {
		t.Fatalf("expected one secret file variable, got %#v", doc.Variables)
	}

	req := doc.Requests[0]
	if len(req.Variables) != 1 {
		t.Fatalf("expected one request variable, got %d", len(req.Variables))
	}
	if rv := req.Variables[0]; rv.Name != "trace.id" || rv.Scope != restfile.ScopeRequest ||
		!rv.Secret {
		t.Fatalf("unexpected request var: %#v", rv)
	}
}

func TestParseConstDirectives(t *testing.T) {
	t.Parallel()

	src := `# @const svc.http http://localhost:8080
# @const greeting Hello World

GET {{svc.http}}/status
`

	doc := Parse("const.http", []byte(src))
	if doc == nil {
		t.Fatalf("expected document")
	}
	if len(doc.Constants) != 2 {
		t.Fatalf("expected 2 constants, got %d", len(doc.Constants))
	}
	consts := make(map[string]restfile.Constant)
	for _, c := range doc.Constants {
		consts[c.Name] = c
	}
	if got := consts["svc.http"].Value; got != "http://localhost:8080" {
		t.Fatalf("expected svc.http to be http://localhost:8080, got %q", got)
	}
	if got := consts["greeting"].Value; got != "Hello World" {
		t.Fatalf("expected greeting to be Hello World, got %q", got)
	}
}

func TestParseSSHGlobalProfile(t *testing.T) {
	src := `# @ssh global edge host=10.0.0.5 user=ops key=~/.ssh/id_ed25519 port=2022 known_hosts=/tmp/kh strict_hostkey=false persist timeout=30s keepalive=15s retries=2
GET http://example.com
`
	doc := Parse("ssh.http", []byte(src))
	if len(doc.SSH) != 1 {
		t.Fatalf("expected 1 ssh profile, got %d", len(doc.SSH))
	}

	prof := doc.SSH[0]
	if prof.Scope != restfile.SSHScopeGlobal {
		t.Fatalf("expected global scope, got %v", prof.Scope)
	}
	if prof.Name != "edge" {
		t.Fatalf("expected name edge, got %q", prof.Name)
	}
	if prof.Host != "10.0.0.5" {
		t.Fatalf("unexpected host %q", prof.Host)
	}
	if prof.Port != 2022 || prof.PortStr != "2022" {
		t.Fatalf("unexpected port %d (%q)", prof.Port, prof.PortStr)
	}
	if prof.User != "ops" {
		t.Fatalf("unexpected user %q", prof.User)
	}
	if prof.Key == "" {
		t.Fatalf("expected key to be set")
	}
	if prof.KnownHosts != "/tmp/kh" {
		t.Fatalf("unexpected known_hosts %q", prof.KnownHosts)
	}
	if !prof.Strict.Set || prof.Strict.Val {
		t.Fatalf("expected strict_hostkey=false to be set")
	}
	if !prof.Persist.Set || !prof.Persist.Val {
		t.Fatalf("expected persist to be set true")
	}
	if !prof.Timeout.Set || prof.TimeoutStr != "30s" {
		t.Fatalf("timeout not captured, val=%v raw=%q", prof.Timeout.Val, prof.TimeoutStr)
	}
	if !prof.KeepAlive.Set || prof.KeepAliveStr != "15s" {
		t.Fatalf("keepalive not captured, val=%v raw=%q", prof.KeepAlive.Val, prof.KeepAliveStr)
	}
	if !prof.Retries.Set || prof.RetriesStr != "2" {
		t.Fatalf("retries not captured")
	}
}

func TestParseSSHRequestUseOverride(t *testing.T) {
	src := `### jump
# @ssh use=edge host={{api_host}} strict_hostkey=false retries=3
GET http://{{api_host}}
`
	doc := Parse("ssh_req.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if req.SSH == nil {
		t.Fatalf("expected ssh spec on request")
	}
	if req.SSH.Use != "edge" {
		t.Fatalf("unexpected use %q", req.SSH.Use)
	}
	if req.SSH.Inline == nil {
		t.Fatalf("expected inline overrides")
	}
	inline := req.SSH.Inline
	if inline.Scope != restfile.SSHScopeRequest {
		t.Fatalf("expected request scope inline, got %v", inline.Scope)
	}
	if inline.Host != "{{api_host}}" {
		t.Fatalf("unexpected inline host %q", inline.Host)
	}
	if !inline.Strict.Set || inline.Strict.Val {
		t.Fatalf("expected strict_hostkey=false")
	}
	if !inline.Retries.Set || inline.RetriesStr != "3" {
		t.Fatalf("expected retries override")
	}
}

func TestParseSSHRequestIgnoresPersist(t *testing.T) {
	src := `### jump
# @ssh request host=1.2.3.4 user=ops persist keepalive=1s
GET http://example.com
`
	doc := Parse("ssh_req_persist.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if req.SSH == nil || req.SSH.Inline == nil {
		t.Fatalf("expected inline ssh profile")
	}
	if req.SSH.Inline.Persist.Set {
		t.Fatalf("request persist should be ignored")
	}
}

func TestParseSSHWithGRPCRequest(t *testing.T) {
	src := `### grpc over ssh
# @ssh use=jump
# @grpc test.Inventory/Seed
GRPC passthrough:///grpc-internal:8082

{}
`
	doc := Parse("grpc_ssh.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if req.GRPC == nil {
		t.Fatalf("expected grpc request to be parsed")
	}
	if req.SSH == nil {
		t.Fatalf("expected ssh spec on grpc request")
	}
	if req.SSH.Use != "jump" {
		t.Fatalf("unexpected ssh use %q", req.SSH.Use)
	}
}

func TestParseRequestVarDirectiveVariants(t *testing.T) {
	src := `# @name Vars
# @var simple foo
# @var equals key=value
# @var colon key: value
# @var url https://example.com:8443/path
GET https://example.com
`

	doc := Parse("vars.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if len(req.Variables) != 4 {
		t.Fatalf("expected 4 request variables, got %d", len(req.Variables))
	}

	vals := make(map[string]string)
	for _, v := range req.Variables {
		vals[v.Name] = v.Value
	}

	checks := map[string]string{
		"simple": "foo",
		"equals": "key=value",
		"colon":  "key: value",
		"url":    "https://example.com:8443/path",
	}
	for name, expected := range checks {
		if vals[name] != expected {
			t.Fatalf("expected %s=%q, got %q", name, expected, vals[name])
		}
	}
}

func TestShorthandBeforeMethodDefaultsToFileScope(t *testing.T) {
	src := `### One
@id abc
GET https://example.com
Header: value
`

	doc := Parse("shorthand-position.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request")
	}
	if len(doc.Variables) != 1 {
		t.Fatalf("expected 1 file variable, got %d", len(doc.Variables))
	}
	v := doc.Variables[0]
	if v.Name != "id" || v.Value != "abc" || v.Scope != restfile.ScopeFile {
		t.Fatalf("unexpected file variable %+v", v)
	}
	if len(doc.Requests[0].Variables) != 0 {
		t.Fatalf("expected no request variables, got %d", len(doc.Requests[0].Variables))
	}
}

func TestShorthandAfterBodyStaysRequestScoped(t *testing.T) {
	src := `### reset
POST https://example.com
Content-Type: application/json

{"ok":true}
@tail outside
@tail2 outside-2
`

	doc := Parse("shorthand-tail.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if len(req.Variables) != 2 {
		t.Fatalf("expected 2 request variables, got %d", len(req.Variables))
	}
	if len(doc.Variables) != 0 {
		t.Fatalf("expected no file variables, got %d", len(doc.Variables))
	}
	values := map[string]string{}
	for _, v := range req.Variables {
		if v.Scope != restfile.ScopeRequest {
			t.Fatalf("expected %s to be request scoped, got %v", v.Name, v.Scope)
		}
		values[v.Name] = v.Value
	}
	if values["tail"] != "outside" || values["tail2"] != "outside-2" {
		t.Fatalf("unexpected request vars: %+v", values)
	}
}

func TestShorthandAfterHeaderWithoutBodyStaysRequestScoped(t *testing.T) {
	src := `GET https://example.com

@tail outside
`

	doc := Parse("shorthand-tail-nobody.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if len(req.Variables) != 1 {
		t.Fatalf("expected 1 request variable, got %d", len(req.Variables))
	}
	if len(doc.Variables) != 0 {
		t.Fatalf("expected no file variables, got %d", len(doc.Variables))
	}
	v := req.Variables[0]
	if v.Name != "tail" || v.Value != "outside" || v.Scope != restfile.ScopeRequest {
		t.Fatalf("unexpected request var %+v", v)
	}
}

func TestTrailingShorthandAfterFirstRequestDoesNotBleedIntoSecond(t *testing.T) {
	src := `### One
GET https://example.com

@fileVar shared

### Two
GET https://example.com/2
@reqVar inside
`

	doc := Parse("shorthand-multi.http", []byte(src))
	if len(doc.Requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(doc.Requests))
	}
	req1 := doc.Requests[0]
	req2 := doc.Requests[1]

	if len(doc.Variables) != 0 {
		t.Fatalf("expected no file variables, got %d", len(doc.Variables))
	}

	if len(req1.Variables) != 1 {
		t.Fatalf("expected 1 request var in first request, got %d", len(req1.Variables))
	}
	if req1.Variables[0].Name != "fileVar" || req1.Variables[0].Scope != restfile.ScopeRequest {
		t.Fatalf("unexpected var in first request: %+v", req1.Variables[0])
	}

	if len(req2.Variables) != 1 {
		t.Fatalf("expected 1 request var in second request, got %d", len(req2.Variables))
	}
	if req2.Variables[0].Name != "reqVar" || req2.Variables[0].Scope != restfile.ScopeRequest {
		t.Fatalf("unexpected var in second request: %+v", req2.Variables[0])
	}
}

func TestShorthandRequestScopeExplicit(t *testing.T) {
	src := `### One
@request requestId abc123
GET https://example.com
`

	doc := Parse("shorthand-request.http", []byte(src))
	if len(doc.Variables) != 0 {
		t.Fatalf("expected no file variables, got %d", len(doc.Variables))
	}
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request")
	}
	req := doc.Requests[0]
	if len(req.Variables) != 1 {
		t.Fatalf("expected 1 request variable, got %d", len(req.Variables))
	}
	v := req.Variables[0]
	if v.Name != "requestId" || v.Value != "abc123" {
		t.Fatalf("unexpected request variable %q=%q", v.Name, v.Value)
	}
	if v.Scope != restfile.ScopeRequest {
		t.Fatalf("expected requestId to be request scoped, got %v", v.Scope)
	}
}

func TestExplicitRequestVarNotMovedAfterBody(t *testing.T) {
	src := `POST https://example.com

{}
@request keep me
`

	doc := Parse("shorthand-request-tail.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request")
	}
	req := doc.Requests[0]
	if len(req.Variables) != 1 {
		t.Fatalf("expected 1 request variable, got %d", len(req.Variables))
	}
	if len(doc.Variables) != 0 {
		t.Fatalf("expected no file variables, got %d", len(doc.Variables))
	}
	v := req.Variables[0]
	if v.Name != "keep" || v.Value != "me" || v.Scope != restfile.ScopeRequest {
		t.Fatalf("unexpected request variable %+v", v)
	}
}

func TestParseCaptureDirectiveGlobal(t *testing.T) {
	src := `# @name Capture
# @capture global auth.token {{response.json.json.token}}
GET https://example.com
`

	doc := Parse("capture.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if len(req.Metadata.Captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(req.Metadata.Captures))
	}
	cap := req.Metadata.Captures[0]
	if cap.Scope != restfile.CaptureScopeGlobal {
		t.Fatalf("expected global capture scope, got %v", cap.Scope)
	}
	if cap.Name != "auth.token" {
		t.Fatalf("expected capture name auth.token, got %q", cap.Name)
	}
	if cap.Expression != "{{response.json.json.token}}" {
		t.Fatalf("unexpected capture expression %q", cap.Expression)
	}
}

func TestParseOAuth2AuthSpec(t *testing.T) {
	spec := parseAuthSpec(
		`oauth2 token_url="https://auth.example.com/token" client_id=my-client client_secret="s3cr3t" scope="read write" grant=password username=jane password=pwd client_auth=body audience=https://api.example.com`,
	)
	if spec == nil {
		t.Fatalf("expected oauth2 spec")
	}
	if spec.Type != "oauth2" {
		t.Fatalf("unexpected auth type %q", spec.Type)
	}
	checks := map[string]string{
		"token_url":     "https://auth.example.com/token",
		"client_id":     "my-client",
		"client_secret": "s3cr3t",
		"scope":         "read write",
		"grant":         "password",
		"username":      "jane",
		"password":      "pwd",
		"client_auth":   "body",
		"audience":      "https://api.example.com",
	}
	for key, expected := range checks {
		if spec.Params[key] != expected {
			t.Fatalf("expected %s=%q, got %q", key, expected, spec.Params[key])
		}
	}
}

func TestParseOAuth2AuthSpecCacheOnly(t *testing.T) {
	spec := parseAuthSpec(`oauth2 cache_key=github`)
	if spec == nil {
		t.Fatalf("expected oauth2 spec for cache-only directive")
	}
	if spec.Type != "oauth2" {
		t.Fatalf("unexpected auth type %q", spec.Type)
	}
	if spec.Params["cache_key"] != "github" {
		t.Fatalf("expected cache_key to be github, got %q", spec.Params["cache_key"])
	}
	if spec.Params["token_url"] != "" {
		t.Fatalf("expected empty token_url, got %q", spec.Params["token_url"])
	}
	if spec.Params["grant"] != "client_credentials" {
		t.Fatalf("expected default grant client_credentials, got %q", spec.Params["grant"])
	}
	if spec.Params["client_auth"] != "basic" {
		t.Fatalf("expected default client_auth basic, got %q", spec.Params["client_auth"])
	}
}

func TestParseCompareDirective(t *testing.T) {
	src := `# @name Compare
# @compare dev stage prod base=stage
GET https://example.com/health
`

	doc := Parse("compare.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("expected no parse errors, got %v", doc.Errors)
	}
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if req.Metadata.Compare == nil {
		t.Fatalf("expected compare metadata")
	}
	spec := req.Metadata.Compare
	expect := []string{"dev", "stage", "prod"}
	if len(spec.Environments) != len(expect) {
		t.Fatalf("unexpected environments: %#v", spec.Environments)
	}
	for idx, env := range expect {
		if spec.Environments[idx] != env {
			t.Fatalf("expected env %q at position %d, got %q", env, idx, spec.Environments[idx])
		}
	}
	if spec.Baseline != "stage" {
		t.Fatalf("expected baseline stage, got %q", spec.Baseline)
	}
}

func TestParseCompareDirectiveErrors(t *testing.T) {
	src := `# @name Compare
# @compare dev dev
GET https://example.com
`

	doc := Parse("compare.http", []byte(src))
	if len(doc.Errors) == 0 {
		t.Fatalf("expected parse errors")
	}
	req := doc.Requests[0]
	if req.Metadata.Compare != nil {
		t.Fatalf("expected compare metadata to be nil on error")
	}
}

func TestParseMultiLineScripts(t *testing.T) {
	src := `# @name Scripted
# @script pre-request
> const token = vars.get("token");
> request.setHeader("X-Debug", token);

# @script test
> tests["status"] = () => {
>   tests.assert(response.statusCode === 200, "status code");
> };
GET https://example.com/api
`

	doc := Parse("scripted.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if len(req.Metadata.Scripts) != 2 {
		t.Fatalf("expected 2 script blocks, got %d", len(req.Metadata.Scripts))
	}
	pre := req.Metadata.Scripts[0]
	if pre.Kind != "pre-request" {
		t.Fatalf("expected pre-request script, got %s", pre.Kind)
	}
	expectedPre := "const token = vars.get(\"token\");\nrequest.setHeader(\"X-Debug\", token);"
	if pre.Body != expectedPre {
		t.Fatalf("unexpected pre-request script body: %q", pre.Body)
	}
	testBlock := req.Metadata.Scripts[1]
	if testBlock.Kind != "test" {
		t.Fatalf("expected test script, got %s", testBlock.Kind)
	}
	if strings.Count(testBlock.Body, "\n") != 2 {
		t.Fatalf("expected multi-line script body, got %q", testBlock.Body)
	}
	for _, fragment := range []string{"tests[\"status\"] = () => {", "tests.assert(response.statusCode === 200, \"status code\");", "};"} {
		if !strings.Contains(testBlock.Body, fragment) {
			t.Fatalf("expected test script body to contain %q, got %q", fragment, testBlock.Body)
		}
	}
}

func TestParseScriptFileInclude(t *testing.T) {
	src := `# @name FileScript
# @script test
> < ./scripts/validation.js
GET https://example.com/api
`

	doc := Parse("filescript.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if len(req.Metadata.Scripts) != 1 {
		t.Fatalf("expected 1 script block, got %d", len(req.Metadata.Scripts))
	}
	script := req.Metadata.Scripts[0]
	if script.FilePath != "./scripts/validation.js" {
		t.Fatalf("unexpected script file path: %q", script.FilePath)
	}
	if script.Body != "" {
		t.Fatalf("expected script body to be empty for file include, got %q", script.Body)
	}
}

func TestParseScriptFileIncludeWithIndent(t *testing.T) {
	src := `# @script test
>     < ./script.js
GET https://example.com`

	doc := Parse("indent.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if len(req.Metadata.Scripts) != 1 {
		t.Fatalf("expected 1 script block, got %d", len(req.Metadata.Scripts))
	}
	if req.Metadata.Scripts[0].FilePath != "./script.js" {
		t.Fatalf("unexpected script file path: %q", req.Metadata.Scripts[0].FilePath)
	}
}

func TestParseProfileDirective(t *testing.T) {
	src := `### Timed
# @profile count=5 warmup=2 delay=250ms
GET https://example.com/api
`

	doc := Parse("profile.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if req.Metadata.Profile == nil {
		t.Fatalf("expected profile metadata to be parsed")
	}
	prof := req.Metadata.Profile
	if prof.Count != 5 {
		t.Fatalf("expected count=5, got %d", prof.Count)
	}
	if prof.Warmup != 2 {
		t.Fatalf("expected warmup=2, got %d", prof.Warmup)
	}
	if prof.Delay != 250*time.Millisecond {
		t.Fatalf("expected delay=250ms, got %s", prof.Delay)
	}
}

func TestParseBodyExpandDirective(t *testing.T) {
	src := `### ExpandBody
# @body expand
POST https://example.com/api

< ./payload.json
`

	doc := Parse("body-expand.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if !req.Body.Options.ExpandTemplates {
		t.Fatalf("expected expand templates flag to be set")
	}
	if req.Body.FilePath != "./payload.json" {
		t.Fatalf("unexpected file path %q", req.Body.FilePath)
	}
}

func TestParseWorkflowDirectives(t *testing.T) {
	src := `# @workflow provision-account on-failure=continue
# @description Provision new account flow
# @tag smoke regression
# @step Authenticate using=AuthLogin expect.status="200 OK"
# @step CreateProfile using=CreateUser on-failure=stop vars.request.name={{vars.global.username}} expect.status="201 Created"
# @step Audit using=AuditLog capture=global.auditId

### AuthLogin
GET https://example.com/auth

### CreateUser
POST https://example.com/users

### AuditLog
GET https://example.com/audit
`

	doc := Parse("workflow.http", []byte(src))
	if len(doc.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(doc.Workflows))
	}
	workflow := doc.Workflows[0]
	if workflow.Name != "provision-account" {
		t.Fatalf("unexpected workflow name %q", workflow.Name)
	}
	if workflow.DefaultOnFailure != restfile.WorkflowOnFailureContinue {
		t.Fatalf("expected default on-failure=continue, got %s", workflow.DefaultOnFailure)
	}
	if workflow.Description == "" ||
		!strings.Contains(workflow.Description, "Provision new account flow") {
		t.Fatalf("expected workflow description, got %q", workflow.Description)
	}
	if len(workflow.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %v", workflow.Tags)
	}
	if len(workflow.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(workflow.Steps))
	}
	step0 := workflow.Steps[0]
	if step0.Using != "AuthLogin" {
		t.Fatalf("expected first step to use AuthLogin, got %q", step0.Using)
	}
	if step0.Expect["status"] != "200 OK" {
		t.Fatalf("expected first step expect.status=200 OK, got %q", step0.Expect["status"])
	}
	step1 := workflow.Steps[1]
	if step1.OnFailure != restfile.WorkflowOnFailureStop {
		t.Fatalf("expected second step on-failure=stop, got %s", step1.OnFailure)
	}
	varsKey := "vars.request.name"
	if step1.Vars[varsKey] != "{{vars.global.username}}" {
		t.Fatalf("expected %s override, got %q", varsKey, step1.Vars[varsKey])
	}
	if step1.Expect["status"] != "201 Created" {
		t.Fatalf("expected quoted status value, got %q", step1.Expect["status"])
	}
	step2 := workflow.Steps[2]
	if step2.Options["capture"] != "global.auditId" {
		t.Fatalf("expected capture option propagated, got %v", step2.Options)
	}
	if workflow.LineRange.Start != 1 {
		t.Fatalf("expected workflow start line 1, got %d", workflow.LineRange.Start)
	}
	if workflow.LineRange.End < workflow.LineRange.Start {
		t.Fatalf("invalid workflow line range: %#v", workflow.LineRange)
	}
	if len(doc.Requests) != 3 {
		t.Fatalf("expected 3 requests parsed, got %d", len(doc.Requests))
	}
}

func TestParseBlockComments(t *testing.T) {
	src := `/**
 * @name Blocked
 * @tag smoke regression
 */
GET https://example.org
`

	doc := Parse("block.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if req.Metadata.Name != "Blocked" {
		t.Fatalf("expected name from block comment, got %q", req.Metadata.Name)
	}
	if len(req.Metadata.Tags) != 2 {
		t.Fatalf("expected tags from block comment, got %v", req.Metadata.Tags)
	}
	if req.Metadata.Tags[0] != "smoke" || req.Metadata.Tags[1] != "regression" {
		t.Fatalf("unexpected tags: %v", req.Metadata.Tags)
	}
}

func TestParseScannerError(t *testing.T) {
	line := strings.Repeat("a", maxScanToken+1)
	doc := Parse("long.http", []byte(line))
	if len(doc.Errors) == 0 {
		t.Fatalf("expected parse errors for oversized line")
	}
	if !strings.Contains(doc.Errors[0].Message, "line exceeds") {
		t.Fatalf("unexpected error message: %q", doc.Errors[0].Message)
	}
	if doc.Errors[0].Line != 1 {
		t.Fatalf("expected error at line 1, got %d", doc.Errors[0].Line)
	}
}

func TestParseGraphQLRequest(t *testing.T) {
	src := `# @name GraphQLExample
# @graphql
# @operation FetchUser
POST https://example.com/graphql

query FetchUser($id: ID!) {
  user(id: $id) {
    id
    name
  }
}

# @variables
{
  "id": "123"
}
`

	doc := Parse("graphql.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if req.Body.GraphQL == nil {
		t.Fatalf("expected GraphQL body")
	}
	gql := req.Body.GraphQL
	if gql.OperationName != "FetchUser" {
		t.Fatalf("unexpected operation name: %q", gql.OperationName)
	}
	if !strings.Contains(gql.Query, "user(id: $id)") {
		t.Fatalf("expected query body, got %q", gql.Query)
	}
	if strings.TrimSpace(gql.Variables) == "" {
		t.Fatalf("expected variables to be captured")
	}
	if !strings.Contains(gql.Variables, "\"id\": \"123\"") {
		t.Fatalf("expected variables json to contain id, got %q", gql.Variables)
	}
	if strings.Contains(gql.Query, "# @variables") {
		t.Fatalf("expected directives stripped from query")
	}
}

func TestParseGraphQLDisableResetsState(t *testing.T) {
	src := `# @graphql
POST https://example.com/graphql

query First {
  user { id }
}
# @graphql false
# @graphql
query Second {
  user { name }
}
`
	doc := Parse("graphql-reset.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if req.Body.GraphQL == nil {
		t.Fatalf("expected GraphQL body")
	}
	query := req.Body.GraphQL.Query
	if strings.Contains(query, "First") {
		t.Fatalf("unexpected stale query: %q", query)
	}
	if !strings.Contains(query, "Second") {
		t.Fatalf("expected query to contain second block, got %q", query)
	}
}

func TestParseOptionTokensQuotedValues(t *testing.T) {
	input := `expect.status="201 Created" vars.request.item_name='Workflow Demo Item' note=alpha\ beta message="He said \"hi\"" flag`
	opts := parseOptionTokens(input)

	if got := opts["expect.status"]; got != "201 Created" {
		t.Fatalf("expected expect.status to be '201 Created', got %q", got)
	}
	if got := opts["vars.request.item_name"]; got != "Workflow Demo Item" {
		t.Fatalf("expected vars.request.item_name to keep spaces, got %q", got)
	}
	if got := opts["note"]; got != "alpha beta" {
		t.Fatalf("expected escaped spaces to collapse, got %q", got)
	}
	if got := opts["message"]; got != "He said \"hi\"" {
		t.Fatalf("expected escaped quotes preserved, got %q", got)
	}
	if got := opts["flag"]; got != "true" {
		t.Fatalf("expected bare flag to default to true, got %q", got)
	}
}

func TestParseGRPCRequest(t *testing.T) {
	src := `# @name GRPCSample
# @grpc my.pkg.UserService/GetUser
# @grpc-descriptor descriptors/user.pb
# @grpc-plaintext false
# @grpc-metadata authorization: Bearer 123
GRPC localhost:50051

{
  "id": "abc"
}
`

	doc := Parse("grpc.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if req.GRPC == nil {
		t.Fatalf("expected grpc metadata")
	}
	grpc := req.GRPC
	if grpc.Service != "UserService" || grpc.Method != "GetUser" {
		t.Fatalf("unexpected service/method: %s/%s", grpc.Service, grpc.Method)
	}
	if grpc.Package != "my.pkg" {
		t.Fatalf("unexpected package: %s", grpc.Package)
	}
	if grpc.FullMethod != "/my.pkg.UserService/GetUser" {
		t.Fatalf("unexpected full method: %s", grpc.FullMethod)
	}
	if grpc.DescriptorSet != "descriptors/user.pb" {
		t.Fatalf("unexpected descriptor: %s", grpc.DescriptorSet)
	}
	if grpc.Plaintext {
		t.Fatalf("expected plaintext to be false")
	}
	if !grpc.PlaintextSet {
		t.Fatalf("expected plaintext directive to be marked as set")
	}
	found := false
	for _, pair := range grpc.Metadata {
		if pair.Key == "authorization" && pair.Value == "Bearer 123" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected metadata to be captured")
	}
	if strings.TrimSpace(grpc.Message) == "" {
		t.Fatalf("expected message body to be captured")
	}
}

func TestParseGRPCMetadataRepeats(t *testing.T) {
	src := `# @grpc my.pkg.UserService/GetUser
# @grpc-metadata x-id: one
# @grpc-metadata x-id: two
GRPC localhost:50051
{}`

	doc := Parse("grpc.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	grpc := doc.Requests[0].GRPC
	if grpc == nil {
		t.Fatalf("expected grpc metadata")
	}
	if len(grpc.Metadata) != 2 {
		t.Fatalf("expected 2 metadata entries, got %d", len(grpc.Metadata))
	}
	if grpc.Metadata[0].Key != "x-id" || grpc.Metadata[0].Value != "one" {
		t.Fatalf("unexpected first metadata entry: %#v", grpc.Metadata[0])
	}
	if grpc.Metadata[1].Key != "x-id" || grpc.Metadata[1].Value != "two" {
		t.Fatalf("unexpected second metadata entry: %#v", grpc.Metadata[1])
	}
}

func TestParseGRPCRequestDefaultsPlaintextToUnset(t *testing.T) {
	src := `# @name DefaultPlaintext
# @grpc my.pkg.UserService/GetUser
GRPC localhost:50051
{}
`

	doc := Parse("grpc.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	grpc := doc.Requests[0].GRPC
	if grpc == nil {
		t.Fatalf("expected grpc metadata")
	}
	if grpc.PlaintextSet {
		t.Fatalf("expected plaintext to be unset when directive is missing")
	}
}

func TestParseSSEDirective(t *testing.T) {
	src := `# @name stream
# @sse duration=45s idle=5s max-events=200 max-bytes=64kb
GET https://example.com/events
`

	doc := Parse("sse.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if req.SSE == nil {
		t.Fatalf("expected SSE metadata to be parsed")
	}
	if req.SSE.Options.TotalTimeout != 45*time.Second {
		t.Fatalf("unexpected total timeout: %v", req.SSE.Options.TotalTimeout)
	}
	if req.SSE.Options.IdleTimeout != 5*time.Second {
		t.Fatalf("unexpected idle timeout: %v", req.SSE.Options.IdleTimeout)
	}
	if req.SSE.Options.MaxEvents != 200 {
		t.Fatalf("unexpected max events: %d", req.SSE.Options.MaxEvents)
	}
	if req.SSE.Options.MaxBytes != 64*1024 {
		t.Fatalf("unexpected max bytes: %d", req.SSE.Options.MaxBytes)
	}
}

func TestParseWebSocketDirectives(t *testing.T) {
	src := `# @name ws
# @websocket timeout=12s idle=6s max-message-bytes=1mb subprotocols=chat,json compression=false
# @ws send Hello world
# @ws send-json {"op":"ping"}
# @ws send-base64 SGVsbG8=
# @ws send-file < data.bin
# @ws ping heartbeat
# @ws wait 2s
# @ws close 1001 going away
GET ws://example.com/socket
`

	doc := Parse("ws.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if req.WebSocket == nil {
		t.Fatalf("expected websocket metadata")
	}
	ws := req.WebSocket
	if ws.Options.HandshakeTimeout != 12*time.Second {
		t.Fatalf("unexpected handshake timeout: %v", ws.Options.HandshakeTimeout)
	}
	if ws.Options.IdleTimeout != 6*time.Second {
		t.Fatalf("unexpected idle timeout: %v", ws.Options.IdleTimeout)
	}
	if ws.Options.MaxMessageBytes != 1024*1024 {
		t.Fatalf("unexpected max message bytes: %d", ws.Options.MaxMessageBytes)
	}
	if len(ws.Options.Subprotocols) != 2 {
		t.Fatalf("expected 2 subprotocols, got %d", len(ws.Options.Subprotocols))
	}
	if ws.Options.Subprotocols[0] != "chat" || ws.Options.Subprotocols[1] != "json" {
		t.Fatalf("unexpected subprotocol list: %v", ws.Options.Subprotocols)
	}
	if !ws.Options.CompressionSet || ws.Options.Compression {
		t.Fatalf("expected compression flag to be false and explicitly set")
	}
	if len(ws.Steps) != 7 {
		t.Fatalf("expected 7 steps, got %d", len(ws.Steps))
	}
	if ws.Steps[0].Type != restfile.WebSocketStepSendText || ws.Steps[0].Value != "Hello world" {
		t.Fatalf("unexpected first step: %+v", ws.Steps[0])
	}
	if ws.Steps[3].Type != restfile.WebSocketStepSendFile || ws.Steps[3].File != "data.bin" {
		t.Fatalf("unexpected file step: %+v", ws.Steps[3])
	}
	if ws.Steps[5].Type != restfile.WebSocketStepWait || ws.Steps[5].Duration != 2*time.Second {
		t.Fatalf("unexpected wait step: %+v", ws.Steps[5])
	}
	if ws.Steps[6].Type != restfile.WebSocketStepClose || ws.Steps[6].Code != 1001 ||
		ws.Steps[6].Reason != "going away" {
		t.Fatalf("unexpected close step: %+v", ws.Steps[6])
	}
}

func TestParseTraceDirectiveWithBudgets(t *testing.T) {
	src := `# @trace dns<=50ms connect<=120ms total<=400ms tolerance=25ms
GET https://example.com/api
`

	doc := Parse("trace.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	spec := req.Metadata.Trace
	if spec == nil {
		t.Fatalf("expected trace metadata")
	}
	if !spec.Enabled {
		t.Fatalf("expected trace enabled")
	}
	if spec.Budgets.Total != 400*time.Millisecond {
		t.Fatalf("unexpected total budget: %v", spec.Budgets.Total)
	}
	if spec.Budgets.Tolerance != 25*time.Millisecond {
		t.Fatalf("unexpected tolerance: %v", spec.Budgets.Tolerance)
	}
	if spec.Budgets.Phases == nil {
		t.Fatalf("expected phase budgets")
	}
	if spec.Budgets.Phases["dns"] != 50*time.Millisecond {
		t.Fatalf("unexpected dns budget: %v", spec.Budgets.Phases["dns"])
	}
	if spec.Budgets.Phases["connect"] != 120*time.Millisecond {
		t.Fatalf("unexpected connect budget: %v", spec.Budgets.Phases["connect"])
	}
}

func TestParseTraceDirectiveDisabled(t *testing.T) {
	src := `# @trace enabled=false
GET https://example.com/api
`

	doc := Parse("trace-disabled.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	spec := req.Metadata.Trace
	if spec == nil {
		t.Fatalf("expected trace metadata")
	}
	if spec.Enabled {
		t.Fatalf("expected trace disabled")
	}
}

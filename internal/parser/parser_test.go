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

func TestParseOAuth2AuthSpec(t *testing.T) {
	spec := parseAuthSpec(`oauth2 token_url="https://auth.example.com/token" client_id=my-client client_secret="s3cr3t" scope="read write" grant=password username=jane password=pwd client_auth=body audience=https://api.example.com`)
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
	if workflow.Description == "" || !strings.Contains(workflow.Description, "Provision new account flow") {
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
	if grpc.Metadata["authorization"] != "Bearer 123" {
		t.Fatalf("expected metadata to be captured")
	}
	if strings.TrimSpace(grpc.Message) == "" {
		t.Fatalf("expected message body to be captured")
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

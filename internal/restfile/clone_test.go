package restfile

import (
	"net/http"
	"testing"
	"time"
)

func TestRequestCloneIsIndependent(t *testing.T) {
	req := &Request{
		Headers:   http.Header{"X-Test": {"one"}},
		Settings:  map[string]string{"timeout": "1s"},
		Variables: []Variable{{Name: "id", Value: "one"}},
		Metadata: RequestMetadata{
			Tags:    []string{"smoke"},
			Auth:    &AuthSpec{Params: map[string]string{"token": "one"}},
			Scripts: []ScriptBlock{{Lines: []ScriptLine{{Line: 1}}}},
			Applies: []ApplySpec{{Uses: []string{"base"}}},
			Profile: &ProfileSpec{Count: 1},
			Trace: &TraceSpec{
				Budgets: TraceBudget{Phases: map[string]time.Duration{"dns": time.Second}},
			},
			Compare: &CompareSpec{Environments: []string{"dev", "prod"}},
		},
		Body: BodySource{GraphQL: &GraphQLBody{Query: "query One"}},
		GRPC: &GRPCRequest{Metadata: []MetadataPair{{Key: "x-id", Value: "one"}}},
		WebSocket: &WebSocketRequest{
			Options: WebSocketOptions{Subprotocols: []string{"chat"}},
			Steps:   []WebSocketStep{{Type: WebSocketStepSendText, Value: "one"}},
		},
		SSH: &SSHSpec{Inline: &SSHProfile{Host: "one"}},
		K8s: &K8sSpec{Inline: &K8sProfile{Target: "pod/one"}},
	}

	got := req.Clone()
	got.Headers["X-Test"][0] = "two"
	got.Settings["timeout"] = "2s"
	got.Variables[0].Value = "two"
	got.Metadata.Tags[0] = "fast"
	got.Metadata.Auth.Params["token"] = "two"
	got.Metadata.Scripts[0].Lines[0].Line = 2
	got.Metadata.Applies[0].Uses[0] = "other"
	got.Metadata.Profile.Count = 2
	got.Metadata.Trace.Budgets.Phases["dns"] = 2 * time.Second
	got.Metadata.Compare.Environments[0] = "stage"
	got.Body.GraphQL.Query = "query Two"
	got.GRPC.Metadata[0].Value = "two"
	got.WebSocket.Options.Subprotocols[0] = "other"
	got.WebSocket.Steps[0].Value = "two"
	got.SSH.Inline.Host = "two"
	got.K8s.Inline.Target = "pod/two"

	if req.Headers.Get("X-Test") != "one" ||
		req.Settings["timeout"] != "1s" ||
		req.Variables[0].Value != "one" ||
		req.Metadata.Tags[0] != "smoke" ||
		req.Metadata.Auth.Params["token"] != "one" ||
		req.Metadata.Scripts[0].Lines[0].Line != 1 ||
		req.Metadata.Applies[0].Uses[0] != "base" ||
		req.Metadata.Profile.Count != 1 ||
		req.Metadata.Trace.Budgets.Phases["dns"] != time.Second ||
		req.Metadata.Compare.Environments[0] != "dev" ||
		req.Body.GraphQL.Query != "query One" ||
		req.GRPC.Metadata[0].Value != "one" ||
		req.WebSocket.Options.Subprotocols[0] != "chat" ||
		req.WebSocket.Steps[0].Value != "one" ||
		req.SSH.Inline.Host != "one" ||
		req.K8s.Inline.Target != "pod/one" {
		t.Fatal("mutating clone changed source request")
	}
}

func TestWorkflowCloneIsIndependent(t *testing.T) {
	code := 200
	wf := Workflow{
		Tags:    []string{"smoke"},
		Options: map[string]string{"mode": "one"},
		Steps: []WorkflowStep{{
			Expect:  WorkflowExpect{StatusCode: &code, Extra: map[string]string{"x": "one"}},
			Vars:    map[string]string{"id": "one"},
			Options: map[string]string{"retry": "one"},
			When:    &ConditionSpec{Expression: "one"},
			If: &WorkflowIf{
				Elifs: []WorkflowIfBranch{{Cond: "one"}},
				Else:  &WorkflowIfBranch{Run: "one"},
			},
			Switch: &WorkflowSwitch{
				Cases:   []WorkflowSwitchCase{{Expr: "one"}},
				Default: &WorkflowSwitchCase{Run: "one"},
			},
			ForEach: &WorkflowForEach{Expr: "one"},
		}},
	}

	got := wf.Clone()
	got.Tags[0] = "fast"
	got.Options["mode"] = "two"
	got.Steps[0].Expect.Extra["x"] = "two"
	*got.Steps[0].Expect.StatusCode = 201
	got.Steps[0].Vars["id"] = "two"
	got.Steps[0].Options["retry"] = "two"
	got.Steps[0].When.Expression = "two"
	got.Steps[0].If.Elifs[0].Cond = "two"
	got.Steps[0].If.Else.Run = "two"
	got.Steps[0].Switch.Cases[0].Expr = "two"
	got.Steps[0].Switch.Default.Run = "two"
	got.Steps[0].ForEach.Expr = "two"

	step := wf.Steps[0]
	if wf.Tags[0] != "smoke" ||
		wf.Options["mode"] != "one" ||
		step.Expect.Extra["x"] != "one" ||
		*step.Expect.StatusCode != 200 ||
		step.Vars["id"] != "one" ||
		step.Options["retry"] != "one" ||
		step.When.Expression != "one" ||
		step.If.Elifs[0].Cond != "one" ||
		step.If.Else.Run != "one" ||
		step.Switch.Cases[0].Expr != "one" ||
		step.Switch.Default.Run != "one" ||
		step.ForEach.Expr != "one" {
		t.Fatal("mutating clone changed source workflow")
	}
}

func TestDocumentCloneIsIndependent(t *testing.T) {
	doc := &Document{
		Settings: map[string]string{"timeout": "1s"},
		Requests: []*Request{{
			Headers: http.Header{"X-Test": {"one"}},
		}},
		Mocks: []*Mock{{
			Match: MockMatch{
				Query: map[string]MockQueryRule{"id": {Values: []string{"one"}}},
				Headers: map[string]MockHeaderRule{
					"Authorization": {Values: []string{"one"}},
				},
				JSON:      []byte(`{"id":"one"}`),
				JSONRules: []byte(`{"n":{"gt":1}}`),
			},
			Expectation: &MockExpectation{Calls: 1},
			Responses: []MockResponse{{
				Headers: http.Header{"X-Test": {"one"}},
				Body: BodySource{
					GraphQL: &GraphQLBody{Query: "query One"},
				},
			}},
		}},
		Raw: []byte("one"),
	}

	got := doc.Clone()
	got.Settings["timeout"] = "2s"
	got.Requests[0].Headers["X-Test"][0] = "two"
	query := got.Mocks[0].Match.Query["id"]
	query.Values[0] = "two"
	got.Mocks[0].Match.Query["id"] = query
	rule := got.Mocks[0].Match.Headers["Authorization"]
	rule.Values[0] = "two"
	got.Mocks[0].Match.Headers["Authorization"] = rule
	got.Mocks[0].Match.JSON[0] = '['
	got.Mocks[0].Match.JSONRules[0] = '['
	got.Mocks[0].Expectation.Calls = 2
	got.Mocks[0].Responses[0].Headers["X-Test"][0] = "two"
	got.Mocks[0].Responses[0].Body.GraphQL.Query = "query Two"
	got.Raw[0] = 't'

	mock := doc.Mocks[0]
	if doc.Settings["timeout"] != "1s" ||
		doc.Requests[0].Headers.Get("X-Test") != "one" ||
		mock.Match.Query["id"].Values[0] != "one" ||
		mock.Match.Headers["Authorization"].Values[0] != "one" ||
		string(mock.Match.JSON) != `{"id":"one"}` ||
		string(mock.Match.JSONRules) != `{"n":{"gt":1}}` ||
		mock.Expectation.Calls != 1 ||
		mock.Responses[0].Headers.Get("X-Test") != "one" ||
		mock.Responses[0].Body.GraphQL.Query != "query One" ||
		string(doc.Raw) != "one" {
		t.Fatal("mutating clone changed source document")
	}
}

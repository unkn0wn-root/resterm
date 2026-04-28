package workspace

import (
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestRefsCollectsStaticRequestRefs(t *testing.T) {
	doc := &restfile.Document{
		Path: "api.http",
		Uses: []restfile.UseSpec{
			{Path: "./helpers.rts", Line: 1},
		},
		Patches: []restfile.PatchProfile{
			{Expression: `json.file("./patch.json")`, Line: 2},
		},
		Requests: []*restfile.Request{
			{
				LineRange: restfile.LineRange{Start: 10, End: 30},
				Body: restfile.BodySource{
					FilePath: "./payload.json",
					GraphQL: &restfile.GraphQLBody{
						QueryFile:     "./query.graphql",
						VariablesFile: "./vars.json",
					},
				},
				GRPC: &restfile.GRPCRequest{
					MessageFile:   "./grpc.json",
					DescriptorSet: "./desc.protoset",
				},
				Metadata: restfile.RequestMetadata{
					Scripts: []restfile.ScriptBlock{
						{FilePath: "./pre.js"},
						{Lang: "rts", Body: `let flags = json.file("./script-flags.json")`},
					},
					Uses: []restfile.UseSpec{
						{Path: "./request.rts", Line: 11},
					},
					When: &restfile.ConditionSpec{
						Expression: `json.file("./when.json").enabled`,
						Line:       12,
					},
					ForEach: &restfile.ForEachSpec{
						Expression: `json.file("./items.json")`,
						Line:       13,
					},
					Asserts: []restfile.AssertSpec{
						{Expression: `json.file("./assert.json").ok`, Line: 14},
						{Expression: `json.file(vars.get("dynamic"))`, Line: 15},
					},
					Applies: []restfile.ApplySpec{
						{Expression: `rts.json.file("./apply.json")`, Line: 16},
					},
					Captures: []restfile.CaptureSpec{
						{
							Expression: `json.file("./capture.json")`,
							Mode:       restfile.CaptureExprModeAuto,
							Line:       17,
						},
						{
							Expression: `json.file("./template-capture.json")`,
							Mode:       restfile.CaptureExprModeTemplate,
							Line:       18,
						},
					},
				},
				WebSocket: &restfile.WebSocketRequest{
					Steps: []restfile.WebSocketStep{
						{Type: restfile.WebSocketStepSendFile, File: "./ws.bin"},
					},
				},
			},
		},
		Workflows: []restfile.Workflow{
			{
				Steps: []restfile.WorkflowStep{
					{
						When: &restfile.ConditionSpec{
							Expression: `stdlib.json.file("./workflow.json")`,
							Line:       40,
						},
					},
				},
			},
		},
	}

	refs := Refs(doc)
	tests := []struct {
		kind RefKind
		path string
	}{
		{kind: RefUse, path: "./helpers.rts"},
		{kind: RefRTSJSON, path: "./patch.json"},
		{kind: RefBody, path: "./payload.json"},
		{kind: RefGraphQL, path: "./query.graphql"},
		{kind: RefGraphQL, path: "./vars.json"},
		{kind: RefGRPC, path: "./grpc.json"},
		{kind: RefGRPC, path: "./desc.protoset"},
		{kind: RefScript, path: "./pre.js"},
		{kind: RefRTSJSON, path: "./script-flags.json"},
		{kind: RefUse, path: "./request.rts"},
		{kind: RefRTSJSON, path: "./when.json"},
		{kind: RefRTSJSON, path: "./items.json"},
		{kind: RefRTSJSON, path: "./assert.json"},
		{kind: RefRTSJSON, path: "./apply.json"},
		{kind: RefRTSJSON, path: "./capture.json"},
		{kind: RefWebSocket, path: "./ws.bin"},
		{kind: RefRTSJSON, path: "./workflow.json"},
	}
	for _, tt := range tests {
		if !hasRef(refs, tt.kind, tt.path) {
			t.Fatalf("expected %s as kind %d in %+v", tt.path, tt.kind, refs)
		}
	}

	for _, path := range []string{"dynamic", "./template-capture.json"} {
		if hasAnyRef(refs, path) {
			t.Fatalf("did not expect %s in %+v", path, refs)
		}
	}
}

func TestRefsNilDocument(t *testing.T) {
	if refs := Refs(nil); refs != nil {
		t.Fatalf("expected nil refs, got %+v", refs)
	}
}

func hasRef(refs []Ref, kind RefKind, path string) bool {
	for _, ref := range refs {
		if ref.Kind == kind && ref.Path == path {
			return true
		}
	}
	return false
}

func hasAnyRef(refs []Ref, path string) bool {
	for _, ref := range refs {
		if ref.Path == path {
			return true
		}
	}
	return false
}

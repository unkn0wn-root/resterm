package restfile

import (
	"bytes"
	"maps"
	"slices"
)

// Parsed values are reused by previews, profiles, comparisons, and workflows.
// These methods keep one run's mutations from leaking into another.
func (req *Request) Clone() *Request {
	if req == nil {
		return nil
	}

	dst := *req
	dst.Headers = req.Headers.Clone()
	dst.Settings = maps.Clone(req.Settings)
	dst.Variables = slices.Clone(req.Variables)
	dst.Metadata = req.Metadata.Clone()
	dst.Body = cloneBodySource(req.Body)
	dst.GRPC = cloneGRPCRequest(req.GRPC)
	dst.SSE = clonePtr(req.SSE)
	dst.WebSocket = cloneWebSocketRequest(req.WebSocket)
	dst.SSH = cloneSSHSpec(req.SSH)
	dst.K8s = cloneK8sSpec(req.K8s)
	return &dst
}

func (meta RequestMetadata) Clone() RequestMetadata {
	meta.Tags = slices.Clone(meta.Tags)
	meta.Auth = meta.Auth.Clone()
	meta.Scripts = cloneScriptBlocks(meta.Scripts)
	meta.Uses = slices.Clone(meta.Uses)
	meta.Applies = cloneApplySpecs(meta.Applies)
	meta.When = clonePtr(meta.When)
	meta.ForEach = clonePtr(meta.ForEach)
	meta.Asserts = slices.Clone(meta.Asserts)
	meta.Captures = slices.Clone(meta.Captures)
	meta.Profile = clonePtr(meta.Profile)
	meta.Trace = meta.Trace.Clone()
	meta.Compare = meta.Compare.Clone()
	return meta
}

func (e WorkflowExpect) Clone() WorkflowExpect {
	e.StatusCode = clonePtr(e.StatusCode)
	e.Extra = maps.Clone(e.Extra)
	return e
}

func cloneScriptBlocks(src []ScriptBlock) []ScriptBlock {
	if len(src) == 0 {
		return nil
	}
	dst := slices.Clone(src)
	for i := range dst {
		dst[i].Lines = slices.Clone(src[i].Lines)
	}
	return dst
}

func (spec *TraceSpec) Clone() *TraceSpec {
	if spec == nil {
		return nil
	}
	dst := *spec
	dst.Budgets.Phases = maps.Clone(spec.Budgets.Phases)
	return &dst
}

func (spec *CompareSpec) Clone() *CompareSpec {
	if spec == nil {
		return nil
	}
	dst := *spec
	dst.Environments = slices.Clone(spec.Environments)
	return &dst
}

func (wf Workflow) Clone() Workflow {
	wf.Tags = slices.Clone(wf.Tags)
	wf.Options = maps.Clone(wf.Options)
	wf.Steps = cloneWorkflowSteps(wf.Steps)
	return wf
}

func (step WorkflowStep) Clone() WorkflowStep {
	step.Expect = step.Expect.Clone()
	step.Vars = maps.Clone(step.Vars)
	step.Options = maps.Clone(step.Options)
	step.When = clonePtr(step.When)
	step.If = step.If.Clone()
	step.Switch = step.Switch.Clone()
	step.ForEach = clonePtr(step.ForEach)
	return step
}

func (flow *WorkflowIf) Clone() *WorkflowIf {
	if flow == nil {
		return nil
	}
	dst := *flow
	dst.Elifs = slices.Clone(flow.Elifs)
	dst.Else = clonePtr(flow.Else)
	return &dst
}

func (flow *WorkflowSwitch) Clone() *WorkflowSwitch {
	if flow == nil {
		return nil
	}
	dst := *flow
	dst.Cases = slices.Clone(flow.Cases)
	dst.Default = clonePtr(flow.Default)
	return &dst
}

func (mock *Mock) Clone() *Mock {
	if mock == nil {
		return nil
	}
	dst := *mock
	dst.Match = cloneMockMatch(mock.Match)
	dst.Expectation = clonePtr(mock.Expectation)
	dst.Responses = cloneMockResponses(mock.Responses)
	return &dst
}

func (doc *Document) Clone() *Document {
	if doc == nil {
		return nil
	}

	dst := *doc
	dst.Variables = slices.Clone(doc.Variables)
	dst.Globals = slices.Clone(doc.Globals)
	dst.Constants = slices.Clone(doc.Constants)
	dst.Auth = cloneAuthProfiles(doc.Auth)
	dst.SSH = slices.Clone(doc.SSH)
	dst.K8s = slices.Clone(doc.K8s)
	dst.Patches = slices.Clone(doc.Patches)
	dst.Settings = maps.Clone(doc.Settings)
	dst.Uses = slices.Clone(doc.Uses)
	dst.Requests = cloneRequests(doc.Requests)
	dst.Mocks = cloneMocks(doc.Mocks)
	dst.Workflows = cloneWorkflows(doc.Workflows)
	dst.Errors = slices.Clone(doc.Errors)
	dst.Warnings = slices.Clone(doc.Warnings)
	dst.Raw = bytes.Clone(doc.Raw)
	return &dst
}

func cloneApplySpecs(src []ApplySpec) []ApplySpec {
	dst := slices.Clone(src)
	for i := range dst {
		dst[i].Uses = slices.Clone(src[i].Uses)
	}
	return dst
}

func cloneBodySource(src BodySource) BodySource {
	src.GraphQL = clonePtr(src.GraphQL)
	return src
}

func cloneGRPCRequest(src *GRPCRequest) *GRPCRequest {
	dst := clonePtr(src)
	if dst != nil {
		dst.Metadata = slices.Clone(src.Metadata)
	}
	return dst
}

func cloneWebSocketRequest(src *WebSocketRequest) *WebSocketRequest {
	dst := clonePtr(src)
	if dst != nil {
		dst.Options.Subprotocols = slices.Clone(src.Options.Subprotocols)
		dst.Steps = slices.Clone(src.Steps)
	}
	return dst
}

func cloneSSHSpec(src *SSHSpec) *SSHSpec {
	dst := clonePtr(src)
	if dst != nil {
		dst.Inline = clonePtr(src.Inline)
	}
	return dst
}

func cloneK8sSpec(src *K8sSpec) *K8sSpec {
	dst := clonePtr(src)
	if dst != nil {
		dst.Inline = clonePtr(src.Inline)
	}
	return dst
}

func cloneAuthProfiles(src []AuthProfile) []AuthProfile {
	dst := slices.Clone(src)
	for i := range dst {
		dst[i].Spec = *src[i].Spec.Clone()
	}
	return dst
}

func cloneRequests(src []*Request) []*Request {
	if src == nil {
		return nil
	}
	dst := make([]*Request, len(src))
	for i, req := range src {
		dst[i] = req.Clone()
	}
	return dst
}

func cloneMocks(src []*Mock) []*Mock {
	if src == nil {
		return nil
	}
	dst := make([]*Mock, len(src))
	for i, mock := range src {
		dst[i] = mock.Clone()
	}
	return dst
}

func cloneMockMatch(src MockMatch) MockMatch {
	src.Query = cloneMockQuery(src.Query)
	src.Headers = cloneMockHeaders(src.Headers)
	src.JSON = bytes.Clone(src.JSON)
	return src
}

func cloneMockQuery(src map[string]MockQueryRule) map[string]MockQueryRule {
	if src == nil {
		return nil
	}
	dst := make(map[string]MockQueryRule, len(src))
	for key, rule := range src {
		rule.Values = slices.Clone(rule.Values)
		dst[key] = rule
	}
	return dst
}

func cloneMockHeaders(src map[string]MockHeaderRule) map[string]MockHeaderRule {
	if src == nil {
		return nil
	}
	dst := make(map[string]MockHeaderRule, len(src))
	for key, rule := range src {
		rule.Values = slices.Clone(rule.Values)
		dst[key] = rule
	}
	return dst
}

func cloneMockResponses(src []MockResponse) []MockResponse {
	dst := slices.Clone(src)
	for i := range dst {
		dst[i].Headers = src[i].Headers.Clone()
		dst[i].Body = cloneBodySource(src[i].Body)
	}
	return dst
}

func cloneWorkflows(src []Workflow) []Workflow {
	if src == nil {
		return nil
	}
	dst := make([]Workflow, len(src))
	for i, wf := range src {
		dst[i] = wf.Clone()
	}
	return dst
}

func cloneWorkflowSteps(src []WorkflowStep) []WorkflowStep {
	if src == nil {
		return nil
	}
	dst := make([]WorkflowStep, len(src))
	for i, step := range src {
		dst[i] = step.Clone()
	}
	return dst
}

func clonePtr[T any](src *T) *T {
	if src == nil {
		return nil
	}
	dst := *src
	return &dst
}

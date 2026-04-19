package runner

import (
	"bytes"
	"maps"
	"slices"

	"github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func cloneDoc(src *restfile.Document) *restfile.Document {
	if src == nil {
		return nil
	}

	dst := *src
	dst.Variables = slices.Clone(src.Variables)
	dst.Globals = slices.Clone(src.Globals)
	dst.Constants = slices.Clone(src.Constants)
	dst.Auth = cloneAuthProfiles(src.Auth)
	dst.SSH = slices.Clone(src.SSH)
	dst.K8s = slices.Clone(src.K8s)
	dst.Patches = slices.Clone(src.Patches)
	dst.Settings = maps.Clone(src.Settings)
	dst.Uses = slices.Clone(src.Uses)
	dst.Requests = cloneRequests(src.Requests)
	dst.Workflows = cloneWorkflows(src.Workflows)
	dst.Errors = slices.Clone(src.Errors)
	dst.Warnings = slices.Clone(src.Warnings)
	dst.Raw = bytes.Clone(src.Raw)
	return &dst
}

func cloneAuthProfiles(src []restfile.AuthProfile) []restfile.AuthProfile {
	dst := slices.Clone(src)
	for i := range dst {
		dst[i].Spec = restfile.CloneAuthSpecValue(src[i].Spec)
	}
	return dst
}

func cloneRequests(src []*restfile.Request) []*restfile.Request {
	if len(src) == 0 {
		return nil
	}

	dst := make([]*restfile.Request, len(src))
	for i, req := range src {
		dst[i] = request.CloneRequest(req)
	}
	return dst
}

func cloneWorkflows(src []restfile.Workflow) []restfile.Workflow {
	dst := slices.Clone(src)
	for i := range dst {
		dst[i] = cloneWorkflow(src[i])
	}
	return dst
}

func cloneWorkflow(src restfile.Workflow) restfile.Workflow {
	dst := src
	dst.Tags = slices.Clone(src.Tags)
	dst.Options = maps.Clone(src.Options)
	dst.Steps = cloneWorkflowSteps(src.Steps)
	return dst
}

func cloneWorkflowSteps(src []restfile.WorkflowStep) []restfile.WorkflowStep {
	dst := slices.Clone(src)
	for i := range dst {
		dst[i] = cloneWorkflowStep(src[i])
	}
	return dst
}

func cloneWorkflowStep(src restfile.WorkflowStep) restfile.WorkflowStep {
	dst := src
	dst.Expect = maps.Clone(src.Expect)
	dst.Vars = maps.Clone(src.Vars)
	dst.Options = maps.Clone(src.Options)
	dst.When = clonePtr(src.When)
	dst.If = cloneWorkflowIf(src.If)
	dst.Switch = cloneWorkflowSwitch(src.Switch)
	dst.ForEach = clonePtr(src.ForEach)
	return dst
}

func cloneWorkflowIf(src *restfile.WorkflowIf) *restfile.WorkflowIf {
	dst := clonePtr(src)
	if dst == nil {
		return nil
	}
	dst.Then = src.Then
	dst.Elifs = slices.Clone(src.Elifs)
	if src.Else != nil {
		br := *src.Else
		dst.Else = &br
	}
	return dst
}

func cloneWorkflowSwitch(src *restfile.WorkflowSwitch) *restfile.WorkflowSwitch {
	dst := clonePtr(src)
	if dst == nil {
		return nil
	}
	dst.Cases = slices.Clone(src.Cases)
	if src.Default != nil {
		cs := *src.Default
		dst.Default = &cs
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

package request

import (
	"maps"
	"net/http"
	"slices"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func CloneRequest(req *restfile.Request) *restfile.Request { return cloneRequest(req) }

func cloneRequest(req *restfile.Request) *restfile.Request {
	if req == nil {
		return nil
	}

	dst := *req
	dst.Headers = cloneHeader(req.Headers)
	dst.Settings = maps.Clone(req.Settings)
	dst.Variables = slices.Clone(req.Variables)
	dst.Metadata = cloneRequestMetadata(req.Metadata)
	dst.Body = cloneBodySource(req.Body)
	dst.GRPC = cloneGRPCRequest(req.GRPC)
	dst.SSE = clonePtr(req.SSE)
	dst.WebSocket = cloneWebSocketRequest(req.WebSocket)
	dst.SSH = cloneSSHSpec(req.SSH)
	dst.K8s = cloneK8sSpec(req.K8s)
	return engine.NormReq(&dst)
}

func cloneRequestMetadata(src restfile.RequestMetadata) restfile.RequestMetadata {
	dst := src
	dst.Tags = slices.Clone(src.Tags)
	dst.Auth = restfile.CloneAuthSpec(src.Auth)
	dst.Scripts = restfile.CloneScriptBlocks(src.Scripts)
	dst.Uses = slices.Clone(src.Uses)
	dst.Applies = cloneApplySpecs(src.Applies)
	dst.When = clonePtr(src.When)
	dst.ForEach = clonePtr(src.ForEach)
	dst.Asserts = slices.Clone(src.Asserts)
	dst.Captures = slices.Clone(src.Captures)
	dst.Profile = clonePtr(src.Profile)
	dst.Trace = cloneTraceSpec(src.Trace)
	dst.Compare = cloneCompareSpec(src.Compare)
	return dst
}

func cloneApplySpecs(src []restfile.ApplySpec) []restfile.ApplySpec {
	dst := slices.Clone(src)
	for i := range dst {
		dst[i].Uses = slices.Clone(src[i].Uses)
	}
	return dst
}

func cloneTraceSpec(src *restfile.TraceSpec) *restfile.TraceSpec {
	dst := clonePtr(src)
	if dst == nil {
		return nil
	}
	dst.Budgets.Phases = maps.Clone(src.Budgets.Phases)
	return dst
}

func cloneCompareSpec(src *restfile.CompareSpec) *restfile.CompareSpec {
	dst := clonePtr(src)
	if dst == nil {
		return nil
	}
	dst.Environments = slices.Clone(src.Environments)
	return dst
}

func cloneBodySource(src restfile.BodySource) restfile.BodySource {
	dst := src
	dst.GraphQL = clonePtr(src.GraphQL)
	return dst
}

func cloneGRPCRequest(src *restfile.GRPCRequest) *restfile.GRPCRequest {
	dst := clonePtr(src)
	if dst == nil {
		return nil
	}
	dst.Metadata = slices.Clone(src.Metadata)
	return dst
}

func cloneWebSocketRequest(src *restfile.WebSocketRequest) *restfile.WebSocketRequest {
	dst := clonePtr(src)
	if dst == nil {
		return nil
	}
	dst.Options.Subprotocols = slices.Clone(src.Options.Subprotocols)
	dst.Steps = slices.Clone(src.Steps)
	return dst
}

func cloneSSHSpec(src *restfile.SSHSpec) *restfile.SSHSpec {
	dst := clonePtr(src)
	if dst == nil {
		return nil
	}
	dst.Inline = clonePtr(src.Inline)
	return dst
}

func cloneK8sSpec(src *restfile.K8sSpec) *restfile.K8sSpec {
	dst := clonePtr(src)
	if dst == nil {
		return nil
	}
	dst.Inline = clonePtr(src.Inline)
	return dst
}

func cloneHeader(src http.Header) http.Header {
	if src == nil {
		return nil
	}

	dst := make(http.Header, len(src))
	for name, values := range src {
		dst[name] = slices.Clone(values)
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

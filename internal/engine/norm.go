package engine

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func ReqMethod(req *restfile.Request) string {
	if req == nil {
		return "REQ"
	}
	switch {
	case req.GRPC != nil:
		return "GRPC"
	case req.WebSocket != nil:
		return "WS"
	case req.SSE != nil:
		return "SSE"
	}
	if m := up(req.Method); m != "" {
		return m
	}
	return "REQ"
}

func ReqTarget(req *restfile.Request) string {
	if req == nil {
		return ""
	}
	if req.GRPC != nil {
		if m := trim(req.GRPC.FullMethod); m != "" {
			return m
		}
		if t := trim(req.GRPC.Target); t != "" {
			return t
		}
	}
	return trim(req.URL)
}

func ReqTitle(req *restfile.Request) string {
	if req == nil {
		return ""
	}
	name := trim(req.Metadata.Name)
	if name == "" {
		name = ReqTarget(req)
		if len(name) > 60 {
			name = name[:57] + "..."
		}
	}
	return ReqMethod(req) + " " + name
}

func ReqID(req *restfile.Request) string {
	if req == nil {
		return ""
	}
	if name := trim(req.Metadata.Name); name != "" {
		return name
	}
	return trim(req.URL)
}

func Tags(xs []string) []string {
	if len(xs) == 0 {
		return nil
	}
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		x = trim(x)
		if x != "" {
			out = append(out, x)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func NormReq(req *restfile.Request) *restfile.Request {
	if req == nil {
		return nil
	}

	req.Metadata.Name = trim(req.Metadata.Name)
	req.Metadata.Tags = Tags(req.Metadata.Tags)
	req.Method = up(req.Method)
	req.URL = trim(req.URL)
	normBody(&req.Body)
	normGRPC(req.GRPC)
	normWS(req.WebSocket)
	return req
}

func NormWf(wf restfile.Workflow) restfile.Workflow {
	wf.Name = trim(wf.Name)
	wf.Tags = Tags(wf.Tags)
	for i := range wf.Steps {
		normWfStep(&wf.Steps[i])
	}
	return wf
}

func normBody(body *restfile.BodySource) {
	body.FilePath = trim(body.FilePath)
	body.MimeType = trim(body.MimeType)
	normGQL(body.GraphQL)
}

func normGQL(gql *restfile.GraphQLBody) {
	if gql == nil {
		return
	}
	gql.QueryFile = trim(gql.QueryFile)
	gql.VariablesFile = trim(gql.VariablesFile)
	gql.OperationName = trim(gql.OperationName)
}

func normGRPC(grpc *restfile.GRPCRequest) {
	if grpc == nil {
		return
	}
	grpc.Target = trim(grpc.Target)
	grpc.Package = trim(grpc.Package)
	grpc.Service = trim(grpc.Service)
	grpc.Method = trim(grpc.Method)
	grpc.FullMethod = trim(grpc.FullMethod)
	grpc.DescriptorSet = trim(grpc.DescriptorSet)
	grpc.Authority = trim(grpc.Authority)
	grpc.MessageFile = trim(grpc.MessageFile)
}

func normWS(ws *restfile.WebSocketRequest) {
	if ws == nil {
		return
	}
	for i := range ws.Steps {
		ws.Steps[i].File = trim(ws.Steps[i].File)
	}
}

func normWfStep(step *restfile.WorkflowStep) {
	step.Name = trim(step.Name)
	step.Using = trim(step.Using)
}

func trim(s string) string {
	return strings.TrimSpace(s)
}

func up(s string) string {
	return strings.ToUpper(trim(s))
}

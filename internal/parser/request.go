package parser

import (
	"strings"

	graphqlbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/graphql"
	grpcbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/grpc"
	httpbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/http"
	ssebuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/sse"
	wsbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/websocket"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

type requestBuilder struct {
	startLine         int
	metadata          restfile.RequestMetadata
	variables         []restfile.Variable
	originalLines     []string
	currentScriptKind scriptKind
	currentScriptLang scriptLang
	discardScript     bool
	scriptBufferKind  scriptKind
	scriptBufferLang  scriptLang
	scriptSourcePath  string
	scriptBufferLines []restfile.ScriptLine
	scriptBuffer      []string
	settings          map[string]string
	http              *httpbuilder.Builder
	graphql           *graphqlbuilder.Builder
	grpc              *grpcbuilder.Builder
	sse               *ssebuilder.Builder
	websocket         *wsbuilder.Builder
	bodyOptions       restfile.BodyOptions
	multipart         *multipartSpan
	ssh               *restfile.SSHSpec
	k8s               *restfile.K8sSpec
}

// protoDirective offers a directive to each protocol builder in turn. The
// first builder that recognizes the key claims it, so the order decides
// which protocol wins when a key is ambiguous.
func (r *requestBuilder) protoDirective(key, rest string) bool {
	if r.grpc.HandleDirective(key, rest) {
		return true
	}
	if r.websocket.HandleDirective(key, rest) {
		return true
	}
	if r.sse.HandleDirective(key, rest) {
		return true
	}
	return r.graphql.HandleDirective(key, rest)
}

func (r *requestBuilder) protoBodyLine(raw string) bool {
	if r.graphql.HandleBodyLine(raw, r.bodyOptions.ForceInline) {
		return true
	}
	return r.grpc.HandleBodyLine(raw, r.bodyOptions.ForceInline)
}

func (r *requestBuilder) applyReqSettings(req *restfile.Request) {
	if r.settings != nil {
		req.Settings = r.settings
	}
	if r.ssh != nil {
		req.SSH = r.ssh
	}
	if r.k8s != nil {
		req.K8s = r.k8s
	}
}

func (r *requestBuilder) build() *restfile.Request {
	r.flushPendingScript()

	vars := append([]restfile.Variable(nil), r.variables...)

	req := &restfile.Request{
		Metadata:  r.metadata,
		Method:    r.http.Method(),
		URL:       str.Trim(r.http.URL()),
		Headers:   r.http.HeaderMap(),
		Body:      restfile.BodySource{},
		Variables: vars,
		Settings:  map[string]string{},
		LineRange: restfile.LineRange{
			Start: r.startLine,
			End:   r.startLine + len(r.originalLines) - 1,
		},
		OriginalText: strings.Join(r.originalLines, "\n"),
	}

	appliedBody := false
	if wsReq, ok := r.websocket.Finalize(); ok {
		req.WebSocket = wsReq
	}
	if sseReq, ok := r.sse.Finalize(); ok {
		req.SSE = sseReq
	}

	if req.WebSocket == nil && req.SSE == nil {
		if grpcReq, body, mime, ok := r.grpc.Finalize(r.http.MimeType()); ok {
			req.GRPC = grpcReq
			req.Body = body
			if mime != "" {
				req.Body.MimeType = mime
			}
			req.Body.Options = r.bodyOptions
			r.applyReqSettings(req)
			return req
		} else if gql, mime, ok := r.graphql.Finalize(r.http.MimeType()); ok {
			req.Body.GraphQL = gql
			if mime != "" {
				req.Body.MimeType = mime
			}
		} else {
			r.applyHTTPBody(req)
			req.Body.Options = r.bodyOptions
			appliedBody = true
		}
	}

	if !appliedBody {
		r.applyHTTPBody(req)
	}
	r.applyReqSettings(req)

	return req
}

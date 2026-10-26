package parser

import (
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/directive"
	graphqlbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/graphql"
	grpcbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/grpc"
	httpbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/http"
	ssebuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/sse"
	wsbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/websocket"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type requestBuilder struct {
	startLine         int
	opener            directive.Name
	endsWorkflow      bool
	sourcePath        string
	metadata          restfile.RequestMetadata
	variables         []restfile.Variable
	runVars           []restfile.RunVar
	originalLines     []string
	script            scriptMode
	scriptBufferLines []restfile.ScriptLine
	scriptBuffer      []string
	settings          map[string]string
	settingDirectives []parsedDirective
	http              *httpbuilder.Builder
	graphql           *graphqlbuilder.Builder
	grpc              *grpcbuilder.Builder
	grpcDirectives    []parsedDirective
	protocols         []parsedDirective
	sse               *ssebuilder.Builder
	websocket         *wsbuilder.Builder
	bodyOptions       restfile.BodyOptions
	multipart         *multipartSpan
	ssh               *restfile.SSHSpec
	k8s               *restfile.K8sSpec
	retry             *retryDraft
	declared          map[directive.Name]bool
}

type featureBuilder interface {
	HandleDirective(name directive.Name, rest string) (handled, reset bool, err error)
}

// protoDirective offers a directive to each protocol builder in turn. The
// first builder that recognizes the key claims it, so the order decides
// which protocol wins when a key is ambiguous.
func (r *requestBuilder) protoDirective(d parsedDirective) (bool, error) {
	for _, feature := range []featureBuilder{r.grpc, r.websocket, r.sse, r.graphql} {
		handled, reset, err := feature.HandleDirective(d.Name, d.Args)
		if !handled {
			continue
		}
		if reset {
			r.clearDeclared(d.Name)
			r.protocols = slices.DeleteFunc(
				r.protocols,
				func(s parsedDirective) bool { return protocolOf(s.Name) == d.Name },
			)
		} else if !fatalErr(err) && !d.valueMissing() {
			// A rejected or bare directive already has its own error.
			switch {
			case feature == r.grpc:
				r.grpcDirectives = append(r.grpcDirectives, d)
			case d.Name == directive.WebSocket,
				d.Name == directive.WS,
				d.Name == directive.SSE,
				d.Name == directive.GraphQL:
				r.protocols = append(r.protocols, d)
			}
		}
		return true, err
	}
	return false, nil
}

func protocolOf(name directive.Name) directive.Name {
	if name == directive.WS {
		return directive.WebSocket
	}
	return name
}

func (r *requestBuilder) clearDeclared(name directive.Name) {
	for _, cleared := range name.Resets() {
		delete(r.declared, cleared)
	}
}

func (r *requestBuilder) protoBodyLine(raw string) bool {
	if r.isGRPC() {
		return r.grpc.HandleBodyLine(raw, r.bodyOptions.ForceInline)
	}
	return r.graphql.HandleBodyLine(raw, r.bodyOptions.ForceInline)
}

func (r *requestBuilder) isGRPC() bool {
	return r.http.Method() == "GRPC"
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

	urlLine, urlCol := r.http.URLPos()
	req := &restfile.Request{
		Metadata:  r.metadata,
		Method:    r.http.Method(),
		URL:       strings.TrimSpace(r.http.URL()),
		Headers:   r.http.HeaderMap(),
		Body:      restfile.BodySource{},
		Variables: vars,
		RunVars:   r.runVars,
		Settings:  map[string]string{},
		LineRange: restfile.LineRange{
			Start: r.startLine,
			End:   r.startLine + len(r.originalLines) - 1,
		},
		SourcePath:   r.sourcePath,
		OriginalText: strings.Join(r.originalLines, "\n"),
		URLLine:      urlLine,
		URLCol:       urlCol,
		HeaderLines:  r.http.HeaderLines(),
	}

	if r.isGRPC() {
		if grpcReq, body, mime, ok := r.grpc.Finalize(r.http.MimeType()); ok {
			req.GRPC = grpcReq
			req.Body = body
			if mime != "" {
				req.Body.MimeType = mime
			}
			req.Body.Options = r.bodyOptions
			r.applyReqSettings(req)
			return req
		}
	}

	if wsReq, ok := r.websocket.Finalize(); ok {
		req.WebSocket = wsReq
	} else if sseReq, ok := r.sse.Finalize(); ok {
		req.SSE = sseReq
	} else if gql, mime, ok := r.graphql.Finalize(r.http.MimeType()); ok {
		req.Body.GraphQL = gql
		if mime != "" {
			req.Body.MimeType = mime
		}
	} else {
		req.Body.Options = r.bodyOptions
	}
	r.applyHTTPBody(req)
	r.applyReqSettings(req)
	return req
}

package parser

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/parser/graphqlbuilder"
	"github.com/unkn0wn-root/resterm/internal/parser/grpcbuilder"
	"github.com/unkn0wn-root/resterm/internal/parser/httpbuilder"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

const (
	defaultScriptKind = "test"
	defaultScriptLang = "js"
)

type requestBuilder struct {
	startLine         int
	endLine           int
	metadata          restfile.RequestMetadata
	variables         []restfile.Variable
	originalLines     []string
	currentScriptKind string
	currentScriptLang string
	scriptBufferKind  string
	scriptBufferLang  string
	scriptBuffer      []string
	settings          map[string]string
	http              *httpbuilder.Builder
	graphql           *graphqlbuilder.Builder
	grpc              *grpcbuilder.Builder
	sse               *sseBuilder
	websocket         *wsBuilder
	bodyOptions       restfile.BodyOptions
	ssh               *restfile.SSHSpec
}

func normScriptKind(kind string) string {
	out := strings.ToLower(strings.TrimSpace(kind))
	if out == "" {
		return defaultScriptKind
	}
	return out
}

func normScriptLang(lang string) string {
	out := strings.ToLower(strings.TrimSpace(lang))
	switch out {
	case "":
		return defaultScriptLang
	case "javascript":
		return defaultScriptLang
	case "restermlang":
		return "rts"
	default:
		return out
	}
}

func (r *requestBuilder) appendScriptLine(kind, lang, body string) {
	kind = normScriptKind(kind)
	lang = normScriptLang(lang)
	if r.scriptBufferKind != "" &&
		(!strings.EqualFold(r.scriptBufferKind, kind) || !strings.EqualFold(r.scriptBufferLang, lang)) {
		r.flushPendingScript()
	}
	if r.scriptBufferKind == "" {
		r.scriptBufferKind = kind
		r.scriptBufferLang = lang
	}
	r.scriptBuffer = append(r.scriptBuffer, body)
}

func (r *requestBuilder) flushPendingScript() {
	if len(r.scriptBuffer) == 0 {
		return
	}
	script := strings.Join(r.scriptBuffer, "\n")
	r.metadata.Scripts = append(r.metadata.Scripts, restfile.ScriptBlock{
		Kind: r.scriptBufferKind,
		Lang: r.scriptBufferLang,
		Body: script,
	})
	r.scriptBuffer = nil
	r.scriptBufferKind = ""
	r.scriptBufferLang = ""
}

func (r *requestBuilder) appendScriptInclude(kind, lang, path string) {
	kind = normScriptKind(kind)
	lang = normScriptLang(lang)
	r.flushPendingScript()
	r.metadata.Scripts = append(r.metadata.Scripts, restfile.ScriptBlock{
		Kind:     kind,
		Lang:     lang,
		FilePath: path,
	})
}

func (r *requestBuilder) handleBodyDirective(rest string) bool {
	value := strings.TrimSpace(rest)
	if value == "" {
		return false
	}
	key, val := splitDirective(value)
	if key == "" {
		key = value
	}
	switch strings.ToLower(key) {
	case "expand", "expand-templates":
		enabled := true
		if strings.TrimSpace(val) != "" {
			if parsed, ok := parseBool(val); ok {
				enabled = parsed
			}
		}
		r.bodyOptions.ExpandTemplates = enabled
		return true
	default:
		return false
	}
}

func (r *requestBuilder) markHeadersDone() {
	if r == nil || r.http == nil || r.http.HeaderDone() {
		return
	}
	r.http.MarkHeadersDone()
}

func (r *requestBuilder) applyHTTPBody(req *restfile.Request) {
	if file := r.http.BodyFromFile(); file != "" {
		req.Body.FilePath = file
	} else if text := r.http.BodyText(); text != "" {
		req.Body.Text = text
	}
	if mime := r.http.MimeType(); mime != "" {
		req.Body.MimeType = mime
	}
}

func (r *requestBuilder) applyReqSettings(req *restfile.Request) {
	if r.settings != nil {
		req.Settings = r.settings
	}
	if r.ssh != nil {
		req.SSH = r.ssh
	}
}

func (r *requestBuilder) build() *restfile.Request {
	r.flushPendingScript()

	vars := append([]restfile.Variable(nil), r.variables...)

	req := &restfile.Request{
		Metadata:  r.metadata,
		Method:    r.http.Method(),
		URL:       strings.TrimSpace(r.http.URL()),
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

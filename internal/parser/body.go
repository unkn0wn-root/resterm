package parser

import (
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/http/header"
	"github.com/unkn0wn-root/resterm/internal/http/version"
	"github.com/unkn0wn-root/resterm/internal/parser/bodyref"
	grpcbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/grpc"
	httpbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/http"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

func (b *documentBuilder) handleBlankLine(ln line) bool {
	if ln.text != "" {
		return false
	}
	if !b.inRequest {
		return true
	}

	if !b.request.http.HasMethod() {
		b.appendLine(ln.raw)
		return true
	}
	if !b.request.http.HeaderDone() {
		b.request.markHeadersDone()
		b.appendLine(ln.raw)
		return true
	}
	if b.request.protoBodyLine(ln.raw) {
		b.appendLine(ln.raw)
		return true
	}

	b.request.http.AppendBodyLine("", ln.eol)
	b.appendLine(ln.raw)
	return true
}

func (b *documentBuilder) handleBodyContinuation(ln line) bool {
	if b.inRequest && b.request.http.HasMethod() && b.request.http.HeaderDone() {
		b.handleBodyLine(ln)
		b.appendLine(ln.raw)
		return true
	}
	return false
}

// Blank lines fall through to handleBlankLine, which appends them to the body
// with their whitespace normalized away.
func (b *documentBuilder) handleMultipartBodyLine(ln line) bool {
	if ln.text == "" || !b.inRequest || b.request.multipart == nil ||
		!b.request.multipart.bodyLine(ln.text) {
		return false
	}
	b.request.http.AppendBodyLine(ln.raw, ln.eol)
	b.appendLine(ln.raw)
	return true
}

func (b *documentBuilder) handleMethodLine(ln line) bool {
	if grpcbuilder.IsMethodLine(ln.raw) {
		b.ensureRequest(ln.no)
		fields := strings.Fields(ln.raw)
		target := ""
		if len(fields) > 1 {
			target = strings.Join(fields[1:], " ")
		}

		b.request.http.SetMethodAndURL(strings.ToUpper(fields[0]), target)
		b.request.grpc.SetTarget(target)
		b.appendLine(ln.raw)
		return true
	}

	ml, ok, err := httpbuilder.ParseMethodLine(ln.raw)
	if !ok && err == nil {
		ml, ok, err = httpbuilder.ParseWebSocketURLLine(ln.raw)
	}
	switch {
	case err != nil:
		b.addError(ln.no, err.Error())
		b.appendLine(ln.raw)
		return true
	case ok:
		b.ensureRequest(ln.no)

		b.request.http.SetMethodAndURL(ml.Method, ml.URL)
		b.request.settings = version.SetIfMissing(b.request.settings, ml.Version)
		b.appendLine(ln.raw)
		return true
	}

	return false
}

func (b *documentBuilder) handleHeaderLine(ln line) bool {
	if !b.inRequest || !b.request.http.HasMethod() || b.request.http.HeaderDone() {
		return false
	}
	if before, after, ok := strings.Cut(ln.raw, ":"); ok {
		headerName := strings.TrimSpace(before)
		headerValue := strings.TrimSpace(after)
		// A valid header must win before a GraphQL or gRPC raw-block collector.
		// Both protocols accept arbitrary body lines, so offering the line to
		// them first used to swallow valid gRPC headers as protobuf JSON.
		if header.Valid(headerName) {
			b.request.http.AddHeader(headerName, headerValue)
			b.appendLine(ln.raw)
			return true
		}
		// Query/variables and protobuf JSON may contain colons. Give an invalid
		// header-shaped line to the active protocol before diagnosing it as a
		// request header.
		if b.request.protoBodyLine(ln.raw) {
			b.request.markHeadersDone()
			b.appendLine(ln.raw)
			return true
		}
		// A name that is not an HTTP field name cannot be sent, and header
		// names are never expanded, so nothing later can rescue it. Report it
		// here, where the line number points at the fix, and still record it so
		// the request stays what the file says.
		b.addError(ln.no, fmt.Sprintf("header name %q is not an HTTP field name", headerName))
		if headerName != "" {
			b.request.http.AddHeader(headerName, headerValue)
		}
	} else if b.request.protoBodyLine(ln.raw) {
		// A feature collecting a raw block takes non-header lines before the
		// ordinary body builder. This supports @query/@variables immediately
		// after the request line as well as protobuf JSON without a blank line.
		// The block also ends the header span, so a later query line such as a
		// GraphQL alias is never read back as a header.
		b.request.markHeadersDone()
		b.appendLine(ln.raw)
		return true
	}
	b.appendLine(ln.raw)
	return true
}

func (b *documentBuilder) handleBodyLine(ln line) {
	if b.request.protoBodyLine(ln.raw) {
		return
	}

	if file, ok := parseHTTPBodyFile(ln.raw, b.request.bodyOptions.ForceInline); ok {
		b.request.http.SetBodyFromFile(file)
		return
	}
	b.request.http.AppendBodyLine(ln.raw, ln.eol)
}

func parseHTTPBodyFile(line string, forceInline bool) (string, bool) {
	return bodyref.Parse(line, bodyref.Options{
		Location:    bodyref.Line,
		ForceInline: forceInline,
	})
}

func (r *requestBuilder) handleBodyDirective(rest string) bool {
	rs := str.Trim(rest)
	if rs == "" {
		return false
	}
	k, v := directive.CutKey(rs)
	if k == "" {
		return false
	}

	var opt *bool
	switch k {
	case "expand", "expand-templates":
		opt = &r.bodyOptions.ExpandTemplates
	case "inline", "raw":
		opt = &r.bodyOptions.ForceInline
	default:
		return false
	}

	enabled := true
	if str.Trim(v) != "" {
		b, ok := directive.ParseBool(v)
		if !ok {
			return false
		}
		enabled = b
	}
	*opt = enabled
	return true
}

func (r *requestBuilder) markHeadersDone() {
	if r.http.HeaderDone() {
		return
	}
	r.http.MarkHeadersDone()
	if ct := r.http.MimeType(); restfile.IsMultipartMime(ct) {
		r.multipart = newMultipartSpan(ct)
	}
}

// multipartSpan tracks the region between the first multipart delimiter and
// the close delimiter. Lines inside it are body content and must bypass the
// comment, script, and variable handlers, which would otherwise consume them
// (the "--" comment marker eats boundary lines, "#" eats part content, ...).
type multipartSpan struct {
	delimiter string // "--" + boundary; empty when Content-Type has no boundary param
	open      bool
	closed    bool
}

func newMultipartSpan(ct string) *multipartSpan {
	return &multipartSpan{delimiter: "--" + boundaryParam(ct)}
}

// bodyLine reports whether text is multipart body content: a delimiter
// line, or any line between the first delimiter and the close delimiter.
// Without a known boundary only "--" lines count, so comment-like part
// content is not protected but boundary lines still survive.
func (s *multipartSpan) bodyLine(text string) bool {
	if s.delimiter == "--" {
		return strings.HasPrefix(text, "--")
	}
	switch {
	case s.closed:
		return false
	case text == s.delimiter+"--":
		s.closed = true
		return true
	case text == s.delimiter:
		s.open = true
		return true
	default:
		return s.open
	}
}

func boundaryParam(ct string) string {
	params := strings.Split(ct, ";")
	for _, p := range params[1:] {
		k, v, ok := strings.Cut(p, "=")
		if ok && strings.EqualFold(strings.TrimSpace(k), "boundary") {
			return strings.Trim(strings.TrimSpace(v), `"`)
		}
	}
	return ""
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

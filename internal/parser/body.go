package parser

import (
	"net/http"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/httpver"
	"github.com/unkn0wn-root/resterm/internal/parser/bodyref"
	grpcbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/grpc"
	httpbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/http"
	dvalue "github.com/unkn0wn-root/resterm/internal/parser/directive/value"
	"github.com/unkn0wn-root/resterm/internal/parser/lexer"
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

	b.request.http.AppendBodyLine("")
	b.appendLine(ln.raw)
	return true
}

func (b *documentBuilder) handleBodyContinuation(ln line) bool {
	if b.inRequest && b.request.http.HasMethod() && b.request.http.HeaderDone() {
		b.handleBodyLine(ln.raw)
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
	b.request.http.AppendBodyLine(ln.raw)
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

	if method, url, ver, ok := httpbuilder.ParseMethodLine(ln.raw); ok {
		b.ensureRequest(ln.no)

		b.request.http.SetMethodAndURL(method, url)
		b.request.settings = httpver.SetIfMissing(b.request.settings, ver)
		b.appendLine(ln.raw)
		return true
	}

	if url, ok := httpbuilder.ParseWebSocketURLLine(ln.raw); ok {
		b.ensureRequest(ln.no)

		b.request.http.SetMethodAndURL(http.MethodGet, url)
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
		if headerName != "" {
			b.request.http.AddHeader(headerName, headerValue)
		}
	}
	b.appendLine(ln.raw)
	return true
}

func (b *documentBuilder) handleBodyLine(raw string) {
	if b.request.protoBodyLine(raw) {
		return
	}

	if file, ok := parseHTTPBodyFile(raw, b.request.bodyOptions.ForceInline); ok {
		b.request.http.SetBodyFromFile(file)
		return
	}
	b.request.http.AppendBodyLine(raw)
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
	k, v := lexer.SplitDirective(rs)
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
		b, ok := dvalue.ParseBool(v)
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

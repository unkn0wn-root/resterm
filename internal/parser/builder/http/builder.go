package http

import (
	stdhttp "net/http"
	"regexp"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/http/version"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

var methodRe = regexp.MustCompile(
	`^(?i)(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS|TRACE|CONNECT|WS|WSS)\b`,
)

func isMethodLine(line string) bool {
	return methodRe.MatchString(line)
}

// MethodLine contains the parsed parts of a request line.
type MethodLine struct {
	Method  string
	URL     string
	Version version.HTTP
}

// ParseMethodLine parses an HTTP request line.
func ParseMethodLine(line string) (ml MethodLine, ok bool, err error) {
	if !isMethodLine(line) {
		return MethodLine{}, false, nil
	}

	fields := strings.Fields(line)
	if len(fields) < 2 {
		return MethodLine{}, false, nil
	}

	urlFields, ver, err := version.SplitToken(fields[1:])
	if err != nil {
		return MethodLine{}, false, err
	}
	if len(urlFields) == 0 {
		return MethodLine{}, false, nil
	}

	method := str.UpperTrim(fields[0])
	if method == "WS" || method == "WSS" {
		method = stdhttp.MethodGet
	}
	return MethodLine{
		Method:  method,
		URL:     strings.Join(urlFields, " "),
		Version: ver,
	}, true, nil
}

func ParseWebSocketURLLine(line string) (url string, ok bool) {
	s := str.Trim(line)
	if s == "" {
		return "", false
	}
	lower := str.Lower(s)
	if strings.HasPrefix(lower, "ws://") || strings.HasPrefix(lower, "wss://") {
		return s, true
	}
	return "", false
}

type bodyLine struct {
	text string
	term string
}

type Builder struct {
	method       string
	url          string
	headers      stdhttp.Header
	headerDone   bool
	bodyLines    []bodyLine
	bodyFromFile string
	mimeType     string
}

func New() *Builder {
	return &Builder{}
}

func (b *Builder) HasMethod() bool {
	return b.method != ""
}

func (b *Builder) SetMethodAndURL(method, url string) {
	m := str.UpperTrim(method)
	if m == "WS" || m == "WSS" {
		m = stdhttp.MethodGet
	}
	b.method = m
	b.url = str.Trim(url)
}

func (b *Builder) Method() string {
	return b.method
}

func (b *Builder) URL() string {
	return b.url
}

func (b *Builder) Headers() stdhttp.Header {
	if b.headers == nil {
		b.headers = make(stdhttp.Header)
	}
	return b.headers
}

func (b *Builder) HeaderMap() stdhttp.Header {
	return b.headers
}

func (b *Builder) AddHeader(name, value string) {
	headers := b.Headers()
	headers.Add(name, value)
	if strings.EqualFold(name, "Content-Type") {
		b.mimeType = value
	}
}

func (b *Builder) HeaderDone() bool {
	return b.headerDone
}

func (b *Builder) MarkHeadersDone() {
	b.headerDone = true
}

func (b *Builder) AppendBodyLine(text, term string) {
	b.bodyLines = append(b.bodyLines, bodyLine{text: text, term: term})
}

func (b *Builder) SetBodyFromFile(path string) {
	b.bodyFromFile = str.Trim(path)
	b.bodyLines = nil
}

func (b *Builder) BodyFromFile() string {
	return b.bodyFromFile
}

// BodyText rebuilds the body with its original line endings.
// The final line ending is not part of the body.
func (b *Builder) BodyText() string {
	var sb strings.Builder
	for i, ln := range b.bodyLines {
		sb.WriteString(ln.text)
		if i < len(b.bodyLines)-1 {
			sb.WriteString(ln.term)
		}
	}
	return sb.String()
}

func (b *Builder) MimeType() string {
	return b.mimeType
}

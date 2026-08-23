package http

import (
	stdhttp "net/http"
	"regexp"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/http/version"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

// A method has to be followed by whitespace. A word boundary would also match
// the colon in "ws://host", which reads that scheme as the WS method.
var methodRe = regexp.MustCompile(
	`^(?i)(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS|TRACE|CONNECT|WS|WSS)(\s|$)`,
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

// ParseRequestLine parses a method request or a bare HTTP/WebSocket URL.
func ParseRequestLine(line string) (MethodLine, bool, error) {
	if ml, ok, err := ParseMethodLine(line); ok || err != nil {
		return ml, ok, err
	}
	return parseURLLine(line, requestSchemes)
}

// ParseWebSocketURLLine parses a bare WebSocket request line.
func ParseWebSocketURLLine(line string) (MethodLine, bool, error) {
	return parseURLLine(line, webSocketSchemes)
}

var (
	requestSchemes   = []string{"http://", "https://", "ws://", "wss://"}
	webSocketSchemes = []string{"ws://", "wss://"}
)

func parseURLLine(line string, schemes []string) (MethodLine, bool, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return MethodLine{}, false, nil
	}

	urlFields, ver, err := version.SplitToken(fields)
	// Check the scheme before reporting a version error so prose ending in
	// HTTP/<number> is not mistaken for a request.
	url := strings.Trim(strings.Join(urlFields, " "), `"'`)
	if !hasScheme(url, schemes) {
		return MethodLine{}, false, nil
	}
	if err != nil {
		return MethodLine{}, false, err
	}
	return MethodLine{Method: stdhttp.MethodGet, URL: url, Version: ver}, true, nil
}

func hasScheme(url string, schemes []string) bool {
	lower := str.Lower(url)
	for _, scheme := range schemes {
		if strings.HasPrefix(lower, scheme) {
			return true
		}
	}
	return false
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

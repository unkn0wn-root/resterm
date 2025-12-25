package httpbuilder

import (
	"net/http"
	"regexp"
	"strings"
)

var methodRe = regexp.MustCompile(
	`^(?i)(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS|TRACE|CONNECT|WS|WSS)\b`,
)

func IsMethodLine(line string) bool {
	return methodRe.MatchString(line)
}

func ParseMethodLine(line string) (method string, url string, ok bool) {
	if !IsMethodLine(line) {
		return "", "", false
	}

	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", "", false
	}

	method = strings.ToUpper(fields[0])
	if method == "WS" || method == "WSS" {
		method = http.MethodGet
	}
	url = strings.Join(fields[1:], " ")
	return method, url, true
}

func ParseWebSocketURLLine(line string) (url string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", false
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "ws://") || strings.HasPrefix(lower, "wss://") {
		return trimmed, true
	}
	return "", false
}

type Builder struct {
	method       string
	url          string
	headers      http.Header
	headerDone   bool
	bodyLines    []string
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
	m := strings.ToUpper(strings.TrimSpace(method))
	if m == "WS" || m == "WSS" {
		m = http.MethodGet
	}
	b.method = m
	b.url = strings.TrimSpace(url)
}

func (b *Builder) Method() string {
	return b.method
}

func (b *Builder) URL() string {
	return b.url
}

func (b *Builder) Headers() http.Header {
	if b.headers == nil {
		b.headers = make(http.Header)
	}
	return b.headers
}

func (b *Builder) HeaderMap() http.Header {
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

func (b *Builder) AppendBodyLine(line string) {
	b.bodyLines = append(b.bodyLines, line)
}

func (b *Builder) SetBodyFromFile(path string) {
	b.bodyFromFile = strings.TrimSpace(path)
	b.bodyLines = nil
}

func (b *Builder) BodyFromFile() string {
	return b.bodyFromFile
}

func (b *Builder) BodyText() string {
	if len(b.bodyLines) == 0 {
		return ""
	}
	return strings.Join(b.bodyLines, "\n")
}

func (b *Builder) MimeType() string {
	return b.mimeType
}

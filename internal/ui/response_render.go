package ui

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

const (
	responseFormattingBase       = "Formatting response"
	responseReflowingMessage     = "Reflowing response..."
	defaultResponseViewportWidth = 80
)

type cachedWrap struct {
	width   int
	content string
	valid   bool
}

type responseRenderedMsg struct {
	token          string
	pretty         string
	raw            string
	headers        string
	width          int
	prettyWrapped  string
	rawWrapped     string
	headersWrapped string
}

type responseWrapMsg struct {
	token          string
	width          int
	prettyWrapped  string
	rawWrapped     string
	headersWrapped string
}

var responseRenderSeq uint64

func nextResponseRenderToken() string {
	id := atomic.AddUint64(&responseRenderSeq, 1)
	return fmt.Sprintf("render-%d", id)
}

func renderHTTPResponseCmd(token string, resp *httpclient.Response, tests []scripts.TestResult, scriptErr error, width int) tea.Cmd {
	if resp == nil {
		return nil
	}

	respCopy := cloneHTTPResponse(resp)
	testsCopy := append([]scripts.TestResult(nil), tests...)

	if width <= 0 {
		width = defaultResponseViewportWidth
	}

	targetWidth := width

	return func() tea.Msg {
		pretty, raw, headers := buildHTTPResponseViews(respCopy, testsCopy, scriptErr)
		return responseRenderedMsg{
			token:          token,
			pretty:         pretty,
			raw:            raw,
			headers:        headers,
			width:          targetWidth,
			prettyWrapped:  wrapToWidth(pretty, targetWidth),
			rawWrapped:     wrapToWidth(raw, targetWidth),
			headersWrapped: wrapToWidth(headers, targetWidth),
		}
	}
}

func wrapResponseContentCmd(token string, pretty, raw, headers string, width int) tea.Cmd {
	if width <= 0 {
		width = defaultResponseViewportWidth
	}

	targetWidth := width
	prettyContent := pretty
	rawContent := raw
	headersContent := headers

	return func() tea.Msg {
		return responseWrapMsg{
			token:          token,
			width:          targetWidth,
			prettyWrapped:  wrapToWidth(prettyContent, targetWidth),
			rawWrapped:     wrapToWidth(rawContent, targetWidth),
			headersWrapped: wrapToWidth(headersContent, targetWidth),
		}
	}
}

func cloneHTTPResponse(resp *httpclient.Response) *httpclient.Response {
	if resp == nil {
		return nil
	}
	var headers http.Header
	if resp.Headers != nil {
		headers = make(http.Header, len(resp.Headers))
		for key, values := range resp.Headers {
			copied := append([]string(nil), values...)
			headers[key] = copied
		}
	}
	body := append([]byte(nil), resp.Body...)
	return &httpclient.Response{
		Status:       resp.Status,
		StatusCode:   resp.StatusCode,
		Proto:        resp.Proto,
		Headers:      headers,
		Body:         body,
		Duration:     resp.Duration,
		EffectiveURL: resp.EffectiveURL,
		Request:      resp.Request,
	}
}

func buildHTTPResponseViews(resp *httpclient.Response, tests []scripts.TestResult, scriptErr error) (string, string, string) {
	if resp == nil {
		return noResponseMessage, noResponseMessage, noResponseMessage
	}

	summary := buildResponseSummary(resp, tests, scriptErr)
	headersContent := formatHTTPHeaders(resp.Headers)

	contentType := ""
	if resp.Headers != nil {
		contentType = resp.Headers.Get("Content-Type")
	}

	prettyBodyRaw := prettifyBody(resp.Body, contentType)
	prettyBody := trimResponseBody(prettyBodyRaw)
	if isBodyEmpty(prettyBody) {
		prettyBody = "<empty>"
	}

	rawBody := trimResponseBody(string(resp.Body))
	if isBodyEmpty(rawBody) {
		rawBody = "<empty>"
	}

	headersSection := ""
	if headersContent != "" {
		headersSection = "Headers:\n" + headersContent
	}

	prettyView := joinSections(summary, prettyBody)
	rawView := joinSections(summary, rawBody)
	headersView := joinSections(summary, headersSection)

	return prettyView, rawView, headersView
}

func formatHTTPHeaders(headers http.Header) string {
	if len(headers) == 0 {
		return ""
	}
	builder := strings.Builder{}
	for name, values := range headers {
		builder.WriteString(fmt.Sprintf("%s: %s\n", name, strings.Join(values, ", ")))
	}
	return strings.TrimRight(builder.String(), "\n")
}

func trimResponseBody(body string) string {
	return strings.TrimRight(body, "\n")
}

func isBodyEmpty(body string) bool {
	return strings.TrimSpace(stripANSIEscape(body)) == ""
}

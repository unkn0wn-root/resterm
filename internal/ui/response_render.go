package ui

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
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
	base    string
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
			prettyWrapped:  wrapContentForTab(responseTabPretty, pretty, targetWidth),
			rawWrapped:     wrapContentForTab(responseTabRaw, raw, targetWidth),
			headersWrapped: wrapContentForTab(responseTabHeaders, headers, targetWidth),
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
	coloredHeaders := formatHTTPHeaders(resp.Headers, true)

	contentType := ""
	if resp.Headers != nil {
		contentType = resp.Headers.Get("Content-Type")
	}

	prettyBodyRaw := prettifyBody(resp.Body, contentType)
	prettyBody := trimResponseBody(prettyBodyRaw)
	if isBodyEmpty(prettyBody) {
		prettyBody = "<empty>"
	}

	rawBody := formatRawBody(resp.Body, contentType)
	if isBodyEmpty(rawBody) {
		rawBody = "<empty>"
	}

	headersSectionColored := ""
	if coloredHeaders != "" {
		headersSectionColored = statsHeadingStyle.Render("Headers:") + "\n" + coloredHeaders
	}

	coloredSummary := summary
	plainSummary := stripANSIEscape(summary)

	prettyView := joinSections(coloredSummary, prettyBody)
	rawView := joinSections(plainSummary, rawBody)
	headersView := joinSections(coloredSummary, headersSectionColored)

	return prettyView, rawView, headersView
}

func formatRawBody(body []byte, contentType string) string {
	raw := trimResponseBody(string(body))
	formatted, ok := indentRawBody(body, contentType)
	if !ok {
		return raw
	}
	return trimResponseBody(formatted)
}

func indentRawBody(body []byte, contentType string) (string, bool) {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "json"):
		var buf bytes.Buffer
		if err := json.Indent(&buf, body, "", "  "); err == nil {
			return buf.String(), true
		}
	case strings.Contains(ct, "xml"):
		if formatted, ok := indentXML(body); ok {
			return formatted, true
		}
	}
	return "", false
}

func indentXML(body []byte) (string, bool) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)
	encoder.Indent("", "  ")
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", false
		}
		if err := encoder.EncodeToken(tok); err != nil {
			return "", false
		}
	}
	if err := encoder.Flush(); err != nil {
		return "", false
	}
	return buf.String(), true
}

func formatHTTPHeaders(headers http.Header, colored bool) string {
	if len(headers) == 0 {
		return ""
	}
	keys := make([]string, 0, len(headers))
	for name := range headers {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	builder := strings.Builder{}
	for _, name := range keys {
		values := append([]string(nil), headers[name]...)
		sort.Strings(values)
		joined := strings.Join(values, ", ")
		if colored {
			if strings.TrimSpace(joined) == "" {
				builder.WriteString(statsLabelStyle.Render(name + ":"))
			} else {
				builder.WriteString(renderLabelValue(name, joined, statsLabelStyle, statsHeaderValueStyle))
			}
		} else {
			if strings.TrimSpace(joined) == "" {
				builder.WriteString(fmt.Sprintf("%s:", name))
			} else {
				builder.WriteString(fmt.Sprintf("%s: %s", name, joined))
			}
		}
		builder.WriteString("\n")
	}
	return strings.TrimRight(builder.String(), "\n")
}

func trimResponseBody(body string) string {
	return strings.TrimRight(body, "\n")
}

func isBodyEmpty(body string) bool {
	return strings.TrimSpace(stripANSIEscape(body)) == ""
}

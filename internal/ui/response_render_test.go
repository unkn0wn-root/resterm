package ui

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
	"github.com/unkn0wn-root/resterm/internal/bodyfmt"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

func TestRenderHTTPResponseCmdRawWrappedPreservesRawBody(t *testing.T) {
	body := []byte("{\"value\":\"" + strings.Repeat("a", 48) + "\"}")
	resp := &httpclient.Response{
		Status:       "200 OK",
		StatusCode:   200,
		Headers:      http.Header{"Content-Type": {"application/json"}},
		Body:         body,
		Duration:     12 * time.Millisecond,
		EffectiveURL: "https://example.com/items",
	}

	model := New(Config{})
	cmd := model.respFmtCmd(context.Background(), "token", resp, nil, nil, 12)
	if cmd == nil {
		t.Fatalf("expected command")
	}

	msgVal := cmd()
	msg, ok := msgVal.(responseRenderedMsg)
	if !ok {
		t.Fatalf("unexpected message type %T", msgVal)
	}

	wrapped := wrapContentForTab(responseTabRaw, msg.raw, 12)
	lines := strings.Split(wrapped, "\n")
	var (
		indent      string
		indentIndex = -1
	)
	for i, line := range lines {
		if strings.Contains(line, "\"value\"") {
			for _, r := range line {
				if r == ' ' || r == '\t' {
					indent += string(r)
					continue
				}
				break
			}
			indentIndex = i
			break
		}
	}
	if indentIndex == -1 {
		t.Fatalf("expected wrapped content to include value line, got %v", lines)
	}
	if indent == "" {
		t.Fatalf("expected value line to be indented, got %q", lines[indentIndex])
	}
	if indentIndex+1 >= len(lines) {
		t.Fatalf("expected continuation line after value segment, got %v", lines)
	}
	if !strings.HasPrefix(lines[indentIndex+1], indent) {
		t.Fatalf(
			"expected continuation line to retain indentation %q, got %q",
			indent,
			lines[indentIndex+1],
		)
	}
}

func TestBuildHTTPResponseViewsPreservesLeadingWhitespace(t *testing.T) {
	body := []byte("  leading line\n    indented line")
	resp := &httpclient.Response{
		Status:       "200 OK",
		StatusCode:   200,
		Headers:      http.Header{"Content-Type": {"text/plain"}},
		Body:         body,
		Duration:     5 * time.Millisecond,
		EffectiveURL: "https://example.com/whitespace",
	}

	views := buildHTTPResponseViews(resp, nil, nil)
	pretty, raw := views.pretty, views.raw
	if !strings.Contains(pretty, "  leading line") {
		t.Fatalf("expected pretty view to retain leading spaces, got %q", pretty)
	}
	if !strings.Contains(raw, "  leading line") {
		t.Fatalf("expected raw view to retain leading spaces, got %q", raw)
	}
}

func TestBuildHTTPResponseViewsColorsSummaryExceptRaw(t *testing.T) {
	resp := &httpclient.Response{
		Status:     "201 Created",
		StatusCode: 201,
		Headers: http.Header{
			"Content-Type": {"application/json"},
			"X-Demo":       {"value"},
		},
		Body:         []byte(`{"id":1}`),
		Duration:     3 * time.Millisecond,
		EffectiveURL: "https://api.example.com/items",
	}

	views := buildHTTPResponseViews(resp, nil, nil)
	pretty, raw, headers := views.pretty, views.raw, views.headers
	if !strings.Contains(pretty, statsLabelStyle.Render("Status:")) {
		t.Fatalf("expected colored status label, got %q", pretty)
	}
	if !strings.Contains(pretty, statsSuccessStyle.Render("201 Created")) {
		t.Fatalf("expected colored status value, got %q", pretty)
	}
	if !strings.Contains(pretty, statsDurationStyle.Render("3ms")) {
		t.Fatalf("expected colored duration value, got %q", pretty)
	}
	if strings.Contains(raw, "\x1b[") {
		t.Fatalf("expected raw view without ANSI codes, got %q", raw)
	}
	if !strings.Contains(headers, statsHeadingStyle.Render("Response headers")) {
		t.Fatalf("expected colored response headers heading, got %q", headers)
	}
	if !strings.Contains(headers, statsLabelStyle.Render("Content-Type")) {
		t.Fatalf("expected colored header names, got %q", headers)
	}
	if !strings.Contains(headers, statsHeaderValueStyle.Render("application/json")) {
		t.Fatalf("expected colored header values, got %q", headers)
	}
}

func TestBuildHTTPResponseViewsWithLightPaletteUsesReadableSummaryStyles(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prevProfile)

	body := []byte(`{"id":1,"name":"demo"}`)
	resp := &httpclient.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Headers: http.Header{
			"Content-Type": {"application/json"},
			"X-Demo":       {"value"},
		},
		Body:         body,
		Duration:     8 * time.Millisecond,
		EffectiveURL: "https://api.example.com/items",
	}

	lightTheme := theme.DefaultTheme()
	lightTheme.ExplainMuted = lipgloss.NewStyle().Foreground(lipgloss.Color("#475569"))
	lightTheme.ExplainLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("#0369a1"))
	lightTheme.HeaderTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("#1d4ed8"))
	lightTheme.HeaderValue = lipgloss.NewStyle().Foreground(lipgloss.Color("#334155"))
	lightTheme.StatusBarKey = lipgloss.NewStyle().Foreground(lipgloss.Color("#b45309"))
	lightTheme.Success = lipgloss.NewStyle().Foreground(lipgloss.Color("#15803d"))
	lightTheme.Error = lipgloss.NewStyle().Foreground(lipgloss.Color("#b91c1c"))
	lightTheme.ResponseSelection = lipgloss.NewStyle().Background(lipgloss.Color("#e2e8f0"))
	lightTheme.PaneActiveForeground = lipgloss.Color("#0f172a")

	renderer := newResponseRenderer(lightStatsPalette(lightTheme), "github")
	views := renderer.buildHTTPResponseViews(resp, nil, nil)
	contentLength := formatByteSize(int64(len(body)))

	if !strings.Contains(views.pretty, renderer.stats.Value.Render(resp.EffectiveURL)) {
		t.Fatalf("expected light palette URL style, got %q", views.pretty)
	}
	if strings.Contains(views.pretty, statsValueStyle.Render(resp.EffectiveURL)) {
		t.Fatalf("expected light palette URL to avoid dark default style, got %q", views.pretty)
	}
	if !strings.Contains(views.pretty, renderer.stats.Value.Render(contentLength)) {
		t.Fatalf("expected light palette content length style, got %q", views.pretty)
	}
	if !strings.Contains(views.headers, renderer.stats.HeaderValue.Render("application/json")) {
		t.Fatalf("expected light palette header value style, got %q", views.headers)
	}
	if strings.Contains(views.headers, statsHeaderValueStyle.Render("application/json")) {
		t.Fatalf(
			"expected light palette headers to avoid dark default style, got %q",
			views.headers,
		)
	}
}

func TestBuildHTTPRequestHeadersViewUsesExecutedRequest(t *testing.T) {
	resp := &httpclient.Response{
		ReqMethod:    "GET",
		EffectiveURL: "https://final.example.com/items",
		Request: &restfile.Request{
			Method: "POST",
			URL:    "https://{{env}}/items",
		},
		RequestHeaders: http.Header{"X-Test": {"1"}},
	}

	view := buildHTTPRequestHeadersView(resp)
	plain := stripANSIEscape(view)
	if !strings.Contains(plain, "GET https://final.example.com/items") {
		t.Fatalf("expected request line to use executed method/url, got %q", plain)
	}
	if strings.Contains(plain, "{{env}}") {
		t.Fatalf("expected expanded URL to omit template placeholder, got %q", plain)
	}
	if !strings.Contains(plain, "Request headers") {
		t.Fatalf("expected request headers heading, got %q", plain)
	}
}

func TestHeaderPanelRuleUsesTitleWidth(t *testing.T) {
	renderer := defaultResponseRenderer()
	view := stripANSIEscape(renderer.renderHeaderPanel(
		"Response headers",
		[]bodyfmt.HeaderField{{Name: "X-Test", Value: "ok"}},
		"empty",
	))
	lines := strings.Split(view, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected heading and rule, got %q", view)
	}
	if got, want := lines[1], strings.Repeat("─", len("Response headers")); got != want {
		t.Fatalf("expected rule %q, got %q", want, got)
	}
	if strings.Contains(lines[1], "1 header") {
		t.Fatalf("rule should not include count text, got %q", lines[1])
	}
}

func TestHeaderPanelAlignsSeparatorToLongestName(t *testing.T) {
	renderer := defaultResponseRenderer()
	view := stripANSIEscape(renderer.renderHeaderPanel(
		"Response headers",
		[]bodyfmt.HeaderField{
			{Name: "Access-Control-Allow-Origin", Value: "*"},
			{Name: "Access-Control-Allow-Credentials", Value: "true"},
		},
		"empty",
	))
	lines := strings.Split(view, "\n")
	var cols []int
	for _, line := range lines {
		idx := strings.IndexRune(line, '│')
		if idx >= 0 {
			cols = append(cols, idx)
		}
	}
	if len(cols) != 2 {
		t.Fatalf("expected two header rows with separators, got %q", view)
	}
	if cols[0] != cols[1] {
		t.Fatalf("expected aligned separators, got columns %v in %q", cols, view)
	}
}

func TestBuildRequestHeaderMapAddsDefaults(t *testing.T) {
	resp := &httpclient.Response{
		ReqMethod: "GET",
		ReqHost:   "example.com",
	}
	hdrs := buildRequestHeaderMap(resp)
	if hdrs.Get("Host") != "example.com" {
		t.Fatalf("expected host to be populated from request host, got %q", hdrs.Get("Host"))
	}
}

func TestBinaryResponsesUseSummaryAndHexRaw(t *testing.T) {
	body := []byte{0x00, 0x01, 0x02, 0x03}
	resp := &httpclient.Response{
		Status:       "200 OK",
		StatusCode:   200,
		Headers:      http.Header{"Content-Type": {"application/octet-stream"}},
		Body:         body,
		Duration:     10 * time.Millisecond,
		EffectiveURL: "https://example.com/download/file.bin",
	}

	views := buildHTTPResponseViews(resp, nil, nil)
	pretty, raw, rawText, rawHex, rawBase64, mode := views.pretty, views.raw, views.rawText, views.rawHex, views.rawBase64, views.rawMode
	if mode != rawViewHex {
		t.Fatalf("expected binary responses to default to hex raw mode")
	}
	if rawHex != "" && !strings.Contains(raw, rawHex) {
		t.Fatalf("expected raw view to include hex dump, got %q", raw)
	}
	if rawHex != binaryview.HexDump(body, 16) {
		t.Fatalf("unexpected hex dump, got %q", rawHex)
	}
	if rawText == rawHex {
		t.Fatalf("expected raw text to differ from hex view")
	}
	if rawBase64 == "" {
		t.Fatalf("expected base64 preview to be populated")
	}
	if !strings.Contains(pretty, "Binary body") {
		t.Fatalf("expected pretty view to show binary summary, got %q", pretty)
	}
}

func TestBinaryBodySummaryKeepsOriginalUILabelAndStyling(t *testing.T) {
	body := []byte{0x00, 0x01, 0x02, 0x03}
	meta := binaryview.Analyze(body, "application/octet-stream")

	views := buildBodyViews(body, "application/octet-stream", &meta, nil, "")
	want := renderLabelValue("Raw tab", "hex / base64", statsLabelStyle, statsValueStyle)
	if !strings.Contains(views.pretty, want) {
		t.Fatalf("expected original UI label rendering, got %q", views.pretty)
	}
	if strings.Contains(
		views.pretty,
		renderLabelValue("Raw view", "hex / base64", statsLabelStyle, statsValueStyle),
	) {
		t.Fatalf("expected binary summary to avoid renamed label, got %q", views.pretty)
	}
}

func TestHeavyBinaryDefaultsToSummary(t *testing.T) {
	body := bytes.Repeat([]byte{0x00, 0xff}, rawHeavyLimit/2+1)
	resp := &httpclient.Response{
		Status:       "200 OK",
		StatusCode:   200,
		Headers:      http.Header{"Content-Type": {"application/octet-stream"}},
		Body:         body,
		Duration:     10 * time.Millisecond,
		EffectiveURL: "https://example.com/download/file.bin",
	}

	views := buildHTTPResponseViews(resp, nil, nil)
	if views.rawMode != rawViewSummary {
		t.Fatalf("expected heavy binary to default to summary mode")
	}
	if views.rawHex != "" || views.rawBase64 != "" {
		t.Fatalf("expected heavy binary dumps to be deferred")
	}
	if !strings.Contains(views.raw, "<raw dump deferred>") {
		t.Fatalf("expected raw summary placeholder, got %q", views.raw)
	}
}

func TestPrintableOctetStreamDefaultsToText(t *testing.T) {
	body := []byte("plain text body")
	resp := &httpclient.Response{
		Status:       "200 OK",
		StatusCode:   200,
		Headers:      http.Header{"Content-Type": {"application/octet-stream"}},
		Body:         body,
		Duration:     5 * time.Millisecond,
		EffectiveURL: "https://example.com/download",
	}

	views := buildHTTPResponseViews(resp, nil, nil)
	pretty, raw, rawText, rawHex, mode := views.pretty, views.raw, views.rawText, views.rawHex, views.rawMode
	if mode != rawViewText {
		t.Fatalf("expected raw mode to default to text for printable octet-stream")
	}
	if strings.Contains(pretty, "Binary body") {
		t.Fatalf("expected pretty view to render text, got %q", pretty)
	}
	if !strings.Contains(raw, "plain text body") {
		t.Fatalf("expected raw view to include body text, got %q", raw)
	}
	if rawHex == "" {
		t.Fatalf("expected hex dump to remain available")
	}
	if rawText == "" {
		t.Fatalf("expected raw text to be populated")
	}
}

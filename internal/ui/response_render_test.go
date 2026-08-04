package ui

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
	"github.com/unkn0wn-root/resterm/internal/bodyfmt"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

func TestRenderHTTPResponseCmdRawWrappedPreservesRawBody(t *testing.T) {
	body := []byte("{\"value\":\"" + strings.Repeat("a", 48) + "\"}")
	resp := &httpx.Response{
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
	resp := &httpx.Response{
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
	resp := &httpx.Response{
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
	if strings.Contains(stripANSIEscape(headers), "Response headers") {
		t.Fatalf("expected response header pane title to be omitted, got %q", headers)
	}
	if !strings.Contains(headers, statsNeutralStyle.Render("2 HEADERS")) {
		t.Fatalf("expected colored response header count, got %q", headers)
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
	resp := &httpx.Response{
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

	renderer := newResponseRenderer(lightStatsPalette(lightTheme), "github", eSty{})
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

func TestBuildGRPCResponseViewsColorsPrettyStatusAndDetailsOnly(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prevProfile)

	const (
		method = "/pkg.Service/Fail"
		detail = `{"@type":"type.googleapis.com/google.rpc.BadRequest","field":"page_size"}`
	)
	resp := &grpcx.Response{
		ContentType:   "application/json",
		StatusCode:    codes.InvalidArgument,
		StatusMessage: "invalid page size",
		StatusDetails: []string{detail},
	}
	renderer := defaultResponseRenderer()
	views := renderer.buildGRPCResponseViews(resp, method)

	if !strings.Contains(
		views.pretty,
		renderer.stats.Warn.Render(resp.StatusText()),
	) {
		t.Fatalf("expected colored gRPC status in pretty view, got %q", views.pretty)
	}
	prettyDetail := bodyfmt.Prettify(
		context.Background(),
		[]byte(detail),
		"application/json",
		bodyfmt.PrettyOptions{Color: termcolor.TrueColor(), Style: renderer.syntaxStyle},
	)
	if !strings.Contains(views.pretty, strings.TrimRight(prettyDetail, "\r\n")) {
		t.Fatalf("expected syntax-colored gRPC detail in pretty view, got %q", views.pretty)
	}
	if strings.Contains(views.raw, "\x1b[") {
		t.Fatalf("expected raw gRPC view without ANSI codes, got %q", views.raw)
	}
	if !strings.Contains(views.raw, detail) {
		t.Fatalf("expected raw gRPC view to preserve detail JSON, got %q", views.raw)
	}
	if !strings.Contains(views.headers, renderer.renderGRPCStatusLine(resp, method)) {
		t.Fatalf("expected colored gRPC status in headers view, got %q", views.headers)
	}
	if views.rawSummary != renderer.grpcStatusBlock(resp, method) {
		t.Fatalf(
			"raw summary = %q, want plain status block %q",
			views.rawSummary,
			renderer.grpcStatusBlock(resp, method),
		)
	}
	// The plain line is stripped from the styled one, so pin the wording here.
	wantLine := "gRPC pkg.Service/Fail - " + resp.StatusText()
	if got := renderer.grpcStatusLine(resp, method); got != wantLine {
		t.Fatalf("grpcStatusLine() = %q, want %q", got, wantLine)
	}
}

func TestRenderGRPCResponseHeadersEncodesBinaryMetadata(t *testing.T) {
	st, err := status.New(codes.InvalidArgument, "invalid page size").WithDetails(
		&errdetails.BadRequest{
			FieldViolations: []*errdetails.BadRequest_FieldViolation{{
				Field:       "page_size",
				Description: "must be between 1 and 1000",
			}},
		},
	)
	if err != nil {
		t.Fatalf("build gRPC status: %v", err)
	}
	wire, err := proto.Marshal(st.Proto())
	if err != nil {
		t.Fatalf("marshal gRPC status: %v", err)
	}

	resp := &grpcx.Response{
		StatusCode:    codes.InvalidArgument,
		StatusMessage: "invalid page size",
		Trailers: map[string][]string{
			"content-type":            {"application/grpc"},
			"grpc-status-details-bin": {string(wire)},
		},
	}
	const width = 64
	view := stripANSIEscape(defaultResponseRenderer().renderGRPCRespHdrs(
		resp,
		"/pkg.Service/Fail",
		width,
	))
	if !strings.Contains(view, "grpc-status-details-bin:") {
		t.Fatalf("expected binary trailer name, got %q", view)
	}
	encoded := base64.RawStdEncoding.EncodeToString(wire)
	if !strings.Contains(view, encoded[:16]) {
		t.Fatalf("expected base64 binary trailer prefix %q, got %q", encoded[:16], view)
	}
	for _, r := range view {
		if unicode.IsControl(r) && r != '\n' {
			t.Fatalf("rendered binary metadata contains control character %U in %q", r, view)
		}
	}
	for line := range strings.SplitSeq(view, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("rendered header width = %d, want <= %d in %q", got, width, line)
		}
	}
}

func TestBuildHTTPRequestHeadersViewUsesExecutedRequest(t *testing.T) {
	resp := &httpx.Response{
		ReqMethod:    "GET",
		EffectiveURL: "https://final.example.com/items",
		Request: &restfile.Request{
			Method: "POST",
			URL:    "https://{{env}}/items",
		},
		RequestHeaders: http.Header{"X-Test": {"1"}},
	}

	view := defaultResponseRenderer().renderHTTPReqHdrs(resp, defaultResponseViewportWidth)
	plain := stripANSIEscape(view)
	if !strings.Contains(plain, "GET https://final.example.com/items") {
		t.Fatalf("expected request line to use executed method/url, got %q", plain)
	}
	if strings.Contains(plain, "{{env}}") {
		t.Fatalf("expected expanded URL to omit template placeholder, got %q", plain)
	}
	if strings.Contains(plain, "Request headers") {
		t.Fatalf("expected request header pane title to be omitted, got %q", plain)
	}
	if !strings.Contains(plain, "1 HEADER") {
		t.Fatalf("expected request header count, got %q", plain)
	}
}

func TestGRPCRequestHeaderMapEncodesBinaryMetadata(t *testing.T) {
	raw := string([]byte{0x00, 0x01, 0xff})
	req := &restfile.Request{
		GRPC: &restfile.GRPCRequest{
			Metadata: []restfile.MetadataPair{{Key: "trace-bin", Value: raw}},
		},
	}

	want := base64.RawStdEncoding.EncodeToString([]byte(raw))
	if got := grpcRequestHeaderMap(req).Get("trace-bin"); got != want {
		t.Fatalf("trace-bin = %q, want %q", got, want)
	}
	if req.GRPC.Metadata[0].Value != raw {
		t.Fatalf("grpcRequestHeaderMap mutated request metadata: %q", req.GRPC.Metadata[0].Value)
	}
}

func TestHeaderPanelRuleUsesAvailableWidth(t *testing.T) {
	renderer := defaultResponseRenderer()
	view := stripANSIEscape(renderer.renderHeaderPanel(
		"",
		[]bodyfmt.HeaderField{
			{Name: "X", Value: "short"},
			{Name: "Longer", Value: strings.Repeat("v", 40)},
		},
		"empty",
		32,
	))
	lines := strings.Split(view, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected heading and rows, got %q", view)
	}
	if got := lipgloss.Width(lines[0]); got != 32 {
		t.Fatalf("expected heading width 32, got %d in %q", got, lines[0])
	}
	if !strings.HasPrefix(lines[0], "2 HEADERS ") || !strings.Contains(lines[0], "─") {
		t.Fatalf("expected count embedded in rule, got %q", lines[0])
	}
	if strings.Contains(lines[0], "vvvv") && lipgloss.Width(lines[0]) > 32 {
		t.Fatalf("heading should not depend on longest value, got %q", lines[0])
	}
}

func TestHeaderPanelEmptyEmbedsCountInRule(t *testing.T) {
	renderer := defaultResponseRenderer()
	view := stripANSIEscape(renderer.renderHeaderPanel("", nil, "empty", 24))
	lines := strings.Split(view, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected heading and empty message, got %q", view)
	}
	if got := lipgloss.Width(lines[0]); got != 24 {
		t.Fatalf("expected heading width 24, got %d in %q", got, lines[0])
	}
	if !strings.HasPrefix(lines[0], "0 HEADERS ") || !strings.Contains(lines[0], "─") {
		t.Fatalf("expected count embedded in rule, got %q", lines[0])
	}
	if got, want := lines[1], "empty"; got != want {
		t.Fatalf("expected empty message %q, got %q", want, got)
	}
}

func TestHeaderPanelWrappedValueKeepsValueIndent(t *testing.T) {
	renderer := defaultResponseRenderer()
	view := stripANSIEscape(renderer.renderHeaderPanel(
		"",
		[]bodyfmt.HeaderField{
			{Name: "X-Test", Value: "alpha beta gamma delta"},
		},
		"empty",
		18,
	))
	lines := strings.Split(view, "\n")
	if len(lines) < 4 {
		t.Fatalf("expected wrapped header value, got %q", view)
	}
	first := lines[1]
	next := lines[2]
	indent := strings.Repeat(" ", len("X-Test: "))
	if !strings.HasPrefix(first, "X-Test: ") {
		t.Fatalf("expected first row to start with header name, got %q", first)
	}
	if !strings.HasPrefix(next, indent) {
		t.Fatalf("expected continuation indent %q, got %q in %q", indent, next, view)
	}
}

func TestHeaderPanelLongNameStacksValue(t *testing.T) {
	renderer := defaultResponseRenderer()
	view := stripANSIEscape(renderer.renderHeaderPanel(
		"",
		[]bodyfmt.HeaderField{
			{Name: "Access-Control-Allow-Credentials", Value: "true"},
		},
		"empty",
		20,
	))
	lines := strings.Split(view, "\n")
	if len(lines) < 4 {
		t.Fatalf("expected long name to stack value, got %q", view)
	}
	if strings.Contains(view, "│") || strings.Contains(view, "┬") {
		t.Fatalf("expected explain-style fields without table separators, got %q", view)
	}
	if !strings.HasPrefix(lines[len(lines)-1], "  true") {
		t.Fatalf("expected stacked value indentation, got %q in %q", lines[len(lines)-1], view)
	}
}

func TestBuildRequestHeaderMapAddsDefaults(t *testing.T) {
	resp := &httpx.Response{
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
	resp := &httpx.Response{
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

	views := defaultResponseRenderer().buildBodyViewsCtx(context.Background(), body, "application/octet-stream", &meta, nil, "")
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
	resp := &httpx.Response{
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
	resp := &httpx.Response{
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

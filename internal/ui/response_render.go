package ui

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

const (
	responseFormattingBase       = "Formatting response"
	responseReflowingMessage     = "Reflowing response..."
	defaultResponseViewportWidth = 80
)

const (
	compareColEnvWidth      = 11
	compareColStatusWidth   = 13
	compareColCodeWidth     = 6
	compareColDurationWidth = 10
	compareColumnGap        = "  "
)

type cachedWrap struct {
	width   int
	content string
	base    string
	valid   bool
}

type responseRenderedMsg struct {
	token                 string
	pretty                string
	raw                   string
	rawSummary            string
	headers               string
	requestHeaders        string
	width                 int
	prettyWrapped         string
	rawWrapped            string
	headersWrapped        string
	requestHeadersWrapped string
	body                  []byte
	meta                  binaryview.Meta
	contentType           string
	rawText               string
	rawHex                string
	rawBase64             string
	rawMode               rawViewMode
	headersMap            http.Header
	effectiveURL          string
}

var responseRenderSeq uint64

func nextResponseRenderToken() string {
	id := atomic.AddUint64(&responseRenderSeq, 1)
	return fmt.Sprintf("render-%d", id)
}

func renderHTTPResponseCmd(
	token string,
	resp *httpclient.Response,
	tests []scripts.TestResult,
	scriptErr error,
	width int,
) tea.Cmd {
	if resp == nil {
		return nil
	}

	respCopy := cloneHTTPResponse(resp)
	testsCopy := append([]scripts.TestResult(nil), tests...)

	if width <= 0 {
		width = defaultResponseViewportWidth
	}

	targetWidth := width
	headersMap := cloneHeaders(respCopy.Headers)
	effectiveURL := strings.TrimSpace(respCopy.EffectiveURL)

	return func() tea.Msg {
		views := buildHTTPResponseViews(respCopy, testsCopy, scriptErr)
		pretty := views.pretty
		raw := views.raw
		headers := views.headers
		rawSummary := views.rawSummary
		meta := views.meta
		ct := views.contentType
		rawText := views.rawText
		rawHex := views.rawHex
		rawBase64 := views.rawBase64
		rawMode := views.rawMode
		requestHeaders := buildHTTPRequestHeadersView(respCopy)
		return responseRenderedMsg{
			token:          token,
			pretty:         pretty,
			raw:            raw,
			rawSummary:     rawSummary,
			headers:        headers,
			requestHeaders: requestHeaders,
			width:          targetWidth,
			prettyWrapped:  wrapContentForTab(responseTabPretty, pretty, targetWidth),
			rawWrapped:     wrapContentForTab(responseTabRaw, raw, targetWidth),
			headersWrapped: wrapContentForTab(responseTabHeaders, headers, targetWidth),
			requestHeadersWrapped: wrapContentForTab(
				responseTabHeaders,
				requestHeaders,
				targetWidth,
			),
			body:         append([]byte(nil), respCopy.Body...),
			meta:         meta,
			contentType:  ct,
			rawText:      rawText,
			rawHex:       rawHex,
			rawBase64:    rawBase64,
			rawMode:      rawMode,
			headersMap:   headersMap,
			effectiveURL: effectiveURL,
		}
	}
}

func cloneHTTPResponse(resp *httpclient.Response) *httpclient.Response {
	if resp == nil {
		return nil
	}
	var headers http.Header
	var reqHeaders http.Header
	if resp.Headers != nil {
		headers = make(http.Header, len(resp.Headers))
		for key, values := range resp.Headers {
			copied := append([]string(nil), values...)
			headers[key] = copied
		}
	}
	if resp.RequestHeaders != nil {
		reqHeaders = make(http.Header, len(resp.RequestHeaders))
		for key, values := range resp.RequestHeaders {
			copied := append([]string(nil), values...)
			reqHeaders[key] = copied
		}
	}
	reqTE := append([]string(nil), resp.ReqTE...)
	body := append([]byte(nil), resp.Body...)
	var (
		timeline    *nettrace.Timeline
		traceReport *nettrace.Report
	)
	if resp.Timeline != nil {
		timeline = resp.Timeline.Clone()
	}
	if resp.TraceReport != nil {
		traceReport = resp.TraceReport.Clone()
	}

	return &httpclient.Response{
		Status:         resp.Status,
		StatusCode:     resp.StatusCode,
		Proto:          resp.Proto,
		Headers:        headers,
		ReqMethod:      resp.ReqMethod,
		RequestHeaders: reqHeaders,
		ReqHost:        resp.ReqHost,
		ReqLen:         resp.ReqLen,
		ReqTE:          reqTE,
		Body:           body,
		Duration:       resp.Duration,
		EffectiveURL:   resp.EffectiveURL,
		Request:        resp.Request,
		Timeline:       timeline,
		TraceReport:    traceReport,
	}
}

type responseViews struct {
	pretty      string
	raw         string
	rawSummary  string
	headers     string
	meta        binaryview.Meta
	contentType string
	rawText     string
	rawHex      string
	rawBase64   string
	rawMode     rawViewMode
}

func buildHTTPResponseViews(
	resp *httpclient.Response,
	tests []scripts.TestResult,
	scriptErr error,
) responseViews {
	if resp == nil {
		return responseViews{
			pretty:     noResponseMessage,
			raw:        noResponseMessage,
			rawSummary: "",
			headers:    noResponseMessage,
			meta:       binaryview.Meta{},
			rawMode:    rawViewText,
		}
	}

	summary := buildRespSum(resp, tests, scriptErr)
	prettySummary := buildRespSumPretty(resp, tests, scriptErr)
	coloredHeaders := formatHTTPHeaders(resp.Headers, true)

	contentType := ""
	if resp.Headers != nil {
		contentType = resp.Headers.Get("Content-Type")
	}
	meta := binaryview.Analyze(resp.Body, contentType)
	bv := buildBodyViews(resp.Body, contentType, &meta, nil, "")

	headersSectionColored := ""
	if coloredHeaders != "" {
		headersSectionColored = statsHeadingStyle.Render("Headers:") + "\n" + coloredHeaders
	}

	plainSummary := stripANSIEscape(summary)
	prettyView := joinSections(prettySummary, bv.pretty)
	rawView := joinSections(plainSummary, bv.raw)
	headersView := joinSections(summary, headersSectionColored)

	return responseViews{
		pretty:      prettyView,
		raw:         rawView,
		rawSummary:  plainSummary,
		headers:     headersView,
		meta:        meta,
		contentType: contentType,
		rawText:     bv.rawText,
		rawHex:      bv.rawHex,
		rawBase64:   bv.rawBase64,
		rawMode:     bv.mode,
	}
}

func buildHTTPRequestHeadersView(resp *httpclient.Response) string {
	if resp == nil {
		return noResponseMessage
	}

	method := strings.ToUpper(strings.TrimSpace(resp.ReqMethod))
	if method == "" && resp.Request != nil {
		method = strings.ToUpper(strings.TrimSpace(resp.Request.Method))
	}

	url := strings.TrimSpace(resp.EffectiveURL)
	if url == "" && resp.Request != nil {
		url = strings.TrimSpace(resp.Request.URL)
	}

	reqLine := strings.TrimSpace(method + " " + url)
	reqLineColored := ""
	if reqLine != "" {
		reqLineColored = renderLabelValue("Request", reqLine, statsLabelStyle, statsValueStyle)
	}

	hdrs := buildRequestHeaderMap(resp)
	colored := formatHTTPHeaders(hdrs, true)
	section := statsHeadingStyle.Render("Headers:")
	if strings.TrimSpace(colored) != "" {
		section += "\n" + colored
	} else {
		section += "\n<none>"
	}

	return joinSections(reqLineColored, section)
}

func buildRequestHeaderMap(resp *httpclient.Response) http.Header {
	var h http.Header
	if resp != nil && resp.RequestHeaders != nil {
		h = resp.RequestHeaders.Clone()
	}
	if h == nil {
		h = make(http.Header)
	}

	if resp == nil {
		return h
	}

	if h.Get("Host") == "" && strings.TrimSpace(resp.ReqHost) != "" {
		h.Set("Host", resp.ReqHost)
	}
	if h.Get("Transfer-Encoding") == "" && len(resp.ReqTE) > 0 {
		h["Transfer-Encoding"] = append([]string(nil), resp.ReqTE...)
	}
	if h.Get("Content-Length") == "" && resp.ReqLen > 0 {
		h.Set("Content-Length", strconv.FormatInt(resp.ReqLen, 10))
	}

	return h
}

func formatRawBody(body []byte, contentType string) string {
	raw := trimResponseBody(string(body))
	formatted, ok := indentRawBody(body, contentType)
	if !ok {
		return raw
	}
	return trimResponseBody(formatted)
}

type bodyViews struct {
	pretty    string
	raw       string
	rawText   string
	rawHex    string
	rawBase64 string
	mode      rawViewMode
	meta      binaryview.Meta
	ct        string
}

func buildBodyViews(
	body []byte,
	contentType string,
	meta *binaryview.Meta,
	viewBody []byte,
	viewContentType string,
) bodyViews {
	var detected binaryview.Meta
	if meta == nil {
		detected = binaryview.Analyze(body, contentType)
		meta = &detected
	}
	localMeta := *meta

	if len(viewBody) == 0 {
		viewBody = body
	}
	if strings.TrimSpace(viewContentType) == "" {
		viewContentType = contentType
	}

	if !bytes.Equal(viewBody, body) {
		viewMeta := binaryview.Analyze(viewBody, viewContentType)
		if viewMeta.Kind == binaryview.KindText {
			localMeta = viewMeta
		}
		if strings.TrimSpace(localMeta.MIME) == "" {
			localMeta.MIME = viewMeta.MIME
		}
		if strings.TrimSpace(localMeta.Charset) == "" {
			localMeta.Charset = viewMeta.Charset
		}
	}

	sz := len(body)
	rawHex := ""
	rawBase64 := ""
	if sz <= rawHeavyLimit {
		rawHex = binaryview.HexDump(body, binaryview.HexDumpBytesPerLine)
		rawBase64 = binaryview.Base64Lines(body, rawBase64LineWidth)
	}

	rawMode := rawViewText
	rawText := ""
	if localMeta.Kind != binaryview.KindBinary || localMeta.Printable {
		rawText = formatRawBody(viewBody, viewContentType)
	}

	decoded := viewBody
	if localMeta.Kind == binaryview.KindText {
		if decodedText, ok, errStr := binaryview.DecodeText(viewBody, localMeta.Charset); ok {
			decoded = []byte(decodedText)
			rawText = formatRawBody(decoded, viewContentType)
		} else if errStr != "" {
			localMeta.DecodeErr = errStr
		}
	}

	var prettyBody string
	if localMeta.Kind == binaryview.KindBinary {
		prettyBody = renderBinarySummary(localMeta)
		if rawHeavyBin(localMeta, sz) {
			rawMode = rawViewSummary
		} else {
			rawMode = rawViewHex
		}
	} else {
		prettyBody = trimResponseBody(prettifyBody(decoded, viewContentType))
	}
	rawMode = clampRawViewMode(localMeta, sz, rawMode)
	if rawMode == rawViewHex && rawHex == "" {
		if rawHeavyBin(localMeta, sz) {
			rawMode = rawViewSummary
		} else {
			rawMode = rawViewText
		}
	}

	if isBodyEmpty(prettyBody) {
		prettyBody = "<empty>"
	}

	rawDefault := rawText
	if rawMode == rawViewSummary {
		rawDefault = rawSum(localMeta, sz)
	} else if rawMode == rawViewHex && rawHex != "" {
		rawDefault = rawHex
	}
	if isBodyEmpty(rawDefault) {
		rawDefault = "<empty>"
	}

	*meta = localMeta
	return bodyViews{
		pretty:    prettyBody,
		raw:       rawDefault,
		rawText:   rawText,
		rawHex:    rawHex,
		rawBase64: rawBase64,
		mode:      rawMode,
		meta:      localMeta,
		ct:        viewContentType,
	}
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

func renderBinarySummary(meta binaryview.Meta) string {
	lines := []string{
		statsHeadingStyle.Render(fmt.Sprintf("Binary body (%s)", formatByteSize(int64(meta.Size)))),
	}
	if strings.TrimSpace(meta.MIME) != "" {
		lines = append(
			lines,
			renderLabelValue(
				"MIME",
				strings.TrimSpace(meta.MIME),
				statsLabelStyle,
				statsValueStyle,
			),
		)
	}
	if strings.TrimSpace(meta.DecodeErr) != "" {
		lines = append(
			lines,
			statsWarnStyle.Render("Decode warning: "+strings.TrimSpace(meta.DecodeErr)),
		)
	}
	if meta.PreviewHex != "" {
		lines = append(
			lines,
			renderLabelValue("Preview hex", meta.PreviewHex, statsLabelStyle, statsMessageStyle),
		)
	}
	if meta.PreviewB64 != "" {
		lines = append(
			lines,
			renderLabelValue("Preview base64", meta.PreviewB64, statsLabelStyle, statsMessageStyle),
		)
	}
	if modes := rawViewModeLabels(meta, meta.Size); len(modes) > 0 {
		lines = append(
			lines,
			renderLabelValue(
				"Raw tab",
				strings.Join(modes, " / "),
				statsLabelStyle,
				statsValueStyle,
			),
		)
	}
	return strings.Join(lines, "\n")
}

func cloneHeaders(h http.Header) http.Header {
	if h == nil {
		return nil
	}
	clone := make(http.Header, len(h))
	for k, values := range h {
		clone[k] = append([]string(nil), values...)
	}
	return clone
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
				builder.WriteString(
					renderLabelValue(name, joined, statsLabelStyle, statsHeaderValueStyle),
				)
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

func renderCompareBundle(bundle *compareBundle, focusedEnv string) string {
	if bundle == nil {
		return "Compare data unavailable"
	}
	var buf bytes.Buffer
	baseline := strings.TrimSpace(bundle.Baseline)
	title := "Baseline: (first environment)"
	if baseline != "" {
		title = "Baseline: " + baseline
	}
	buf.WriteString(statsTitleStyle.Render(title))
	buf.WriteString("\n\n")
	buf.WriteString(formatCompareHeader())
	buf.WriteString("\n")
	buf.WriteString(formatCompareSeparator())
	buf.WriteString("\n")
	for _, row := range bundle.Rows {
		buf.WriteString(formatCompareRow(
			formatCompareEnvLabel(row, baseline, focusedEnv),
			formatCompareStatus(row),
			formatCompareCode(row),
			statsDurationStyle.Render(formatDurationShort(row.Duration)),
			formatCompareDiff(row),
		))
		buf.WriteString("\n")
	}
	return buf.String()
}

func truncateCompareField(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}

func formatDurationShort(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	if d < time.Microsecond {
		return d.String()
	}
	if d < time.Millisecond {
		value := d / time.Microsecond
		return fmt.Sprintf("%dµs", value)
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return d.Round(time.Millisecond).String()
}

func formatCompareHeader() string {
	return formatCompareRow(
		statsHeadingStyle.Render("Env"),
		statsHeadingStyle.Render("Status"),
		statsHeadingStyle.Render("Code"),
		statsHeadingStyle.Render("Duration"),
		statsHeadingStyle.Render("Diff"),
	)
}

func formatCompareSeparator() string {
	segments := []string{
		strings.Repeat("─", compareColEnvWidth),
		strings.Repeat("─", compareColStatusWidth),
		strings.Repeat("─", compareColCodeWidth),
		strings.Repeat("─", compareColDurationWidth),
		strings.Repeat("─", 12),
	}
	return strings.Join(segments, compareColumnGap)
}

func formatCompareRow(env, status, code, duration, diff string) string {
	columns := []string{
		padStyled(env, compareColEnvWidth),
		padStyled(status, compareColStatusWidth),
		padStyled(code, compareColCodeWidth),
		padStyled(duration, compareColDurationWidth),
		diff,
	}
	return strings.Join(columns, compareColumnGap)
}

func padStyled(content string, width int) string {
	w := lipgloss.Width(content)
	if w >= width {
		return content
	}
	return content + strings.Repeat(" ", width-w)
}

func formatCompareEnvLabel(row compareRow, baseline, focused string) string {
	env := ""
	if row.Result != nil {
		env = strings.TrimSpace(row.Result.Environment)
	}
	if env == "" {
		env = "(env)"
	}
	label := env
	if baseline != "" && strings.EqualFold(env, baseline) {
		label = label + " *"
	}
	style := statsLabelStyle
	if baseline != "" && strings.EqualFold(env, baseline) {
		style = statsHeadingStyle
	}
	if focused != "" && strings.EqualFold(env, focused) {
		label = "> " + label
		style = statsSelectedStyle
	}
	return style.Render(label)
}

func formatCompareStatus(row compareRow) string {
	status := strings.TrimSpace(row.Status)
	if status == "" {
		status = "pending"
	}
	indicator := compareRowIndicator(row.Result)
	style := statsMessageStyle
	indicatorRendered := ""
	switch indicator {
	case "✓":
		style = statsSuccessStyle
		indicatorRendered = statsSuccessStyle.Render(indicator)
	case "✗":
		style = statsWarnStyle
		indicatorRendered = statsWarnStyle.Render(indicator)
	case "…":
		indicatorRendered = statsNeutralStyle.Render(indicator)
	}
	if indicatorRendered != "" {
		return fmt.Sprintf("%s %s", indicatorRendered, style.Render(status))
	}
	return style.Render(status)
}

func formatCompareCode(row compareRow) string {
	code := strings.TrimSpace(row.Code)
	if code == "" && row.Result != nil {
		switch {
		case row.Result.Response != nil && row.Result.Response.StatusCode > 0:
			code = fmt.Sprintf("%d", row.Result.Response.StatusCode)
		case row.Result.GRPC != nil && row.Result.GRPC.StatusCode > 0:
			code = fmt.Sprintf("%d", row.Result.GRPC.StatusCode)
		}
	}
	if code == "" {
		code = "-"
	}
	style := statsValueStyle
	if code == "-" {
		style = statsLabelStyle
	} else if row.Result != nil && !compareResultSuccess(row.Result) {
		style = statsWarnStyle
	}
	return style.Render(code)
}

func formatCompareDiff(row compareRow) string {
	diff := truncateCompareField(row.Summary, 48)
	if diff == "" {
		diff = "n/a"
	}
	style := statsMessageStyle
	if compareResultSuccess(row.Result) && strings.EqualFold(diff, "match") {
		style = statsSuccessStyle
	} else if row.Result != nil && !compareResultSuccess(row.Result) && diff != "n/a" {
		style = statsWarnStyle
	}
	return style.Render(diff)
}

func compareRowIndicator(result *compareResult) string {
	if result == nil {
		return "…"
	}
	if compareResultSuccess(result) {
		return "✓"
	}
	return "✗"
}

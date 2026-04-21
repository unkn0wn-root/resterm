package ui

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
	"github.com/unkn0wn-root/resterm/internal/bodyfmt"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
)

const (
	responseFormattingBase         = "Formatting response"
	responseFormattingCanceledText = "Formatting canceled"
	responseReflowingMessage       = "Reflowing response"
	responseReflowCanceledText     = "Reflow canceled.\nRun request again to render."
	defaultResponseViewportWidth   = 80
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
	valid   bool
	spans   []lineSpan
	rev     []int
}

type lineSpan struct {
	start int
	end   int
}

type responseRenderedMsg struct {
	token          string
	pretty         string
	raw            string
	rawSummary     string
	headers        string
	requestHeaders string
	width          int
	body           []byte
	meta           binaryview.Meta
	contentType    string
	rawText        string
	rawHex         string
	rawBase64      string
	rawMode        rawViewMode
	headersMap     http.Header
	effectiveURL   string
}

var responseRenderSeq uint64

func nextResponseRenderToken() string {
	id := atomic.AddUint64(&responseRenderSeq, 1)
	return fmt.Sprintf("render-%d", id)
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

func cloneGRPCResponse(resp *grpcclient.Response) *grpcclient.Response {
	if resp == nil {
		return nil
	}
	headers := make(map[string][]string, len(resp.Headers))
	for key, values := range resp.Headers {
		headers[key] = append([]string(nil), values...)
	}
	trailers := make(map[string][]string, len(resp.Trailers))
	for key, values := range resp.Trailers {
		trailers[key] = append([]string(nil), values...)
	}
	return &grpcclient.Response{
		Message:         resp.Message,
		Body:            append([]byte(nil), resp.Body...),
		Wire:            append([]byte(nil), resp.Wire...),
		ContentType:     resp.ContentType,
		WireContentType: resp.WireContentType,
		Headers:         headers,
		Trailers:        trailers,
		StatusCode:      resp.StatusCode,
		StatusMessage:   resp.StatusMessage,
		Duration:        resp.Duration,
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

type responseRenderer struct {
	stats       statsPalette
	syntaxStyle string
}

func newResponseRenderer(stats statsPalette, syntaxStyle string) responseRenderer {
	syntaxStyle = strings.TrimSpace(syntaxStyle)
	if syntaxStyle == "" {
		syntaxStyle = "monokai"
	}
	return responseRenderer{
		stats:       stats,
		syntaxStyle: syntaxStyle,
	}
}

func defaultResponseRenderer() responseRenderer {
	return newResponseRenderer(defaultStatsPalette(), "monokai")
}

func buildHTTPResponseViews(
	resp *httpclient.Response,
	tests []scripts.TestResult,
	scriptErr error,
) responseViews {
	return defaultResponseRenderer().buildHTTPResponseViewsCtx(
		context.Background(),
		resp,
		tests,
		scriptErr,
	)
}

func buildHTTPResponseViewsCtx(
	ctx context.Context,
	resp *httpclient.Response,
	tests []scripts.TestResult,
	scriptErr error,
) responseViews {
	return defaultResponseRenderer().buildHTTPResponseViewsCtx(ctx, resp, tests, scriptErr)
}

func (r responseRenderer) buildHTTPResponseViewsCtx(
	ctx context.Context,
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

	summary := r.buildRespSum(resp, tests, scriptErr)
	prettySummary := r.buildRespSumPretty(resp, tests, scriptErr)
	coloredHeaders := r.formatHTTPHeaders(resp.Headers, true)

	contentType := ""
	if resp.Headers != nil {
		contentType = resp.Headers.Get("Content-Type")
	}
	meta := binaryview.Analyze(resp.Body, contentType)
	bv := r.buildBodyViewsCtx(
		ctx,
		resp.Body,
		contentType,
		&meta,
		nil,
		"",
	)

	headersSectionColored := ""
	if coloredHeaders != "" {
		headersSectionColored = r.stats.Heading.Render("Headers:") + "\n" + coloredHeaders
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
	return defaultResponseRenderer().buildHTTPRequestHeadersView(resp)
}

func (r responseRenderer) buildHTTPRequestHeadersView(resp *httpclient.Response) string {
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
		reqLineColored = renderLabelValue("Request", reqLine, r.stats.Label, r.stats.Value)
	}

	hdrs := buildRequestHeaderMap(resp)
	colored := r.formatHTTPHeaders(hdrs, true)
	section := r.stats.Heading.Render("Headers:")
	if strings.TrimSpace(colored) != "" {
		section += "\n" + colored
	} else {
		section += "\n<none>"
	}

	return joinSections(reqLineColored, section)
}

func buildGRPCResponseViews(resp *grpcclient.Response, fullMethod string) responseViews {
	return defaultResponseRenderer().buildGRPCResponseViews(resp, fullMethod)
}

func (r responseRenderer) buildGRPCResponseViews(
	resp *grpcclient.Response,
	fullMethod string,
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

	headersBuilder := strings.Builder{}
	contentType := strings.TrimSpace(resp.ContentType)
	if len(resp.Headers) > 0 {
		headersBuilder.WriteString("Headers:\n")
		for name, values := range resp.Headers {
			fmt.Fprintf(&headersBuilder, "%s: %s\n", name, strings.Join(values, ", "))
			if strings.EqualFold(name, "Content-Type") && contentType == "" && len(values) > 0 {
				contentType = strings.TrimSpace(values[0])
			}
		}
	}
	if len(resp.Trailers) > 0 {
		if headersBuilder.Len() > 0 {
			headersBuilder.WriteString("\n")
		}
		headersBuilder.WriteString("Trailers:\n")
		for name, values := range resp.Trailers {
			fmt.Fprintf(&headersBuilder, "%s: %s\n", name, strings.Join(values, ", "))
		}
	}
	headersContent := strings.TrimRight(headersBuilder.String(), "\n")

	statusLine := fmt.Sprintf(
		"gRPC %s - %s",
		strings.TrimPrefix(strings.TrimSpace(fullMethod), "/"),
		resp.StatusCode.String(),
	)
	if resp.StatusMessage != "" {
		statusLine += " (" + resp.StatusMessage + ")"
	}

	viewBody := append([]byte(nil), resp.Body...)
	if len(viewBody) == 0 && strings.TrimSpace(resp.Message) != "" {
		viewBody = []byte(resp.Message)
	}
	viewContentType := strings.TrimSpace(resp.ContentType)
	if viewContentType == "" && len(viewBody) > 0 {
		viewContentType = "application/json"
	}

	rawBody := append([]byte(nil), resp.Wire...)
	if len(rawBody) == 0 {
		rawBody = append([]byte(nil), viewBody...)
	}
	rawContentType := strings.TrimSpace(resp.WireContentType)
	if rawContentType == "" {
		rawContentType = contentType
	}
	if rawContentType == "" {
		rawContentType = viewContentType
	}

	meta := binaryview.Analyze(viewBody, viewContentType)
	bv := r.buildBodyViewsCtx(
		context.Background(),
		rawBody,
		rawContentType,
		&meta,
		viewBody,
		viewContentType,
	)

	return responseViews{
		pretty:      joinSections(statusLine, bv.pretty),
		raw:         joinSections(statusLine, bv.raw),
		rawSummary:  statusLine,
		headers:     joinSections(statusLine, headersContent),
		meta:        meta,
		contentType: bv.ct,
		rawText:     bv.rawText,
		rawHex:      bv.rawHex,
		rawBase64:   bv.rawBase64,
		rawMode:     bv.mode,
	}
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
	return bodyfmt.FormatRaw(body, contentType)
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
	return defaultResponseRenderer().buildBodyViewsCtx(
		context.Background(),
		body,
		contentType,
		meta,
		viewBody,
		viewContentType,
	)
}

func buildBodyViewsCtx(
	ctx context.Context,
	body []byte,
	contentType string,
	meta *binaryview.Meta,
	viewBody []byte,
	viewContentType string,
) bodyViews {
	return defaultResponseRenderer().buildBodyViewsCtx(
		ctx,
		body,
		contentType,
		meta,
		viewBody,
		viewContentType,
	)
}

func (r responseRenderer) buildBodyViewsCtx(
	ctx context.Context,
	body []byte,
	contentType string,
	meta *binaryview.Meta,
	viewBody []byte,
	viewContentType string,
) bodyViews {
	out := bodyfmt.BuildContext(ctx, bodyfmt.BuildInput{
		Body:            body,
		ContentType:     contentType,
		Meta:            meta,
		ViewBody:        viewBody,
		ViewContentType: viewContentType,
		Color:           termcolor.TrueColor(),
		Style:           r.syntaxStyle,
	})
	if meta != nil {
		*meta = out.Meta
	}
	pretty := out.Pretty
	if out.Meta.Kind == binaryview.KindBinary {
		pretty = r.renderBinarySummary(out.Meta)
	}
	return bodyViews{
		pretty:    pretty,
		raw:       out.Raw,
		rawText:   out.RawText,
		rawHex:    out.RawHex,
		rawBase64: out.RawBase64,
		mode:      rawViewMode(out.Mode),
		meta:      out.Meta,
		ct:        out.ContentType,
	}
}

func renderBinarySummary(meta binaryview.Meta) string {
	return defaultResponseRenderer().renderBinarySummary(meta)
}

func (r responseRenderer) renderBinarySummary(meta binaryview.Meta) string {
	lines := []string{
		r.stats.Heading.Render(fmt.Sprintf("Binary body (%s)", formatByteSize(int64(meta.Size)))),
	}
	if strings.TrimSpace(meta.MIME) != "" {
		lines = append(
			lines,
			renderLabelValue(
				"MIME",
				strings.TrimSpace(meta.MIME),
				r.stats.Label,
				r.stats.Value,
			),
		)
	}
	if strings.TrimSpace(meta.DecodeErr) != "" {
		lines = append(
			lines,
			r.stats.Warn.Render("Decode warning: "+strings.TrimSpace(meta.DecodeErr)),
		)
	}
	if meta.PreviewHex != "" {
		lines = append(
			lines,
			renderLabelValue("Preview hex", meta.PreviewHex, r.stats.Label, r.stats.Message),
		)
	}
	if meta.PreviewB64 != "" {
		lines = append(
			lines,
			renderLabelValue("Preview base64", meta.PreviewB64, r.stats.Label, r.stats.Message),
		)
	}
	if modes := rawViewModeLabels(meta, meta.Size); len(modes) > 0 {
		lines = append(
			lines,
			renderLabelValue(
				"Raw tab",
				strings.Join(modes, " / "),
				r.stats.Label,
				r.stats.Value,
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
	return defaultResponseRenderer().formatHTTPHeaders(headers, colored)
}

func (r responseRenderer) formatHTTPHeaders(headers http.Header, colored bool) string {
	fields := bodyfmt.HeaderFields(headers)
	if len(fields) == 0 {
		return ""
	}
	builder := strings.Builder{}
	for _, field := range fields {
		if colored {
			if strings.TrimSpace(field.Value) == "" {
				builder.WriteString(r.stats.Label.Render(field.Name + ":"))
			} else {
				builder.WriteString(
					renderLabelValue(
						field.Name,
						field.Value,
						r.stats.Label,
						r.stats.HeaderValue,
					),
				)
			}
		} else {
			if strings.TrimSpace(field.Value) == "" {
				fmt.Fprintf(&builder, "%s:", field.Name)
			} else {
				fmt.Fprintf(&builder, "%s: %s", field.Name, field.Value)
			}
		}
		builder.WriteString("\n")
	}
	return strings.TrimRight(builder.String(), "\n")
}

func trimResponseBody(body string) string {
	return bodyfmt.TrimBody(body)
}

func isBodyEmpty(body string) bool {
	return bodyfmt.IsEmpty(body)
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

package runview

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/bodyfmt"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/runner"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
)

var ErrNilWriter = errors.New("runview: nil writer")

type Mode string

const (
	ModePretty Mode = "pretty"
	ModeRaw    Mode = "raw"
)

type Options struct {
	Mode    Mode
	Headers bool
	Color   termcolor.Config
}

type BodyOptions struct {
	Mode  Mode
	Color termcolor.Config
}

func Write(w io.Writer, rep *runner.Report, opt Options) error {
	if w == nil {
		return ErrNilWriter
	}
	out, err := Render(rep, opt)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, out)
	return err
}

func WriteBody(w io.Writer, rep *runner.Report, opt BodyOptions) error {
	if w == nil {
		return ErrNilWriter
	}
	out, err := RenderBody(rep, opt)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, out)
	return err
}

func Render(rep *runner.Report, opt Options) (string, error) {
	mode, err := normalizeMode(opt.Mode)
	if err != nil {
		return "", err
	}
	res, err := singleResult(rep)
	if err != nil {
		return "", err
	}
	if res.Kind != runner.ResultKindRequest {
		return reportText(rep)
	}
	return renderRequest(*res, Options{
		Mode:    mode,
		Headers: opt.Headers,
		Color:   opt.Color,
	}), nil
}

func RenderBody(rep *runner.Report, opt BodyOptions) (string, error) {
	mode, err := normalizeMode(opt.Mode)
	if err != nil {
		return "", err
	}
	res, err := singleResult(rep)
	if err != nil {
		return "", err
	}
	if res.Kind != runner.ResultKindRequest {
		return "", errors.New("runview: body output requires exactly one request result")
	}
	return requestBodyText(*res, mode, opt.Color), nil
}

func CanRender(rep *runner.Report) bool {
	_, err := singleResult(rep)
	return err == nil
}

func CanRenderRequest(rep *runner.Report) bool {
	res, err := singleResult(rep)
	return err == nil && res.Kind == runner.ResultKindRequest
}

func normalizeMode(mode Mode) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(string(mode))) {
	case "", string(ModePretty):
		return ModePretty, nil
	case string(ModeRaw):
		return ModeRaw, nil
	default:
		return "", fmt.Errorf("runview: unsupported mode %q", mode)
	}
}

func singleResult(rep *runner.Report) (*runner.Result, error) {
	if rep == nil {
		return nil, errors.New("runview: empty report")
	}
	if len(rep.Results) != 1 {
		return nil, fmt.Errorf("runview: expected 1 result, got %d", len(rep.Results))
	}
	return &rep.Results[0], nil
}

func reportText(rep *runner.Report) (string, error) {
	var buf bytes.Buffer
	if err := rep.WriteText(&buf); err != nil {
		return "", err
	}
	return bodyfmt.TrimSection(buf.String()), nil
}

func renderRequest(res runner.Result, opt Options) string {
	st := newStyler(opt.Color)
	summary := requestSummary(res, st)
	issues := requestIssues(res, st)
	warnings := requestWarnings(res, st)
	headers := requestHeadersText(res, opt.Headers, st)
	body := requestBody(res, opt.Mode, opt.Color, st)
	return bodyfmt.JoinSections(summary, issues, warnings, headers, body)
}

func requestSummary(res runner.Result, st styler) string {
	fields := []struct {
		label string
		value string
		tone  tone
	}{
		{"Name", strings.TrimSpace(res.Name), toneValue},
		{"Request", requestLine(res), toneValue},
		{"Environment", strings.TrimSpace(res.Environment), toneValue},
		{"Status", statusText(res), statusTone(res)},
		{"Duration", durationText(res), toneDur},
		{"Content-Length", contentLengthText(res), toneValue},
	}
	var lines []string
	for _, f := range fields {
		if f.value != "" {
			lines = append(lines, st.pair(f.label, f.value, f.tone))
		}
	}
	return strings.Join(lines, "\n")
}

func requestIssues(res runner.Result, st styler) string {
	var parts []string
	if res.Err != nil {
		parts = append(parts, st.pair("Request error", strings.TrimSpace(res.Err.Error()), toneWarn))
	}
	if res.ScriptErr != nil {
		parts = append(parts, st.pair("Script error", strings.TrimSpace(res.ScriptErr.Error()), toneWarn))
	}
	if msg := traceFailureText(res.Trace); msg != "" {
		parts = append(parts, st.pair("Trace", msg, toneWarn))
	}
	errs := ""
	if len(parts) > 0 {
		errs = st.sectionWarn("Errors:") + "\n" + indent(strings.Join(parts, "\n"), "  ")
	}

	tests := testsText(res, st)
	return bodyfmt.JoinSections(errs, tests)
}

func requestWarnings(res runner.Result, st styler) string {
	items, _ := res.UnresolvedTemplateVars()
	if len(items) == 0 {
		return ""
	}
	line := st.pair(
		"Unresolved template variables",
		strings.Join(items, ", "),
		toneCaution,
	)
	return st.sectionCaution("Warnings:") + "\n" + indent(line, "  ")
}

func testsText(res runner.Result, st styler) string {
	var lines []string
	for _, test := range res.Tests {
		line := st.badge(testBadge(test))
		if name := strings.TrimSpace(test.Name); name != "" {
			line += " " + st.value(name, toneValue)
		}
		if msg := strings.TrimSpace(test.Message); msg != "" {
			line += " - " + st.value(msg, toneMsg)
		}
		if test.Elapsed > 0 {
			line += " " + st.value(fmt.Sprintf("(%s)", durationRound(test.Elapsed)), toneDur)
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return ""
	}
	return st.section("Tests:") + "\n" + indent(strings.Join(lines, "\n"), "  ")
}

func testBadge(test scripts.TestResult) (string, bool) {
	switch test.Passed {
	case true:
		return "[PASS]", true
	case false:
		return "[FAIL]", false
	}
	return "[FAIL]", false
}

func requestHeadersText(res runner.Result, show bool, st styler) string {
	if !show {
		return ""
	}
	reqText := ""
	respText := ""
	if resp := res.Response; resp != nil {
		if reqHeaders := buildRequestHeaderMap(resp); len(reqHeaders) > 0 {
			reqText = st.section("Request Headers:") + "\n" + formatHeaders(reqHeaders, st)
		}
		if len(resp.Headers) > 0 {
			respText = st.section("Response Headers:") + "\n" + formatHeaders(resp.Headers, st)
		}
	}
	if grpc := res.GRPC; grpc != nil {
		if hdrs := buildGRPCHeaderMap(grpc); len(hdrs) > 0 {
			respText = st.section("Response Headers:") + "\n" + formatHeaders(hdrs, st)
		}
	}
	return bodyfmt.JoinSections(reqText, respText)
}

func requestBody(res runner.Result, mode Mode, color termcolor.Config, st styler) string {
	heading := "Body:"
	if mode == ModeRaw {
		heading = "Raw Body:"
	}
	body := requestBodyText(res, mode, color)
	if body == "" {
		body = "<empty>"
	}
	return st.section(heading) + "\n" + body
}

func requestBodyText(res runner.Result, mode Mode, color termcolor.Config) string {
	input := resolveBodyInput(res)
	if bodyInputEmpty(input) {
		return ""
	}
	if mode == ModePretty {
		input.Color = color
	}
	return selectBody(bodyfmt.Build(input), mode)
}

func resolveBodyInput(res runner.Result) bodyfmt.BuildInput {
	if resp := res.Response; resp != nil {
		return httpBodyInput(res, resp)
	}
	if grpc := res.GRPC; grpc != nil {
		return grpcBodyInput(res, grpc)
	}
	body, ct := streamFallback(res)
	return bodyfmt.BuildInput{Body: body, ContentType: ct}
}

func httpBodyInput(res runner.Result, resp *httpclient.Response) bodyfmt.BuildInput {
	ct := ""
	if resp.Headers != nil {
		ct = resp.Headers.Get("Content-Type")
	}
	body := cloneBytes(resp.Body)
	if len(body) == 0 {
		if raw, rawType := streamFallback(res); len(raw) > 0 {
			body = raw
			ct = rawType
		}
	}
	return bodyfmt.BuildInput{Body: body, ContentType: ct}
}

func grpcBodyInput(res runner.Result, grpc *grpcclient.Response) bodyfmt.BuildInput {
	viewBody := cloneBytes(grpc.Body)
	if len(viewBody) == 0 && strings.TrimSpace(grpc.Message) != "" {
		viewBody = []byte(grpc.Message)
	}
	viewType := strings.TrimSpace(grpc.ContentType)
	if viewType == "" && len(viewBody) > 0 {
		viewType = "application/json"
	}
	rawBody := cloneBytes(grpc.Wire)
	rawType := strings.TrimSpace(grpc.WireContentType)
	if len(rawBody) == 0 {
		rawBody = cloneBytes(viewBody)
	}
	if rawType == "" {
		rawType = viewType
	}
	if len(viewBody) == 0 {
		if fallback, fallbackType := streamFallback(res); len(fallback) > 0 {
			viewBody = fallback
			viewType = fallbackType
			rawBody = cloneBytes(fallback)
			rawType = fallbackType
		}
	}
	return bodyfmt.BuildInput{
		Body:            rawBody,
		ContentType:     rawType,
		ViewBody:        viewBody,
		ViewContentType: viewType,
	}
}

func selectBody(views bodyfmt.BodyViews, mode Mode) string {
	if mode == ModeRaw {
		return views.Raw
	}
	return views.Pretty
}

func streamFallback(res runner.Result) ([]byte, string) {
	if raw := res.Transcript(); len(raw) > 0 {
		return raw, "application/json"
	}
	if text := streamSummaryText(res.Stream); text != "" {
		return []byte(text), "text/plain"
	}
	return nil, ""
}

func streamSummaryText(info *runner.StreamInfo) string {
	if info == nil {
		return ""
	}
	var lines []string
	if kind := strings.TrimSpace(info.Kind); kind != "" {
		lines = append(lines, "Stream: "+kind)
	}
	if len(info.Summary) > 0 {
		keys := make([]string, 0, len(info.Summary))
		for key := range info.Summary {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			lines = append(lines, fmt.Sprintf("%s: %v", key, info.Summary[key]))
		}
	}
	return strings.Join(lines, "\n")
}

func requestLine(res runner.Result) string {
	method := strings.ToUpper(strings.TrimSpace(res.Method))
	if method == "" {
		method = "REQ"
	}
	target := strings.TrimSpace(res.Target)
	if target == "" {
		target = strings.TrimSpace(res.Name)
	}
	if target == "" {
		return method
	}
	return method + " " + target
}

func statusText(res runner.Result) string {
	switch {
	case res.Response != nil:
		return strings.TrimSpace(res.Response.Status)
	case res.GRPC != nil:
		code := strings.TrimSpace(res.GRPC.StatusCode.String())
		msg := strings.TrimSpace(res.GRPC.StatusMessage)
		if code != "" && msg != "" && !strings.EqualFold(code, msg) {
			return code + " (" + msg + ")"
		}
		return code
	default:
		return ""
	}
}

func durationText(res runner.Result) string {
	dur := requestDuration(res)
	if dur <= 0 {
		return ""
	}
	return durationRound(dur)
}

func requestDuration(res runner.Result) time.Duration {
	if res.Duration > 0 {
		return res.Duration
	}
	switch {
	case res.Response != nil:
		return res.Response.Duration
	case res.GRPC != nil:
		return res.GRPC.Duration
	default:
		return 0
	}
}

func durationRound(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	if rounded := d.Round(time.Millisecond); rounded > 0 {
		return rounded.String()
	}
	return d.String()
}

func contentLengthText(res runner.Result) string {
	if resp := res.Response; resp != nil {
		if resp.Headers != nil {
			if raw := strings.TrimSpace(resp.Headers.Get("Content-Length")); raw != "" {
				if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n >= 0 {
					return bodyfmt.FormatByteQuantity(n)
				}
				return raw
			}
		}
		return bodyfmt.FormatByteQuantity(int64(len(resp.Body)))
	}
	if grpc := res.GRPC; grpc != nil {
		if len(grpc.Body) > 0 {
			return bodyfmt.FormatByteQuantity(int64(len(grpc.Body)))
		}
		if raw := res.Transcript(); len(raw) > 0 {
			return bodyfmt.FormatByteQuantity(int64(len(raw)))
		}
	}
	return ""
}

func buildRequestHeaderMap(resp *httpclient.Response) http.Header {
	var hdrs http.Header
	if resp != nil && resp.RequestHeaders != nil {
		hdrs = resp.RequestHeaders.Clone()
	}
	if hdrs == nil {
		hdrs = make(http.Header)
	}
	if resp == nil {
		return hdrs
	}
	if hdrs.Get("Host") == "" && strings.TrimSpace(resp.ReqHost) != "" {
		hdrs.Set("Host", resp.ReqHost)
	}
	if hdrs.Get("Transfer-Encoding") == "" && len(resp.ReqTE) > 0 {
		hdrs["Transfer-Encoding"] = append([]string(nil), resp.ReqTE...)
	}
	if hdrs.Get("Content-Length") == "" && resp.ReqLen > 0 {
		hdrs.Set("Content-Length", fmt.Sprintf("%d", resp.ReqLen))
	}
	return hdrs
}

func buildGRPCHeaderMap(resp *grpcclient.Response) http.Header {
	if resp == nil || (len(resp.Headers) == 0 && len(resp.Trailers) == 0) {
		return nil
	}
	hdrs := make(http.Header, len(resp.Headers)+len(resp.Trailers))
	for key, values := range resp.Headers {
		hdrs[key] = append([]string(nil), values...)
	}
	for key, values := range resp.Trailers {
		hdrs["Grpc-Trailer-"+key] = append([]string(nil), values...)
	}
	return hdrs
}

func traceFailureText(info *runner.TraceInfo) string {
	if info == nil || info.Summary == nil || len(info.Summary.Breaches) == 0 {
		return ""
	}
	breach := info.Summary.Breaches[0]
	label := strings.TrimSpace(breach.Kind)
	if label == "" {
		label = "trace"
	}
	switch {
	case breach.Over > 0:
		return fmt.Sprintf("trace budget breach %s (+%s)", label, breach.Over)
	case breach.Limit > 0 && breach.Actual > 0:
		return fmt.Sprintf("trace budget breach %s (%s > %s)", label, breach.Actual, breach.Limit)
	default:
		return "trace budget breach " + label
	}
}

func cloneBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	return append([]byte(nil), b...)
}

func bodyInputEmpty(in bodyfmt.BuildInput) bool {
	return len(in.Body) == 0 && len(in.ViewBody) == 0
}

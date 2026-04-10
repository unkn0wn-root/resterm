package ui

import (
	"fmt"
	"net/http"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

func (m *Model) handleRunReqMsg(msg runReqMsg) tea.Cmd {
	if msg.err != nil {
		return m.handleRunErr(msg.err)
	}
	if msg.res.Workflow != nil || msg.res.Compare != nil || msg.res.Profile != nil {
		return m.handleRunErr(
			errdef.New(errdef.CodeUI, "unexpected aggregate run result on request path"),
		)
	}
	return m.handleResponseMessage(m.responseMsgFromRun(msg.res))
}

func (m *Model) handleRunErr(err error) tea.Cmd {
	if err == nil {
		return nil
	}
	m.lastError = err
	m.setStatusMessage(statusMsg{text: err.Error(), level: statusError})
	return nil
}

func (m *Model) responseMsgFromRun(res engine.RequestResult) responseMsg {
	return m.responseMsgFromRunState(res, true)
}

func (m *Model) responseMsgFromRunState(res engine.RequestResult, done bool) responseMsg {
	return responseMsg{
		response:       res.Response,
		grpc:           res.GRPC,
		stream:         res.Stream,
		transcript:     append([]byte(nil), res.Transcript...),
		err:            res.Err,
		tests:          append([]scripts.TestResult(nil), res.Tests...),
		scriptErr:      res.ScriptErr,
		executed:       res.Executed,
		requestText:    res.RequestText,
		runtimeSecrets: append([]string(nil), res.RuntimeSecrets...),
		environment:    res.Environment,
		skipped:        res.Skipped,
		skipReason:     res.SkipReason,
		preview:        res.Preview,
		explain:        res.Explain,
		historyDone:    done,
	}
}

func (m *Model) applyRunSnapshot(
	sn *responseSnapshot,
	hr *httpclient.Response,
	gr *grpcclient.Response,
) {
	if sn == nil {
		return
	}
	if m.responseLatest != nil && m.responseLatest.ready {
		m.responsePrevious = m.responseLatest
	}
	m.setResponseSnapshotContent(sn)
	m.lastResponse = hr
	m.lastGRPC = gr
}

func (m *Model) syncRecordedHistory() {
	m.syncHistory()
	m.selectNewestHistoryEntry()
}

func (m *Model) newHTTPSnapshot(
	resp *httpclient.Response,
	tests []scripts.TestResult,
	sErr error,
	env string,
) *responseSnapshot {
	if resp == nil {
		return nil
	}
	views := buildHTTPResponseViews(resp, tests, sErr)
	sn := &responseSnapshot{
		id:              nextResponseRenderToken(),
		pretty:          views.pretty,
		raw:             views.raw,
		rawSummary:      views.rawSummary,
		headers:         views.headers,
		requestHeaders:  buildHTTPRequestHeadersView(resp),
		ready:           true,
		environment:     strings.TrimSpace(env),
		body:            append([]byte(nil), resp.Body...),
		bodyMeta:        views.meta,
		contentType:     views.contentType,
		rawText:         views.rawText,
		rawHex:          views.rawHex,
		rawBase64:       views.rawBase64,
		rawMode:         views.rawMode,
		responseHeaders: cloneHeaders(resp.Headers),
		effectiveURL:    strings.TrimSpace(resp.EffectiveURL),
	}
	if ts := cloneTraceSpec(traceSpecFromRequest(resp.Request)); ts != nil && ts.Enabled {
		sn.traceSpec = ts
	}
	if resp.Timeline != nil {
		sn.timeline = resp.Timeline.Clone()
		sn.traceReport = buildTimelineReport(
			resp.Timeline,
			sn.traceSpec,
			resp.TraceReport,
			newTimelineStyles(&m.theme),
		)
	}
	if resp.TraceReport != nil {
		sn.traceData = resp.TraceReport.Clone()
	}
	applyRawViewMode(sn, sn.rawMode)
	return sn
}

func (m *Model) newGRPCSnapshot(
	req *restfile.Request,
	resp *grpcclient.Response,
	env string,
) *responseSnapshot {
	if resp == nil {
		return nil
	}
	var b strings.Builder
	ct := strings.TrimSpace(resp.ContentType)
	if len(resp.Headers) > 0 {
		b.WriteString("Headers:\n")
		for name, vals := range resp.Headers {
			fmt.Fprintf(&b, "%s: %s\n", name, strings.Join(vals, ", "))
			if strings.EqualFold(name, "Content-Type") && ct == "" && len(vals) > 0 {
				ct = strings.TrimSpace(vals[0])
			}
		}
	}
	if len(resp.Trailers) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("Trailers:\n")
		for name, vals := range resp.Trailers {
			fmt.Fprintf(&b, "%s: %s\n", name, strings.Join(vals, ", "))
		}
	}
	hdr := strings.TrimRight(b.String(), "\n")

	target := ""
	if req != nil && req.GRPC != nil {
		target = strings.TrimPrefix(strings.TrimSpace(req.GRPC.FullMethod), "/")
	}
	if target == "" {
		target = strings.TrimPrefix(strings.TrimSpace(requestTarget(req)), "/")
	}
	status := fmt.Sprintf("gRPC %s - %s", target, resp.StatusCode.String())
	if resp.StatusMessage != "" {
		status += " (" + resp.StatusMessage + ")"
	}

	viewBody := append([]byte(nil), resp.Body...)
	if len(viewBody) == 0 && strings.TrimSpace(resp.Message) != "" {
		viewBody = []byte(resp.Message)
	}
	viewCT := strings.TrimSpace(resp.ContentType)
	if viewCT == "" && len(viewBody) > 0 {
		viewCT = "application/json"
	}
	rawBody := append([]byte(nil), resp.Wire...)
	if len(rawBody) == 0 {
		rawBody = append([]byte(nil), viewBody...)
	}
	rawCT := strings.TrimSpace(resp.WireContentType)
	if rawCT == "" {
		rawCT = ct
	}
	if rawCT == "" {
		rawCT = viewCT
	}
	meta := binaryview.Analyze(viewBody, viewCT)
	bv := buildBodyViews(rawBody, rawCT, &meta, viewBody, viewCT)

	sn := &responseSnapshot{
		id:              nextResponseRenderToken(),
		pretty:          joinSections(status, bv.pretty),
		raw:             joinSections(status, bv.raw),
		rawSummary:      status,
		headers:         joinSections(status, hdr),
		ready:           true,
		environment:     strings.TrimSpace(env),
		body:            rawBody,
		bodyMeta:        meta,
		contentType:     rawCT,
		rawText:         bv.rawText,
		rawHex:          bv.rawHex,
		rawBase64:       bv.rawBase64,
		rawMode:         bv.mode,
		responseHeaders: grpcHeaderMap(resp),
	}
	applyRawViewMode(sn, sn.rawMode)
	return sn
}

func grpcHeaderMap(resp *grpcclient.Response) http.Header {
	if resp == nil || (len(resp.Headers) == 0 && len(resp.Trailers) == 0) {
		return nil
	}
	out := make(http.Header, len(resp.Headers)+len(resp.Trailers))
	for k, v := range resp.Headers {
		out[k] = append([]string(nil), v...)
	}
	for k, v := range resp.Trailers {
		out["Grpc-Trailer-"+k] = append([]string(nil), v...)
	}
	return out
}

func newSkippedSnapshot(reason, env string) *responseSnapshot {
	body := strings.TrimSpace(reason)
	if body == "" {
		body = "Condition evaluated to false."
	}
	return &responseSnapshot{
		id:          nextResponseRenderToken(),
		pretty:      joinSections("Request Skipped", body),
		raw:         joinSections("Request Skipped", body),
		headers:     joinSections("Request Skipped", body),
		ready:       true,
		environment: strings.TrimSpace(env),
	}
}

func newErrorSnapshot(err error, env string) *responseSnapshot {
	code := errdef.CodeOf(err)
	title := requestErrorTitle(code)
	detail := strings.TrimSpace(errdef.Message(err))
	if detail == "" {
		detail = "Request failed with no additional details."
	}
	note := requestErrorNote(code)
	meta := []string{}
	if code != errdef.CodeUnknown && string(code) != "" {
		meta = append(meta, fmt.Sprintf("Code: %s", strings.ToUpper(string(code))))
	}
	if strings.TrimSpace(note) != "" {
		meta = append(meta, note)
	}
	return &responseSnapshot{
		id:          nextResponseRenderToken(),
		pretty:      joinSections(title, detail, note),
		raw:         joinSections(title, detail),
		headers:     joinSections(title, strings.Join(meta, "\n"), detail),
		ready:       true,
		environment: strings.TrimSpace(env),
	}
}

func newTextSnapshot(body, env string) *responseSnapshot {
	body = strings.TrimSpace(body)
	if body == "" {
		body = noResponseMessage
	}
	return &responseSnapshot{
		id:             nextResponseRenderToken(),
		pretty:         body,
		raw:            body,
		headers:        body,
		requestHeaders: body,
		ready:          true,
		environment:    strings.TrimSpace(env),
	}
}

func newStreamSnapshot(
	info *scripts.StreamInfo,
	raw []byte,
	env string,
) *responseSnapshot {
	body := strings.TrimSpace(string(raw))
	if body == "" && info != nil {
		body = streamSummaryText(info)
	}
	if body == "" {
		body = "<no transcript captured>"
	}
	return newTextSnapshot(body, env)
}

func streamSummaryText(info *scripts.StreamInfo) string {
	if info == nil {
		return ""
	}
	lines := []string{}
	if kind := strings.TrimSpace(info.Kind); kind != "" {
		lines = append(lines, "Stream: "+kind)
	}
	for k, v := range info.Summary {
		lines = append(lines, fmt.Sprintf("%s: %v", k, v))
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

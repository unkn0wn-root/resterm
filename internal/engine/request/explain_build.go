package request

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/k8s"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/ssh"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

const explainClip = 512

type explainAuthPreviewResult struct {
	status       xplain.StageStatus
	summary      string
	notes        []string
	extraSecrets []string
}

type explainFinalizeInput struct {
	report       *xplain.Report
	doc          *restfile.Document
	req          *restfile.Request
	env          string
	preview      bool
	status       xplain.Status
	decision     string
	err          error
	trace        *vars.Trace
	settings     map[string]string
	ssh          *ssh.Plan
	k8s          *k8s.Plan
	globals      map[string]scripts.GlobalValue
	extraSecrets []string
}

type explainBuilder struct {
	eng          *Engine
	doc          *restfile.Document
	req          *restfile.Request
	env          string
	preview      bool
	report       *xplain.Report
	trace        *vars.Trace
	settings     map[string]string
	ssh          *ssh.Plan
	k8s          *k8s.Plan
	globals      map[string]scripts.GlobalValue
	extraSecrets []string
}

func newExplainBuilder(
	e *Engine,
	doc *restfile.Document,
	req *restfile.Request,
	env string,
	preview bool,
) *explainBuilder {
	b := &explainBuilder{
		eng:     e,
		doc:     doc,
		req:     req,
		env:     env,
		preview: preview,
		report:  newExplainReport(req, env),
	}
	if e != nil {
		b.globals = effectiveGlobalValues(doc, e.collectStoredGlobalValues(env))
	}
	if !preview || req == nil {
		return b
	}
	if req.Metadata.ForEach != nil {
		b.warn("@for-each iterations are not expanded in explain preview")
	}
	if req.Metadata.Compare != nil {
		b.warn("@compare sweep is not executed in explain preview")
	}
	if req.Metadata.Profile != nil {
		b.warn("@profile run is not executed in explain preview")
	}
	return b
}

func (b *explainBuilder) warn(msg string) {
	addExplainWarn(b.report, msg)
}

func (b *explainBuilder) stage(
	name string,
	st xplain.StageStatus,
	sum string,
	before *restfile.Request,
	after *restfile.Request,
	notes ...string,
) {
	addExplainStage(b.report, name, st, sum, before, after, notes...)
}

func (b *explainBuilder) sentHTTP(
	req *restfile.Request,
	resp *httpclient.Response,
	notes ...string,
) {
	addExplainSentHTTPStage(b.report, req, resp, notes...)
}

func (b *explainBuilder) setSettings(settings map[string]string) {
	b.settings = settings
}

func (b *explainBuilder) setRoute(ssh *ssh.Plan, k8s *k8s.Plan) {
	b.ssh = ssh
	b.k8s = k8s
}

func (b *explainBuilder) addSecrets(vals ...string) {
	for _, val := range vals {
		if strings.TrimSpace(val) == "" {
			continue
		}
		b.extraSecrets = append(b.extraSecrets, val)
	}
}

func (b *explainBuilder) setPrepared(req *restfile.Request) {
	setExplainPrepared(b.report, req, b.settings, b.ssh, b.k8s)
}

func (b *explainBuilder) setGRPC(req *restfile.Request) {
	setExplainGRPC(b.report, req)
}

func (b *explainBuilder) setHTTP(resp *httpclient.Response) {
	setExplainHTTP(b.report, resp)
}

func (b *explainBuilder) finish(
	status xplain.Status,
	decision string,
	err error,
) *xplain.Report {
	if b == nil || b.eng == nil {
		return nil
	}
	return b.eng.finalizeExplainReport(explainFinalizeInput{
		report:       b.report,
		doc:          b.doc,
		req:          b.req,
		env:          b.env,
		preview:      b.preview,
		status:       status,
		decision:     decision,
		err:          err,
		trace:        b.trace,
		settings:     b.settings,
		ssh:          b.ssh,
		k8s:          b.k8s,
		globals:      b.globals,
		extraSecrets: b.extraSecrets,
	})
}

func (e *Engine) finalizeExplainReport(in explainFinalizeInput) *xplain.Report {
	if in.report == nil {
		return nil
	}
	if in.trace != nil {
		finalizeExplainVars(in.report, in.trace)
	}
	if in.report.Final == nil {
		setExplainPrepared(in.report, in.req, in.settings, in.ssh, in.k8s)
	}
	fail := in.report.Failure
	if in.err != nil && strings.TrimSpace(fail) == "" {
		fail = in.err.Error()
	}
	decision := strings.TrimSpace(in.decision)
	if decision == "" {
		decision = strings.TrimSpace(in.report.Decision)
	}
	if decision == "" {
		switch in.status {
		case xplain.StatusSkipped:
			decision = "Request skipped"
		case xplain.StatusError:
			decision = "Request preparation failed"
		default:
			if in.preview {
				decision = "Explain preview ready"
			} else {
				decision = "Request prepared"
			}
		}
	}
	setExplainDecision(in.report, in.status, decision, fail)
	return e.redactExplainReport(
		in.report,
		in.doc,
		in.req,
		in.env,
		in.globals,
		in.extraSecrets...,
	)
}

func newExplainReport(req *restfile.Request, env string) *xplain.Report {
	rep := &xplain.Report{
		Env:    strings.TrimSpace(env),
		Status: xplain.StatusReady,
	}
	if req == nil {
		return rep
	}
	rep.Name = strings.TrimSpace(req.Metadata.Name)
	rep.Method = strings.TrimSpace(req.Method)
	rep.URL = strings.TrimSpace(req.URL)
	return rep
}

func setExplainDecision(rep *xplain.Report, st xplain.Status, decision, failure string) {
	if rep == nil {
		return
	}
	rep.Status = st
	rep.Decision = strings.TrimSpace(decision)
	rep.Failure = strings.TrimSpace(failure)
}

func addExplainStage(
	rep *xplain.Report,
	name string,
	st xplain.StageStatus,
	sum string,
	before *restfile.Request,
	after *restfile.Request,
	notes ...string,
) {
	if rep == nil {
		return
	}
	appendExplainStage(rep, xplain.Stage{
		Name:    strings.TrimSpace(name),
		Status:  st,
		Summary: strings.TrimSpace(sum),
		Changes: explainReqChanges(before, after),
		Notes:   explainNotes(notes),
	})
}

func appendExplainStage(rep *xplain.Report, st xplain.Stage) {
	if rep == nil {
		return
	}
	if st.Summary == "" {
		switch {
		case len(st.Changes) > 0:
			st.Summary = fmt.Sprintf("%d change(s)", len(st.Changes))
		case len(st.Notes) > 0:
			st.Summary = "no request changes"
		default:
			st.Summary = "no changes"
		}
	}
	rep.Stages = append(rep.Stages, st)
}

func addExplainPreparedHTTPStage(
	rep *xplain.Report,
	req *restfile.Request,
	httpReq *http.Request,
	body []byte,
	notes ...string,
) {
	if rep == nil || httpReq == nil {
		return
	}
	appendExplainStage(rep, xplain.Stage{
		Name:    xplain.StageHTTPPrepare,
		Status:  xplain.StageOK,
		Summary: xplain.SummaryHTTPRequestPrepared,
		Changes: explainBuiltHTTPChanges(req, httpReq, body),
		Notes:   explainNotes(notes),
	})
}

func addExplainSentHTTPStage(
	rep *xplain.Report,
	req *restfile.Request,
	resp *httpclient.Response,
	notes ...string,
) {
	if rep == nil || resp == nil {
		return
	}
	appendExplainStage(rep, xplain.Stage{
		Name:    xplain.StageHTTPPrepare,
		Status:  xplain.StageOK,
		Summary: xplain.SummaryHTTPRequestPrepared,
		Changes: explainSentHTTPChanges(req, resp),
		Notes:   explainNotes(notes),
	})
}

func addExplainWarn(rep *xplain.Report, msg string) {
	if rep == nil {
		return
	}
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return
	}
	rep.Warnings = append(rep.Warnings, msg)
}

func finalizeExplainVars(rep *xplain.Report, tr *vars.Trace) {
	if rep == nil || tr == nil {
		return
	}
	items := tr.Items()
	if len(items) == 0 {
		return
	}
	out := make([]xplain.Var, 0, len(items))
	for _, it := range items {
		out = append(out, xplain.Var{
			Name:     strings.TrimSpace(it.Name),
			Source:   strings.TrimSpace(it.Source),
			Value:    it.Value,
			Shadowed: append([]string(nil), it.Shadowed...),
			Uses:     it.Uses,
			Missing:  it.Missing,
			Dynamic:  it.Dynamic,
		})
	}
	rep.Vars = out
}

func setExplainPrepared(
	rep *xplain.Report,
	req *restfile.Request,
	settings map[string]string,
	ssh *ssh.Plan,
	k8s *k8s.Plan,
) {
	if rep == nil {
		return
	}
	final := &xplain.Final{
		Mode:     "prepared",
		Settings: explainSettings(settings),
		Route:    explainRoute(ssh, k8s),
	}
	fillExplainFinal(final, req)
	rep.Final = final
}

func setExplainGRPC(rep *xplain.Report, req *restfile.Request) {
	if rep == nil {
		return
	}
	final := rep.Final
	if final == nil {
		final = &xplain.Final{}
		rep.Final = final
	}
	fillExplainFinal(final, req)
	final.Mode = "sent"
}

func setExplainHTTP(rep *xplain.Report, resp *httpclient.Response) {
	if rep == nil || resp == nil {
		return
	}
	final := rep.Final
	if final == nil {
		final = &xplain.Final{}
		rep.Final = final
	}
	if resp.Request != nil {
		fillExplainFinal(final, resp.Request)
	}
	final.Mode = "sent"
	final.Method = strings.TrimSpace(resp.ReqMethod)
	final.URL = strings.TrimSpace(resp.EffectiveURL)
	final.Headers = explainHeaders(resp.RequestHeaders)
	if strings.TrimSpace(final.Protocol) == "" {
		final.Protocol = "HTTP"
	}
}

func setExplainHTTPPrepared(
	rep *xplain.Report,
	req *restfile.Request,
	httpReq *http.Request,
	body []byte,
) {
	if rep == nil || httpReq == nil {
		return
	}
	final := rep.Final
	if final == nil {
		final = &xplain.Final{Mode: "prepared"}
		rep.Final = final
	}
	if req != nil && strings.TrimSpace(final.Protocol) == "" {
		final.Protocol = explainProtocol(req)
	}
	final.Method = strings.TrimSpace(httpReq.Method)
	if httpReq.URL != nil {
		final.URL = strings.TrimSpace(httpReq.URL.String())
	}
	final.Headers = explainHeaders(httpReq.Header)
	txt, note, ok := explainBuiltBody(req, body)
	if !ok {
		return
	}
	final.Body = txt
	final.BodyNote = note
}

func fillExplainFinal(final *xplain.Final, req *restfile.Request) {
	if final == nil {
		return
	}
	body, note := explainReqBody(req)
	final.Protocol = explainProtocol(req)
	final.Method = strings.TrimSpace(reqMethod(req))
	final.URL = strings.TrimSpace(reqURL(req))
	final.Headers = explainHeaders(reqHeaders(req))
	final.Body = body
	final.BodyNote = note
	final.Details = explainProtocolDetails(req)
	final.Steps = explainProtocolSteps(req)
}

func explainReqChanges(a, b *restfile.Request) []xplain.Change {
	if a == nil && b == nil {
		return nil
	}
	var out []xplain.Change
	addExplainChange(&out, "method", reqMethod(a), reqMethod(b))
	addExplainChange(&out, "url", reqURL(a), reqURL(b))
	addExplainBodyChange(&out, a, b)
	addExplainHeaderChanges(&out, reqHeaders(a), reqHeaders(b))
	addExplainSettingChanges(&out, reqSettings(a), reqSettings(b))
	addExplainVarChanges(&out, reqVars(a), reqVars(b))
	addExplainGRPCChanges(&out, a, b)
	return out
}

func explainBuiltHTTPChanges(
	req *restfile.Request,
	httpReq *http.Request,
	body []byte,
) []xplain.Change {
	if httpReq == nil {
		return nil
	}
	var out []xplain.Change
	addExplainChange(&out, "method", reqMethod(req), strings.TrimSpace(httpReq.Method))
	url := ""
	if httpReq.URL != nil {
		url = strings.TrimSpace(httpReq.URL.String())
	}
	addExplainChange(&out, "url", reqURL(req), url)
	addExplainHeaderChanges(&out, reqHeaders(req), httpReq.Header)
	beforeBody, beforeNote := explainReqBody(req)
	afterBody, afterNote, ok := explainBuiltBody(req, body)
	if ok || beforeNote != "" {
		addExplainChange(&out, "body.note", beforeNote, afterNote)
	}
	if ok || beforeBody != "" {
		addExplainChange(&out, "body", beforeBody, afterBody)
	}
	return out
}

func explainSentHTTPChanges(req *restfile.Request, resp *httpclient.Response) []xplain.Change {
	if resp == nil {
		return nil
	}
	var out []xplain.Change
	addExplainChange(&out, "method", reqMethod(req), strings.TrimSpace(resp.ReqMethod))
	addExplainChange(&out, "url", reqURL(req), strings.TrimSpace(resp.EffectiveURL))
	addExplainHeaderChanges(&out, reqHeaders(req), resp.RequestHeaders)
	return out
}

func addExplainChange(out *[]xplain.Change, field, before, after string) {
	before = strings.TrimSpace(before)
	after = strings.TrimSpace(after)
	if before == after {
		return
	}
	*out = append(
		*out,
		xplain.Change{Field: field, Before: clipExplain(before), After: clipExplain(after)},
	)
}

func addExplainBodyChange(out *[]xplain.Change, a, b *restfile.Request) {
	ab, an := explainReqBody(a)
	bb, bn := explainReqBody(b)
	addExplainChange(out, "body.note", an, bn)
	addExplainChange(out, "body", ab, bb)
}

func addExplainHeaderChanges(out *[]xplain.Change, a, b http.Header) {
	for _, name := range mergedKeySet(a, b) {
		addExplainChange(out, "header."+name, headerValue(a, name), headerValue(b, name))
	}
}

func addExplainSettingChanges(out *[]xplain.Change, a, b map[string]string) {
	addExplainMapChanges(out, "setting.", a, b)
}

func addExplainVarChanges(out *[]xplain.Change, a, b map[string]string) {
	addExplainMapChanges(out, "var.", a, b)
}

func addExplainMapChanges(out *[]xplain.Change, prefix string, a, b map[string]string) {
	for _, name := range mergedKeySet(a, b) {
		addExplainChange(out, prefix+name, explainMapValue(a, name), explainMapValue(b, name))
	}
}

func mergedKeySet[M ~map[string]V, V any](a, b M) []string {
	keys := make(map[string]string, len(a)+len(b))
	add := func(src M) {
		for name := range src {
			display := strings.TrimSpace(name)
			key := normalizedExplainKey(display)
			if key == "" {
				continue
			}
			if _, ok := keys[key]; ok {
				continue
			}
			keys[key] = display
		}
	}
	add(a)
	add(b)
	if len(keys) == 0 {
		return nil
	}
	names := make([]string, 0, len(keys))
	for _, name := range keys {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		left := normalizedExplainKey(names[i])
		right := normalizedExplainKey(names[j])
		if left == right {
			return names[i] < names[j]
		}
		return left < right
	})
	return names
}

func explainMapValue(vals map[string]string, name string) string {
	want := normalizedExplainKey(name)
	for key, val := range vals {
		if normalizedExplainKey(key) == want {
			return val
		}
	}
	return ""
}

func normalizedExplainKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func addExplainGRPCChanges(out *[]xplain.Change, a, b *restfile.Request) {
	var at, bt, am, bm string
	if a != nil && a.GRPC != nil {
		at = strings.TrimSpace(a.GRPC.Target)
		am = strings.TrimSpace(a.GRPC.Message)
	}
	if b != nil && b.GRPC != nil {
		bt = strings.TrimSpace(b.GRPC.Target)
		bm = strings.TrimSpace(b.GRPC.Message)
	}
	addExplainChange(out, "grpc.target", at, bt)
	addExplainChange(out, "grpc.message", am, bm)
}

func explainReqBody(req *restfile.Request) (string, string) {
	if req == nil {
		return "", ""
	}
	switch {
	case req.GRPC != nil:
		if s := strings.TrimSpace(
			req.GRPC.MessageExpanded,
		); req.GRPC.MessageExpandedSet &&
			s != "" {
			note := "gRPC message"
			if path := strings.TrimSpace(req.GRPC.MessageFile); path != "" {
				note = "expanded gRPC message from " + path
			}
			return clipExplain(s), note
		}
		if s := strings.TrimSpace(req.GRPC.Message); s != "" {
			return clipExplain(s), "gRPC message"
		}
		if s := strings.TrimSpace(req.GRPC.MessageFile); s != "" {
			return "", "gRPC message from " + s
		}
	case req.Body.GraphQL != nil:
		gql := req.Body.GraphQL
		if s := strings.TrimSpace(gql.Query); s != "" {
			return clipExplain(s), "graphql query"
		}
		if s := strings.TrimSpace(gql.QueryFile); s != "" {
			return "", "graphql query from " + s
		}
	case strings.TrimSpace(req.Body.Text) != "":
		return clipExplain(req.Body.Text), ""
	case strings.TrimSpace(req.Body.FilePath) != "":
		return "", "< " + strings.TrimSpace(req.Body.FilePath)
	}
	return "", ""
}

func explainBuiltBody(req *restfile.Request, body []byte) (string, string, bool) {
	if len(body) == 0 {
		if req != nil && req.Body.GraphQL != nil && strings.EqualFold(req.Method, "GET") {
			return "", "graphql query encoded in URL", true
		}
		return "", "", false
	}

	note := ""
	switch {
	case req != nil && req.Body.GraphQL != nil:
		note = "graphql payload"
	case req != nil && strings.TrimSpace(req.Body.FilePath) != "":
		path := strings.TrimSpace(req.Body.FilePath)
		note = "body from " + path
		if req.Body.Options.ExpandTemplates {
			note = "expanded body from " + path
		}
	}

	if !utf8.Valid(body) {
		if note == "" {
			note = fmt.Sprintf("binary body (%d bytes)", len(body))
		} else {
			note = fmt.Sprintf("%s (%d bytes)", note, len(body))
		}
		return "", note, true
	}

	return clipExplain(string(body)), note, true
}

func explainHeaders(h http.Header) []xplain.Header {
	if len(h) == 0 {
		return nil
	}
	names := make([]string, 0, len(h))
	for name := range h {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]xplain.Header, 0, len(names))
	for _, name := range names {
		for _, val := range h.Values(name) {
			out = append(out, xplain.Header{Name: name, Value: val})
		}
	}
	return out
}

func explainSettings(settings map[string]string) []xplain.Pair {
	if len(settings) == 0 {
		return nil
	}
	keys := make([]string, 0, len(settings))
	for key := range settings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]xplain.Pair, 0, len(keys))
	for _, key := range keys {
		out = append(out, xplain.Pair{Key: key, Value: settings[key]})
	}
	return out
}

func explainProtocol(req *restfile.Request) string {
	switch {
	case req == nil:
		return ""
	case req.GRPC != nil:
		return "gRPC"
	case req.WebSocket != nil:
		return "WebSocket"
	case req.SSE != nil:
		return "SSE"
	default:
		return "HTTP"
	}
}

func explainProtocolDetails(req *restfile.Request) []xplain.Pair {
	switch {
	case req == nil:
		return nil
	case req.GRPC != nil:
		return explainGRPCDetails(req.GRPC)
	case req.WebSocket != nil:
		return explainWebSocketDetails(req.WebSocket)
	case req.SSE != nil:
		return explainSSEDetails(req.SSE)
	default:
		return explainHTTPDetails(req)
	}
}

func explainProtocolSteps(req *restfile.Request) []string {
	if req == nil || req.WebSocket == nil || len(req.WebSocket.Steps) == 0 {
		return nil
	}
	out := make([]string, 0, len(req.WebSocket.Steps))
	for _, st := range req.WebSocket.Steps {
		if line := explainWebSocketStep(st); line != "" {
			out = append(out, line)
		}
	}
	return out
}

func explainHTTPDetails(req *restfile.Request) []xplain.Pair {
	if req == nil || req.Body.GraphQL == nil {
		return nil
	}
	var out []xplain.Pair
	if op := strings.TrimSpace(req.Body.GraphQL.OperationName); op != "" {
		out = append(out, xplain.Pair{Key: "GraphQL Operation", Value: op})
	}
	return out
}

func explainGRPCDetails(gr *restfile.GRPCRequest) []xplain.Pair {
	if gr == nil {
		return nil
	}
	out := make([]xplain.Pair, 0, len(gr.Metadata)+4)
	if method := strings.TrimPrefix(strings.TrimSpace(gr.FullMethod), "/"); method != "" {
		out = append(out, xplain.Pair{Key: "RPC", Value: method})
	}
	if auth := strings.TrimSpace(gr.Authority); auth != "" {
		out = append(out, xplain.Pair{Key: "Authority", Value: auth})
	}
	if gr.PlaintextSet {
		mode := "tls"
		if gr.Plaintext {
			mode = "plaintext"
		}
		out = append(out, xplain.Pair{Key: "Transport", Value: mode})
	}
	if desc := strings.TrimSpace(gr.DescriptorSet); desc != "" {
		out = append(out, xplain.Pair{Key: "Descriptor Set", Value: desc})
	}
	out = append(out, xplain.Pair{Key: "Reflection", Value: explainToggle(gr.UseReflection)})
	for _, pair := range gr.Metadata {
		key := strings.TrimSpace(pair.Key)
		if key == "" {
			continue
		}
		out = append(out, xplain.Pair{
			Key:   "Metadata",
			Value: key + ": " + strings.TrimSpace(pair.Value),
		})
	}
	return out
}

func explainWebSocketDetails(ws *restfile.WebSocketRequest) []xplain.Pair {
	if ws == nil {
		return nil
	}
	opts := ws.Options
	out := make([]xplain.Pair, 0, 5)
	if opts.HandshakeTimeout > 0 {
		out = append(
			out,
			xplain.Pair{Key: "Handshake Timeout", Value: opts.HandshakeTimeout.String()},
		)
	}
	if opts.IdleTimeout > 0 {
		out = append(out, xplain.Pair{Key: "Idle Timeout", Value: opts.IdleTimeout.String()})
	}
	if opts.MaxMessageBytes > 0 {
		out = append(
			out,
			xplain.Pair{Key: "Max Message Bytes", Value: fmt.Sprintf("%d", opts.MaxMessageBytes)},
		)
	}
	if len(opts.Subprotocols) > 0 {
		out = append(
			out,
			xplain.Pair{Key: "Subprotocols", Value: strings.Join(opts.Subprotocols, ", ")},
		)
	}
	if opts.CompressionSet {
		out = append(out, xplain.Pair{Key: "Compression", Value: explainToggle(opts.Compression)})
	}
	return out
}

func explainSSEDetails(sse *restfile.SSERequest) []xplain.Pair {
	if sse == nil {
		return nil
	}
	opts := sse.Options
	out := make([]xplain.Pair, 0, 4)
	if opts.TotalTimeout > 0 {
		out = append(out, xplain.Pair{Key: "Total Timeout", Value: opts.TotalTimeout.String()})
	}
	if opts.IdleTimeout > 0 {
		out = append(out, xplain.Pair{Key: "Idle Timeout", Value: opts.IdleTimeout.String()})
	}
	if opts.MaxEvents > 0 {
		out = append(out, xplain.Pair{Key: "Max Events", Value: fmt.Sprintf("%d", opts.MaxEvents)})
	}
	if opts.MaxBytes > 0 {
		out = append(out, xplain.Pair{Key: "Max Bytes", Value: fmt.Sprintf("%d", opts.MaxBytes)})
	}
	return out
}

func explainToggle(on bool) string {
	if on {
		return "enabled"
	}
	return "disabled"
}

func explainWebSocketStep(st restfile.WebSocketStep) string {
	switch st.Type {
	case restfile.WebSocketStepSendText:
		if val := strings.TrimSpace(st.Value); val != "" {
			return "Send text " + clipExplain(val)
		}
	case restfile.WebSocketStepSendJSON:
		if val := strings.TrimSpace(st.Value); val != "" {
			return "Send JSON " + clipExplain(val)
		}
	case restfile.WebSocketStepSendBase64:
		if val := strings.TrimSpace(st.Value); val != "" {
			return "Send base64 " + clipExplain(val)
		}
	case restfile.WebSocketStepSendFile:
		if path := strings.TrimSpace(st.File); path != "" {
			return "Send file " + path
		}
	case restfile.WebSocketStepPing:
		if val := strings.TrimSpace(st.Value); val != "" {
			return "Ping " + clipExplain(val)
		}
		return "Ping"
	case restfile.WebSocketStepPong:
		if val := strings.TrimSpace(st.Value); val != "" {
			return "Pong " + clipExplain(val)
		}
		return "Pong"
	case restfile.WebSocketStepWait:
		if st.Duration > 0 {
			return "Wait " + st.Duration.String()
		}
		return "Wait"
	case restfile.WebSocketStepClose:
		switch {
		case st.Code > 0 && strings.TrimSpace(st.Reason) != "":
			return fmt.Sprintf("Close %d %s", st.Code, strings.TrimSpace(st.Reason))
		case st.Code > 0:
			return fmt.Sprintf("Close %d", st.Code)
		case strings.TrimSpace(st.Reason) != "":
			return "Close " + strings.TrimSpace(st.Reason)
		default:
			return "Close"
		}
	}
	return ""
}

func explainRoute(ssh *ssh.Plan, k8s *k8s.Plan) *xplain.Route {
	switch {
	case ssh != nil && ssh.Active():
		cfg := ssh.Config
		if cfg == nil {
			return nil
		}
		sum := cfg.Host
		if cfg.Port > 0 {
			sum = fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
		}
		if cfg.User != "" {
			sum = fmt.Sprintf("%s@%s", cfg.User, sum)
		}
		var notes []string
		if cfg.Name != "" {
			notes = append(notes, "profile="+cfg.Name)
		}
		if !cfg.Strict {
			notes = append(notes, "strict_hostkey=false")
		}
		return &xplain.Route{Kind: xplain.RouteKindSSH, Summary: sum, Notes: notes}
	case k8s != nil && k8s.Active():
		cfg := k8s.Config
		if cfg == nil {
			return nil
		}
		sum := cfg.Namespace
		if cfg.TargetKind != "" && cfg.TargetName != "" {
			if sum != "" {
				sum += " "
			}
			sum += string(cfg.TargetKind) + "/" + cfg.TargetName
		}
		if cfg.PortName != "" {
			sum += ":" + cfg.PortName
		} else if cfg.Port > 0 {
			sum += fmt.Sprintf(":%d", cfg.Port)
		}
		var notes []string
		if cfg.Context != "" {
			notes = append(notes, "context="+cfg.Context)
		}
		if cfg.Container != "" {
			notes = append(notes, "container="+cfg.Container)
		}
		return &xplain.Route{Kind: xplain.RouteKindK8s, Summary: sum, Notes: notes}
	default:
		return &xplain.Route{Kind: xplain.RouteKindDirect, Summary: "direct connection"}
	}
}

func explainNotes(notes []string) []string {
	out := make([]string, 0, len(notes))
	for _, note := range notes {
		note = strings.TrimSpace(note)
		if note == "" {
			continue
		}
		out = append(out, note)
	}
	return out
}

func reqMethod(req *restfile.Request) string {
	if req == nil {
		return ""
	}
	return req.Method
}

func reqURL(req *restfile.Request) string {
	if req == nil {
		return ""
	}
	if req.GRPC != nil && strings.TrimSpace(req.GRPC.Target) != "" {
		return req.GRPC.Target
	}
	return req.URL
}

func reqSettings(req *restfile.Request) map[string]string {
	if req == nil {
		return nil
	}
	return req.Settings
}

func reqVars(req *restfile.Request) map[string]string {
	if req == nil || len(req.Variables) == 0 {
		return nil
	}
	out := make(map[string]string, len(req.Variables))
	names := make(map[string]string, len(req.Variables))
	for _, v := range req.Variables {
		name := strings.TrimSpace(v.Name)
		key := normalizedExplainKey(name)
		if key == "" {
			continue
		}
		if prev, ok := names[key]; ok {
			out[prev] = v.Value
			continue
		}
		names[key] = name
		out[name] = v.Value
	}
	return out
}

func clipExplain(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	rs := []rune(s)
	if len(rs) <= explainClip {
		return s
	}
	return strings.TrimSpace(string(rs[:explainClip])) + " ..."
}

func authSecretValues(auth *restfile.AuthSpec, res *vars.Resolver) []string {
	if auth == nil || len(auth.Params) == 0 {
		return nil
	}
	expand := func(key string) string {
		val := strings.TrimSpace(auth.Params[key])
		if val == "" {
			return ""
		}
		if res == nil {
			return val
		}
		out, err := res.ExpandTemplates(val)
		if err != nil {
			return val
		}
		return strings.TrimSpace(out)
	}

	vals := make(map[string]struct{})
	add := func(val string) {
		if strings.TrimSpace(val) == "" {
			return
		}
		vals[val] = struct{}{}
	}

	switch strings.ToLower(strings.TrimSpace(auth.Type)) {
	case "basic":
		add(expand("password"))
	case "bearer":
		add(expand("token"))
	case "apikey", "api-key", "header":
		add(expand("value"))
	case "oauth2":
		for _, key := range []string{"client_secret", "password", "refresh_token", "access_token"} {
			add(expand(key))
		}
	}

	if len(vals) == 0 {
		return nil
	}
	out := make([]string, 0, len(vals))
	for val := range vals {
		out = append(out, val)
	}
	return out
}

func (e *Engine) prepareExplainAuthPreview(
	doc *restfile.Document,
	req *restfile.Request,
	res *vars.Resolver,
	env string,
) (explainAuthPreviewResult, error) {
	if req == nil || req.Metadata.Auth == nil {
		return explainAuthPreviewResult{}, nil
	}
	auth := req.Metadata.Auth
	kind := strings.ToLower(strings.TrimSpace(auth.Type))
	switch kind {
	case "", "basic", "bearer", "apikey", "api-key", "header":
		return explainAuthPreviewResult{
			status:  xplain.StageOK,
			summary: xplain.SummaryAuthPrepared,
			notes:   []string{"auth headers/query are applied during HTTP request build"},
		}, nil
	case "command":
		prep, err := e.prepareCommandAuth(doc, auth, res, env, 0)
		if err != nil {
			return explainAuthPreviewResult{}, err
		}
		hdr := prep.HeaderName()
		if req.Headers != nil && req.Headers.Get(hdr) != "" {
			return explainAuthPreviewResult{
				status:  xplain.StageOK,
				summary: xplain.SummaryAuthPrepared,
				notes:   []string{"auth header already set on request"},
			}, nil
		}
		ac := e.rt.AuthCmd()
		if ac == nil {
			return explainAuthPreviewResult{}, errdef.New(
				errdef.CodeHTTP,
				"command auth support is not initialised",
			)
		}
		out, ok, err := ac.CachedPrepared(prep)
		if err != nil {
			return explainAuthPreviewResult{}, err
		}
		if !ok {
			return explainAuthPreviewResult{
				status:  xplain.StageSkipped,
				summary: xplain.SummaryCommandAuthExecutionSkipped,
				notes: []string{
					"Command auth execution is skipped in explain preview",
					fmt.Sprintf("%s is omitted without a cached command auth result", hdr),
				},
			}, nil
		}
		if req.Headers == nil {
			req.Headers = make(http.Header)
		}
		req.Headers.Set(out.Header, out.Value)
		return explainAuthPreviewResult{
			status:       xplain.StageOK,
			summary:      xplain.SummaryAuthPrepared,
			notes:        []string{"used cached command auth result for explain preview"},
			extraSecrets: commandAuthSecrets(out),
		}, nil
	case "oauth2":
		oa := e.rt.OAuth()
		if oa == nil {
			return explainAuthPreviewResult{}, errdef.New(
				errdef.CodeHTTP,
				"oauth support is not initialised",
			)
		}
		cfg, err := e.buildOAuthConfig(auth, res)
		if err != nil {
			return explainAuthPreviewResult{}, err
		}
		env = e.envName(env)
		cfg = oa.MergeCachedConfig(env, cfg)
		if cfg.TokenURL == "" {
			return explainAuthPreviewResult{}, errdef.New(
				errdef.CodeHTTP,
				"@auth oauth2 requires token_url (include it once per cache_key to seed the cache)",
			)
		}
		hdr := cfg.Header
		if req.Headers != nil && req.Headers.Get(hdr) != "" {
			return explainAuthPreviewResult{
				status:  xplain.StageOK,
				summary: xplain.SummaryAuthPrepared,
				notes:   []string{"auth header already set on request"},
			}, nil
		}
		tok, ok := oa.CachedToken(env, cfg)
		if !ok {
			return explainAuthPreviewResult{
				status:  xplain.StageSkipped,
				summary: xplain.SummaryOAuthTokenFetchSkipped,
				notes: []string{
					"OAuth token acquisition is skipped in explain preview",
					fmt.Sprintf("%s is omitted without a cached token", hdr),
				},
			}, nil
		}
		if req.Headers == nil {
			req.Headers = make(http.Header)
		}
		val := tok.AccessToken
		if strings.EqualFold(hdr, "authorization") {
			typ := strings.TrimSpace(tok.TokenType)
			if typ == "" {
				typ = "Bearer"
			}
			val = strings.TrimSpace(typ) + " " + tok.AccessToken
		}
		req.Headers.Set(hdr, val)
		return explainAuthPreviewResult{
			status:  xplain.StageOK,
			summary: xplain.SummaryAuthPrepared,
			notes:   []string{"used cached OAuth token for explain preview"},
			extraSecrets: []string{
				tok.AccessToken,
				val,
			},
		}, nil
	default:
		return explainAuthPreviewResult{
			status:  xplain.StageSkipped,
			summary: xplain.SummaryAuthTypeNotApplied,
			notes:   []string{fmt.Sprintf("unsupported auth type %q is not applied", auth.Type)},
		}, nil
	}
}

func (e *Engine) prepareExplainHTTPPreview(
	ctx context.Context,
	rep *xplain.Report,
	req *restfile.Request,
	res *vars.Resolver,
	opts httpclient.Options,
) error {
	if rep == nil || req == nil || e.hc == nil {
		return nil
	}
	httpReq, _, body, err := e.hc.BuildHTTPRequest(ctx, req, res, opts)
	if err != nil {
		return err
	}
	if req.SSE != nil && httpReq.Header.Get("Accept") == "" {
		httpReq.Header.Set("Accept", "text/event-stream")
	}
	addExplainPreparedHTTPStage(rep, req, httpReq, body)
	setExplainHTTPPrepared(rep, req, httpReq, body)
	return nil
}

package ui

import (
	"fmt"
	"net/http"
	"net/textproto"
	"sort"
	"strings"

	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/k8s"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/ssh"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

const explainClip = 512

const (
	explainStageApply            = xplain.StageApply
	explainStageCondition        = xplain.StageCondition
	explainStageRoute            = xplain.StageRoute
	explainStageSettings         = xplain.StageSettings
	explainStageAuth             = xplain.StageAuth
	explainStageRTSPreRequest    = xplain.StageRTSPreRequest
	explainStageJSPreRequest     = xplain.StageJSPreRequest
	explainStageGRPCPrepare      = xplain.StageGRPCPrepare
	explainStageHTTPPrepare      = xplain.StageHTTPPrepare
	explainStageWebSocketPrepare = xplain.StageWebSocketPrepare
	explainStageCaptures         = xplain.StageCaptures
)

const (
	explainRouteKindDirect = xplain.RouteKindDirect
	explainRouteKindSSH    = xplain.RouteKindSSH
	explainRouteKindK8s    = xplain.RouteKindK8s
)

const (
	explainSummaryApplyComplete               = xplain.SummaryApplyComplete
	explainSummaryApplyFailed                 = xplain.SummaryApplyFailed
	explainSummaryConditionPassed             = xplain.SummaryConditionPassed
	explainSummaryConditionBlockedRequest     = xplain.SummaryConditionBlockedRequest
	explainSummaryConditionEvaluationFailed   = xplain.SummaryConditionEvaluationFailed
	explainSummaryRouteSSHResolutionFailed    = xplain.SummaryRouteSSHResolutionFailed
	explainSummaryRouteK8sResolutionFailed    = xplain.SummaryRouteK8sResolutionFailed
	explainSummaryRouteConfigInvalid          = xplain.SummaryRouteConfigInvalid
	explainSummarySettingsMerged              = xplain.SummarySettingsMerged
	explainSummarySettingsApplyFailed         = xplain.SummarySettingsApplyFailed
	explainSummaryAuthPrepared                = xplain.SummaryAuthPrepared
	explainSummaryAuthInjectionFailed         = xplain.SummaryAuthInjectionFailed
	explainSummaryOAuthTokenFetchSkipped      = xplain.SummaryOAuthTokenFetchSkipped
	explainSummaryCommandAuthExecutionSkipped = xplain.SummaryCommandAuthExecutionSkipped
	explainSummaryAuthTypeNotApplied          = xplain.SummaryAuthTypeNotApplied
	explainSummaryRTSPreRequestComplete       = xplain.SummaryRTSPreRequestComplete
	explainSummaryRTSPreRequestFailed         = xplain.SummaryRTSPreRequestFailed
	explainSummaryRTSPreRequestOutputBad      = xplain.SummaryRTSPreRequestOutputBad
	explainSummaryJSPreRequestComplete        = xplain.SummaryJSPreRequestComplete
	explainSummaryJSPreRequestFailed          = xplain.SummaryJSPreRequestFailed
	explainSummaryJSPreRequestOutputBad       = xplain.SummaryJSPreRequestOutputBad
	explainSummaryGRPCRequestPrepared         = xplain.SummaryGRPCRequestPrepared
	explainSummaryGRPCPrepareFailed           = xplain.SummaryGRPCPrepareFailed
	explainSummaryHTTPRequestPrepared         = xplain.SummaryHTTPRequestPrepared
	explainSummaryHTTPRequestBuildFailed      = xplain.SummaryHTTPRequestBuildFailed
	explainSummaryWebSocketRequestPrepared    = xplain.SummaryWebSocketRequestPrepared
	explainSummaryWebSocketPrepareFailed      = xplain.SummaryWebSocketPrepareFailed
	explainSummaryCaptureEvaluationFailed     = xplain.SummaryCaptureEvaluationFailed
)

func explainKey(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

var explainStageDisplayNames = map[string]string{
	explainKey(explainStageApply):            "Apply",
	explainKey(explainStageCondition):        "Condition",
	explainKey(explainStageRoute):            "Route",
	explainKey(explainStageSettings):         "Settings",
	explainKey(explainStageAuth):             "Authentication",
	explainKey(explainStageRTSPreRequest):    "RTS Pre-request",
	explainKey(explainStageJSPreRequest):     "JavaScript Pre-request",
	explainKey(explainStageGRPCPrepare):      "gRPC Request",
	explainKey(explainStageHTTPPrepare):      "HTTP Request",
	explainKey(explainStageWebSocketPrepare): "WebSocket Request",
	explainKey(explainStageCaptures):         "Captures",
}

var explainStageSummaryDisplay = map[string]map[string]string{
	explainKey(explainStageApply): {
		explainKey(explainSummaryApplyComplete): "Applied request mutations",
		explainKey(explainSummaryApplyFailed):   "Failed to apply request mutations",
	},
	explainKey(explainStageCondition): {
		explainKey(explainSummaryConditionPassed):           "Condition matched",
		explainKey(explainSummaryConditionBlockedRequest):   "Condition skipped this request",
		explainKey(explainSummaryConditionEvaluationFailed): "Failed to evaluate condition",
	},
	explainKey(explainStageRoute): {
		explainKey(explainRouteKindDirect):                 "Direct connection",
		explainKey(explainRouteKindSSH):                    "SSH route resolved",
		explainKey(explainRouteKindK8s):                    "Kubernetes route resolved",
		explainKey(explainSummaryRouteSSHResolutionFailed): "Failed to resolve SSH route",
		explainKey(explainSummaryRouteK8sResolutionFailed): "Failed to resolve Kubernetes route",
		explainKey(explainSummaryRouteConfigInvalid):       "Invalid route configuration",
	},
	explainKey(explainStageSettings): {
		explainKey(explainSummarySettingsMerged):      "Merged environment, file, and request settings",
		explainKey(explainSummarySettingsApplyFailed): "Failed to apply merged settings",
	},
	explainKey(explainStageAuth): {
		explainKey(explainSummaryAuthPrepared):                "Prepared authentication",
		explainKey(explainSummaryAuthInjectionFailed):         "Failed to prepare authentication",
		explainKey(explainSummaryOAuthTokenFetchSkipped):      "Skipped OAuth token fetch for explain preview",
		explainKey(explainSummaryCommandAuthExecutionSkipped): "Skipped command auth execution for explain preview",
		explainKey(explainSummaryAuthTypeNotApplied):          "Authentication type is not applied",
	},
	explainKey(explainStageRTSPreRequest): {
		explainKey(explainSummaryRTSPreRequestComplete):  "Applied RTS pre-request script",
		explainKey(explainSummaryRTSPreRequestFailed):    "RTS pre-request script failed",
		explainKey(explainSummaryRTSPreRequestOutputBad): "RTS pre-request script returned invalid output",
	},
	explainKey(explainStageJSPreRequest): {
		explainKey(explainSummaryJSPreRequestComplete):  "Applied JavaScript pre-request script",
		explainKey(explainSummaryJSPreRequestFailed):    "JavaScript pre-request script failed",
		explainKey(explainSummaryJSPreRequestOutputBad): "JavaScript pre-request script returned invalid output",
	},
	explainKey(explainStageGRPCPrepare): {
		explainKey(explainSummaryGRPCRequestPrepared): "Prepared gRPC request",
		explainKey(explainSummaryGRPCPrepareFailed):   "Failed to prepare gRPC request",
	},
	explainKey(explainStageHTTPPrepare): {
		explainKey(explainSummaryHTTPRequestPrepared):    "Prepared HTTP request",
		explainKey(explainSummaryHTTPRequestBuildFailed): "Failed to prepare HTTP request",
	},
	explainKey(explainStageWebSocketPrepare): {
		explainKey(explainSummaryWebSocketRequestPrepared): "Prepared WebSocket request",
		explainKey(explainSummaryWebSocketPrepareFailed):   "Failed to prepare WebSocket request",
	},
	explainKey(explainStageCaptures): {
		explainKey(explainSummaryCaptureEvaluationFailed): "Failed to evaluate captures",
	},
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

func appendExplainStage(rep *xplain.Report, stage xplain.Stage) {
	if rep == nil {
		return
	}
	if stage.Summary == "" {
		switch {
		case len(stage.Changes) > 0:
			stage.Summary = fmt.Sprintf("%d change(s)", len(stage.Changes))
		case len(stage.Notes) > 0:
			stage.Summary = "no request changes"
		default:
			stage.Summary = "no changes"
		}
	}
	rep.Stages = append(rep.Stages, stage)
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
		Name:    explainStageHTTPPrepare,
		Status:  xplain.StageOK,
		Summary: explainSummaryHTTPRequestPrepared,
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
	set map[string]string,
	sp *ssh.Plan,
	kp *k8s.Plan,
) {
	if rep == nil {
		return
	}
	final := &xplain.Final{
		Mode:     "prepared",
		Settings: explainSettings(set),
		Route:    explainRoute(sp, kp),
	}
	fillExplainFinal(final, req)
	rep.Final = final
}

func setExplainHTTP(
	rep *xplain.Report,
	resp *httpclient.Response,
) {
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

func renderExplainReport(rep *xplain.Report) string {
	if rep == nil {
		return ""
	}

	var b strings.Builder

	writeExplainSection(&b, "Summary")
	writeExplainKV(&b, "Result", explainResult(rep))
	writeExplainKV(&b, "Request", explainReqLabel(rep))
	writeExplainKV(&b, "Environment", rep.Env)
	writeExplainKV(&b, "Source", explainRequestLine(rep.Method, rep.URL))
	if rep.Final != nil {
		writeExplainKV(&b, "Final", explainRequestLine(rep.Final.Method, rep.Final.URL))
		writeExplainKV(&b, "Route", explainRouteLabel(rep.Final.Route))
	}
	writeExplainKV(&b, "Pipeline", explainStageCounts(rep.Stages))
	writeExplainKV(&b, "Variables", explainVarCounts(rep.Vars))
	if len(rep.Warnings) > 0 {
		writeExplainKV(&b, "Warnings", fmt.Sprintf("%d", len(rep.Warnings)))
	}

	if strings.TrimSpace(rep.Decision) != "" || strings.TrimSpace(rep.Failure) != "" {
		writeExplainSection(&b, "Decision")
		if strings.TrimSpace(rep.Decision) != "" {
			b.WriteString(rep.Decision)
			b.WriteString("\n")
		}
		if strings.TrimSpace(rep.Failure) != "" {
			b.WriteString("Failure: ")
			b.WriteString(rep.Failure)
			b.WriteString("\n")
		}
	}

	if rep.Final != nil {
		writeExplainSection(&b, "Final Request")
		if line := explainRequestLine(rep.Final.Method, rep.Final.URL); line != "" {
			b.WriteString(line)
			b.WriteString("\n")
		}
		writeExplainKV(&b, "Mode", rep.Final.Mode)
		writeExplainKV(&b, "Protocol", rep.Final.Protocol)
		if rep.Final.Route != nil {
			writeExplainKV(&b, "Route", explainRouteLabel(rep.Final.Route))
			writeExplainKV(&b, "Route Notes", strings.Join(rep.Final.Route.Notes, ", "))
		}
		writeExplainKV(&b, "Settings", explainPairsLabel(rep.Final.Settings))
		if len(rep.Final.Details) > 0 {
			b.WriteString("Details:\n")
			for _, d := range rep.Final.Details {
				if strings.TrimSpace(d.Key) == "" || strings.TrimSpace(d.Value) == "" {
					continue
				}
				b.WriteString("  ")
				b.WriteString(d.Key)
				b.WriteString(": ")
				b.WriteString(d.Value)
				b.WriteString("\n")
			}
		}
		if len(rep.Final.Headers) > 0 {
			b.WriteString("Headers:\n")
			for _, h := range rep.Final.Headers {
				b.WriteString("  ")
				b.WriteString(h.Name)
				b.WriteString(": ")
				b.WriteString(h.Value)
				b.WriteString("\n")
			}
		}
		if strings.TrimSpace(rep.Final.Body) != "" || strings.TrimSpace(rep.Final.BodyNote) != "" {
			if strings.TrimSpace(rep.Final.BodyNote) != "" {
				writeExplainKV(&b, "Body", rep.Final.BodyNote)
			} else {
				b.WriteString("Body:\n")
			}
			if strings.TrimSpace(rep.Final.Body) != "" {
				writeExplainBlock(&b, "  ", rep.Final.Body)
			}
		}
		if len(rep.Final.Steps) > 0 {
			b.WriteString("Steps:\n")
			for _, step := range rep.Final.Steps {
				step = strings.TrimSpace(step)
				if step == "" {
					continue
				}
				b.WriteString("  - ")
				b.WriteString(step)
				b.WriteString("\n")
			}
		}
	}

	if len(rep.Stages) > 0 {
		writeExplainSection(&b, "Stages")
		for i, st := range rep.Stages {
			b.WriteString(explainStageHeadline(i, st))
			b.WriteString("\n")
			for _, ch := range st.Changes {
				b.WriteString("   - ")
				b.WriteString(explainChangeLine(ch))
				b.WriteString("\n")
			}
			for _, note := range explainDisplayStageNotes(st) {
				note = strings.TrimSpace(note)
				if note == "" {
					continue
				}
				b.WriteString("   note: ")
				b.WriteString(note)
				b.WriteString("\n")
			}
		}
	}

	if len(rep.Vars) > 0 {
		writeExplainSection(&b, "Variables")
		for _, v := range rep.Vars {
			name := strings.TrimSpace(v.Name)
			if name == "" {
				continue
			}
			line := "- " + name + " <- "
			if v.Missing {
				line += "missing"
			} else {
				src := strings.TrimSpace(v.Source)
				if src == "" {
					src = "unknown"
				}
				line += src
				if v.Dynamic && !strings.EqualFold(src, "dynamic") {
					line += " dynamic"
				}
				if v.Uses > 1 {
					line += fmt.Sprintf(" x%d", v.Uses)
				}
			}
			b.WriteString(line)
			b.WriteString("\n")
			if !v.Missing && strings.TrimSpace(v.Value) != "" {
				b.WriteString("  value: ")
				b.WriteString(explainValue(v.Value))
				b.WriteString("\n")
			}
			if len(v.Shadowed) > 0 {
				b.WriteString("  shadowed: ")
				b.WriteString(strings.Join(v.Shadowed, ", "))
				b.WriteString("\n")
			}
		}
	}

	if len(rep.Warnings) > 0 {
		writeExplainSection(&b, "Warnings")
		for _, msg := range rep.Warnings {
			msg = strings.TrimSpace(msg)
			if msg == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(msg)
			b.WriteString("\n")
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

func writeExplainSection(b *strings.Builder, title string) {
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(strings.Repeat("=", len(title)))
	b.WriteString("\n")
}

func writeExplainKV(b *strings.Builder, key, val string) {
	key = strings.TrimSpace(key)
	val = strings.TrimSpace(val)
	if key == "" || val == "" {
		return
	}
	b.WriteString(key)
	b.WriteString(": ")
	b.WriteString(val)
	b.WriteString("\n")
}

func writeExplainBlock(b *strings.Builder, pad, text string) {
	text = strings.TrimRight(text, "\n")
	if strings.TrimSpace(text) == "" {
		return
	}
	for _, line := range strings.Split(text, "\n") {
		b.WriteString(pad)
		b.WriteString(line)
		b.WriteString("\n")
	}
}

func explainReqLabel(rep *xplain.Report) string {
	if rep == nil {
		return ""
	}
	if strings.TrimSpace(rep.Name) != "" {
		return rep.Name
	}
	return strings.TrimSpace(rep.Method + " " + rep.URL)
}

func explainResult(rep *xplain.Report) string {
	if rep == nil {
		return ""
	}
	switch rep.Status {
	case xplain.StatusReady:
		if rep.Final != nil && strings.EqualFold(strings.TrimSpace(rep.Final.Mode), "sent") {
			return "sent"
		}
		if rep.Final != nil {
			return "prepared"
		}
		return "ready"
	case xplain.StatusSkipped:
		return "skipped"
	case xplain.StatusError:
		return "error"
	default:
		return string(rep.Status)
	}
}

func explainRequestLine(method, url string) string {
	method = strings.TrimSpace(method)
	url = strings.TrimSpace(url)
	switch {
	case method == "" && url == "":
		return ""
	case method == "":
		return url
	case url == "":
		return method
	default:
		return method + " " + url
	}
}

func explainStageCounts(stages []xplain.Stage) string {
	if len(stages) == 0 {
		return ""
	}
	var okN, skipN, errN int
	for _, st := range stages {
		switch st.Status {
		case xplain.StageOK:
			okN++
		case xplain.StageSkipped:
			skipN++
		case xplain.StageError:
			errN++
		}
	}
	var parts []string
	if okN > 0 {
		parts = append(parts, fmt.Sprintf("%d ok", okN))
	}
	if skipN > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipN))
	}
	if errN > 0 {
		parts = append(parts, fmt.Sprintf("%d error", errN))
	}
	if len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%d stage(s)", len(stages)))
	}
	return strings.Join(parts, ", ")
}

func explainVarCounts(vs []xplain.Var) string {
	if len(vs) == 0 {
		return ""
	}
	var resolved, miss, dyn int
	for _, v := range vs {
		if v.Missing {
			miss++
			continue
		}
		resolved++
		if v.Dynamic {
			dyn++
		}
	}
	var parts []string
	if resolved > 0 {
		parts = append(parts, fmt.Sprintf("%d resolved", resolved))
	}
	if miss > 0 {
		parts = append(parts, fmt.Sprintf("%d missing", miss))
	}
	if dyn > 0 {
		parts = append(parts, fmt.Sprintf("%d dynamic", dyn))
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%d variable(s)", len(vs))
	}
	return strings.Join(parts, ", ")
}

func explainStageHeadline(i int, st xplain.Stage) string {
	name := explainDisplayStageName(st.Name)
	if name == "" {
		name = "Stage"
	}
	sum := explainDisplayStageSummary(st)
	if len(st.Changes) > 0 && !strings.Contains(strings.ToLower(sum), "change") {
		if sum == "" {
			sum = fmt.Sprintf("%d change(s)", len(st.Changes))
		} else {
			sum = fmt.Sprintf("%s (%d change(s))", sum, len(st.Changes))
		}
	}
	if sum == "" && len(explainDisplayStageNotes(st)) > 0 {
		sum = fmt.Sprintf("%d note(s)", len(st.Notes))
	}
	head := fmt.Sprintf("%s [%s]", name, string(st.Status))
	if sum != "" {
		head += ": " + sum
	}
	return head
}

func explainChangeLine(ch xplain.Change) string {
	field := explainChangeField(ch.Field)
	before := strings.TrimSpace(ch.Before)
	after := strings.TrimSpace(ch.After)
	switch {
	case before == "" && after != "":
		return fmt.Sprintf("set %s = %s", field, explainValue(after))
	case before != "" && after == "":
		return fmt.Sprintf("remove %s (was %s)", field, explainValue(before))
	default:
		return fmt.Sprintf("change %s: %s -> %s", field, explainValue(before), explainValue(after))
	}
}

func explainChangeField(field string) string {
	field = strings.TrimSpace(field)
	switch {
	case field == "body.note":
		return "body source"
	case field == "body":
		return "body"
	case field == "method":
		return "method"
	case field == "url":
		return "url"
	case field == "grpc.target":
		return "gRPC target"
	case field == "grpc.message":
		return "gRPC message"
	case strings.HasPrefix(field, "header."):
		return "header " + textproto.CanonicalMIMEHeaderKey(strings.TrimPrefix(field, "header."))
	case strings.HasPrefix(field, "setting."):
		return "setting " + strings.TrimPrefix(field, "setting.")
	case strings.HasPrefix(field, "var."):
		return "var " + strings.TrimPrefix(field, "var.")
	default:
		return field
	}
}

func explainValue(val string) string {
	val = strings.TrimSpace(val)
	if val == "" {
		return "<empty>"
	}
	return val
}

func explainRouteLabel(rt *xplain.Route) string {
	if rt == nil {
		return ""
	}
	kind := strings.TrimSpace(rt.Kind)
	sum := strings.TrimSpace(rt.Summary)
	switch {
	case kind == "" && sum == "":
		return ""
	case kind == "" || strings.EqualFold(kind, "direct"):
		if sum != "" {
			return sum
		}
		return kind
	case sum == "":
		return kind
	default:
		return kind + " via " + sum
	}
}

func explainPairsLabel(xs []xplain.Pair) string {
	if len(xs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(xs))
	for _, kv := range xs {
		key := strings.TrimSpace(kv.Key)
		if key == "" {
			continue
		}
		parts = append(parts, key+"="+strings.TrimSpace(kv.Value))
	}
	return strings.Join(parts, ", ")
}

func explainDisplayStageName(name string) string {
	if display, ok := explainStageDisplayNames[explainKey(name)]; ok {
		return display
	}
	return explainTitleWords(name)
}

func explainDisplayStageSummary(st xplain.Stage) string {
	sum := strings.TrimSpace(st.Summary)
	if displayBySummary, ok := explainStageSummaryDisplay[explainKey(st.Name)]; ok {
		if display, ok := displayBySummary[explainKey(sum)]; ok {
			return display
		}
	}
	return sum
}

func explainDisplayStageNotes(st xplain.Stage) []string {
	notes := append([]string(nil), st.Notes...)
	if len(notes) == 0 {
		return nil
	}
	if explainKey(st.Name) == explainStageRoute {
		sum := strings.TrimSpace(explainDisplayStageSummary(st))
		var out []string
		for _, note := range notes {
			note = strings.TrimSpace(note)
			if note == "" {
				continue
			}
			if strings.EqualFold(note, sum) {
				continue
			}
			out = append(out, note)
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}
	return notes
}

func explainTitleWords(s string) string {
	parts := strings.Fields(strings.TrimSpace(s))
	if len(parts) == 0 {
		return ""
	}
	for i, part := range parts {
		switch strings.ToLower(part) {
		case "rts":
			parts[i] = "RTS"
		case "js":
			parts[i] = "JavaScript"
		case "grpc":
			parts[i] = "gRPC"
		case "k8s":
			parts[i] = "Kubernetes"
		case "ssh":
			parts[i] = "SSH"
		default:
			if part == "" {
				continue
			}
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, " ")
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

func explainMapValue(values map[string]string, name string) string {
	want := normalizedExplainKey(name)
	for key, value := range values {
		if normalizedExplainKey(key) == want {
			return value
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
		for _, v := range h.Values(name) {
			out = append(out, xplain.Header{Name: name, Value: v})
		}
	}
	return out
}

func explainSettings(set map[string]string) []xplain.Pair {
	if len(set) == 0 {
		return nil
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]xplain.Pair, 0, len(keys))
	for _, k := range keys {
		out = append(out, xplain.Pair{Key: k, Value: set[k]})
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
	for _, step := range req.WebSocket.Steps {
		if line := explainWebSocketStep(step); line != "" {
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
	if authority := strings.TrimSpace(gr.Authority); authority != "" {
		out = append(out, xplain.Pair{Key: "Authority", Value: authority})
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
		out = append(out, xplain.Pair{
			Key:   "Max Message Bytes",
			Value: fmt.Sprintf("%d", opts.MaxMessageBytes),
		})
	}
	if len(opts.Subprotocols) > 0 {
		out = append(out, xplain.Pair{
			Key:   "Subprotocols",
			Value: strings.Join(opts.Subprotocols, ", "),
		})
	}
	if opts.CompressionSet {
		out = append(out, xplain.Pair{
			Key:   "Compression",
			Value: explainToggle(opts.Compression),
		})
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
		out = append(out, xplain.Pair{
			Key:   "Max Events",
			Value: fmt.Sprintf("%d", opts.MaxEvents),
		})
	}
	if opts.MaxBytes > 0 {
		out = append(out, xplain.Pair{
			Key:   "Max Bytes",
			Value: fmt.Sprintf("%d", opts.MaxBytes),
		})
	}
	return out
}

func explainToggle(on bool) string {
	if on {
		return "enabled"
	}
	return "disabled"
}

func explainWebSocketStep(step restfile.WebSocketStep) string {
	switch step.Type {
	case restfile.WebSocketStepSendText:
		if val := strings.TrimSpace(step.Value); val != "" {
			return "Send text " + clipExplain(val)
		}
	case restfile.WebSocketStepSendJSON:
		if val := strings.TrimSpace(step.Value); val != "" {
			return "Send JSON " + clipExplain(val)
		}
	case restfile.WebSocketStepSendBase64:
		if val := strings.TrimSpace(step.Value); val != "" {
			return "Send base64 " + clipExplain(val)
		}
	case restfile.WebSocketStepSendFile:
		if path := strings.TrimSpace(step.File); path != "" {
			return "Send file " + path
		}
	case restfile.WebSocketStepPing:
		if val := strings.TrimSpace(step.Value); val != "" {
			return "Ping " + clipExplain(val)
		}
		return "Ping"
	case restfile.WebSocketStepPong:
		if val := strings.TrimSpace(step.Value); val != "" {
			return "Pong " + clipExplain(val)
		}
		return "Pong"
	case restfile.WebSocketStepWait:
		if step.Duration > 0 {
			return "Wait " + step.Duration.String()
		}
		return "Wait"
	case restfile.WebSocketStepClose:
		switch {
		case step.Code > 0 && strings.TrimSpace(step.Reason) != "":
			return fmt.Sprintf("Close %d %s", step.Code, strings.TrimSpace(step.Reason))
		case step.Code > 0:
			return fmt.Sprintf("Close %d", step.Code)
		case strings.TrimSpace(step.Reason) != "":
			return "Close " + strings.TrimSpace(step.Reason)
		default:
			return "Close"
		}
	}
	return ""
}

func explainRoute(sp *ssh.Plan, kp *k8s.Plan) *xplain.Route {
	switch {
	case sp != nil && sp.Active():
		cfg := sp.Config
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
		return &xplain.Route{Kind: explainRouteKindSSH, Summary: sum, Notes: notes}
	case kp != nil && kp.Active():
		cfg := kp.Config
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
		return &xplain.Route{Kind: explainRouteKindK8s, Summary: sum, Notes: notes}
	default:
		return &xplain.Route{Kind: explainRouteKindDirect, Summary: "direct connection"}
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

func reqHeaders(req *restfile.Request) http.Header {
	if req == nil {
		return nil
	}
	return req.Headers
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
		if existing, ok := names[key]; ok {
			out[existing] = v.Value
			continue
		}
		names[key] = name
		out[name] = v.Value
	}
	return out
}

func headerValue(h http.Header, name string) string {
	if len(h) == 0 {
		return ""
	}
	for key, vals := range h {
		if !strings.EqualFold(strings.TrimSpace(key), name) {
			continue
		}
		return strings.Join(vals, ", ")
	}
	return ""
}

func clipExplain(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= explainClip {
		return s
	}
	return strings.TrimSpace(string(runes[:explainClip])) + " ..."
}

func (m *Model) redactExplainReportWithState(
	rep *xplain.Report,
	env string,
	req *restfile.Request,
	globals map[string]scripts.GlobalValue,
	extras ...string,
) *xplain.Report {
	if rep == nil {
		return nil
	}
	secrets := m.explainSecretsForRedaction(env, req, globals, extras)
	mask := maskSecret("", true)

	rep.Name = redactHistoryText(rep.Name, secrets, false)
	rep.Method = redactHistoryText(rep.Method, secrets, false)
	rep.URL = redactHistoryText(rep.URL, secrets, false)
	rep.Decision = redactHistoryText(rep.Decision, secrets, false)
	rep.Failure = redactHistoryText(rep.Failure, secrets, false)

	for i := range rep.Vars {
		rep.Vars[i].Value = redactHistoryText(rep.Vars[i].Value, secrets, false)
	}
	for i := range rep.Stages {
		rep.Stages[i].Summary = redactHistoryText(rep.Stages[i].Summary, secrets, false)
		for j := range rep.Stages[i].Changes {
			rep.Stages[i].Changes[j].Before = redactExplainChangeValue(
				rep.Stages[i].Changes[j].Field,
				rep.Stages[i].Changes[j].Before,
				secrets,
				mask,
			)
			rep.Stages[i].Changes[j].After = redactExplainChangeValue(
				rep.Stages[i].Changes[j].Field,
				rep.Stages[i].Changes[j].After,
				secrets,
				mask,
			)
		}
		for j := range rep.Stages[i].Notes {
			rep.Stages[i].Notes[j] = redactHistoryText(rep.Stages[i].Notes[j], secrets, false)
		}
	}
	if rep.Final != nil {
		rep.Final.Method = redactHistoryText(rep.Final.Method, secrets, false)
		rep.Final.URL = redactHistoryText(rep.Final.URL, secrets, false)
		rep.Final.Body = redactHistoryText(rep.Final.Body, secrets, false)
		rep.Final.BodyNote = redactHistoryText(rep.Final.BodyNote, secrets, false)
		for i := range rep.Final.Headers {
			if shouldMaskHistoryHeader(rep.Final.Headers[i].Name) {
				rep.Final.Headers[i].Value = mask
				continue
			}
			rep.Final.Headers[i].Value = redactHistoryText(
				rep.Final.Headers[i].Value,
				secrets,
				false,
			)
		}
		for i := range rep.Final.Settings {
			rep.Final.Settings[i].Value = redactHistoryText(
				rep.Final.Settings[i].Value,
				secrets,
				false,
			)
		}
		for i := range rep.Final.Details {
			rawKey := rep.Final.Details[i].Key
			rawValue := rep.Final.Details[i].Value
			rep.Final.Details[i].Key = redactHistoryText(rawKey, secrets, false)
			if shouldMaskExplainPair(rawKey, rawValue) {
				rep.Final.Details[i].Value = mask
				continue
			}
			rep.Final.Details[i].Value = redactHistoryText(rawValue, secrets, false)
		}
		for i := range rep.Final.Steps {
			rep.Final.Steps[i] = redactHistoryText(rep.Final.Steps[i], secrets, false)
		}
		if rep.Final.Route != nil {
			rep.Final.Route.Summary = redactHistoryText(rep.Final.Route.Summary, secrets, false)
			for i := range rep.Final.Route.Notes {
				rep.Final.Route.Notes[i] = redactHistoryText(
					rep.Final.Route.Notes[i],
					secrets,
					false,
				)
			}
		}
	}
	for i := range rep.Warnings {
		rep.Warnings[i] = redactHistoryText(rep.Warnings[i], secrets, false)
	}
	return rep
}

func (m *Model) explainSecretsForRedaction(
	env string,
	req *restfile.Request,
	globals map[string]scripts.GlobalValue,
	extras []string,
) []string {
	values := make(map[string]struct{})
	add := func(value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		values[value] = struct{}{}
	}

	for _, value := range m.secretValuesForEnvironment(env, req) {
		add(value)
	}
	for _, entry := range globals {
		if entry.Secret && !entry.Delete {
			add(entry.Value)
		}
	}
	for _, value := range extras {
		add(value)
	}
	if len(values) == 0 {
		return nil
	}

	secrets := make([]string, 0, len(values))
	for value := range values {
		secrets = append(secrets, value)
	}
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	return secrets
}

func redactExplainChangeValue(field, value string, secrets []string, mask string) string {
	if header, ok := explainHeaderField(field); ok && shouldMaskHistoryHeader(header) {
		if strings.TrimSpace(value) == "" {
			return value
		}
		return mask
	}
	return redactHistoryText(value, secrets, false)
}

func explainHeaderField(field string) (string, bool) {
	field = strings.TrimSpace(field)
	if !strings.HasPrefix(strings.ToLower(field), "header.") {
		return "", false
	}
	name := strings.TrimSpace(field[len("header."):])
	if name == "" {
		return "", false
	}
	return textproto.CanonicalMIMEHeaderKey(name), true
}

func shouldMaskExplainPair(key, value string) bool {
	if !strings.EqualFold(strings.TrimSpace(key), "Metadata") {
		return false
	}
	name, _, ok := strings.Cut(value, ":")
	if !ok {
		return false
	}
	return shouldMaskHistoryHeader(strings.TrimSpace(name))
}

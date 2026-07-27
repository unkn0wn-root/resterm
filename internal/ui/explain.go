package ui

import (
	"fmt"
	"net/http"
	"net/textproto"
	"sort"
	"strings"

	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/restfile"
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
	for line := range strings.SplitSeq(text, "\n") {
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

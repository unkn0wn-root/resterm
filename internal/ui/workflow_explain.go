package ui

import (
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/engine/core"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
)

func (state *workflowState) explainReport() *xplain.Report {
	if state == nil {
		return nil
	}
	title := strings.TrimSpace(state.runDisplayName())
	if title == "" {
		title = "Workflow"
	}
	entries := buildWorkflowStatsEntries(state)
	rep := &xplain.Report{
		Name:     title,
		URL:      title,
		Env:      state.explainEnv(entries),
		Status:   state.explainStatus(entries),
		Decision: state.summary(),
		Failure:  state.explainFailure(entries),
		Vars:     wfExplainVars(entries),
		Warnings: wfExplainWarnings(entries),
	}
	for _, entry := range entries {
		rep.Stages = append(rep.Stages, stepExplain(entry.result).stages()...)
	}
	return rep
}

func (state *workflowState) explainEnv(entries []workflowStatsEntry) string {
	for _, entry := range entries {
		rep := entry.result.Explain
		if rep == nil {
			continue
		}
		if env := strings.TrimSpace(rep.Env); env != "" {
			return env
		}
	}
	return ""
}

func (state *workflowState) explainStatus(entries []workflowStatsEntry) xplain.Status {
	if state != nil && state.canceled {
		return xplain.StatusError
	}
	if len(entries) == 0 {
		return xplain.StatusReady
	}
	allSkipped := true
	for _, entry := range entries {
		result := entry.result
		switch {
		case result.Canceled:
			return xplain.StatusError
		case result.Skipped:
			continue
		case !result.Success:
			return xplain.StatusError
		default:
			allSkipped = false
		}
	}
	if allSkipped {
		return xplain.StatusSkipped
	}
	return xplain.StatusReady
}

func (state *workflowState) explainFailure(entries []workflowStatsEntry) string {
	if state != nil && state.canceled {
		return strings.TrimSpace(state.cancelReason)
	}
	for i := len(entries) - 1; i >= 0; i-- {
		result := entries[i].result
		if result.Canceled {
			if msg := strings.TrimSpace(result.Message); msg != "" {
				return msg
			}
			continue
		}
		if result.Skipped || result.Success {
			continue
		}
		if rep := result.Explain; rep != nil {
			if failure := strings.TrimSpace(rep.Failure); failure != "" {
				return failure
			}
		}
		if result.ScriptErr != nil {
			return result.ScriptErr.Error()
		}
		if result.Err != nil {
			return result.Err.Error()
		}
		if msg := strings.TrimSpace(result.Message); msg != "" {
			return msg
		}
		if status := strings.TrimSpace(result.Status); status != "" {
			return status
		}
	}
	return ""
}

func wfExplainWarnings(entries []workflowStatsEntry) []string {
	var out []string
	for _, entry := range entries {
		rep := entry.result.Explain
		if rep == nil {
			continue
		}
		label := core.StepLabel(
			entry.result.Step,
			entry.result.Branch,
			entry.result.Iteration,
			entry.result.Total,
		)
		for _, warn := range rep.Warnings {
			warn = strings.TrimSpace(warn)
			if warn == "" {
				continue
			}
			if label != "" {
				warn = label + ": " + warn
			}
			out = appendExplainNote(out, warn)
		}
	}
	return out
}

// A variable can appear in every step report. Merge by name and source so the
// workflow report shows one row with the combined usage details.
func wfExplainVars(entries []workflowStatsEntry) []xplain.Var {
	var (
		out   []xplain.Var
		index = make(map[string]int)
	)
	for _, entry := range entries {
		rep := entry.result.Explain
		if rep == nil {
			continue
		}
		for _, v := range rep.Vars {
			key := normalizedExplainKey(v.Name) + "\x00" + normalizedExplainKey(v.Source)
			if idx, ok := index[key]; ok {
				curr := &out[idx]
				curr.Uses += v.Uses
				curr.Missing = curr.Missing || v.Missing
				curr.Dynamic = curr.Dynamic || v.Dynamic
				if strings.TrimSpace(curr.Value) == "" {
					curr.Value = v.Value
				}
				for _, shadowed := range v.Shadowed {
					if !slices.Contains(curr.Shadowed, shadowed) {
						curr.Shadowed = append(curr.Shadowed, shadowed)
					}
				}
				continue
			}
			copyVar := v
			copyVar.Shadowed = append([]string(nil), v.Shadowed...)
			out = append(out, copyVar)
			index[key] = len(out) - 1
		}
	}
	return out
}

type wfStepExplain struct {
	r   workflowStepResult
	lbl string
}

func stepExplain(r workflowStepResult) wfStepExplain {
	return wfStepExplain{r: r, lbl: core.StepLabel(r.Step, r.Branch, r.Iteration, r.Total)}
}

type wfExplainStage struct {
	key string
	st  xplain.Stage
}

func (x wfStepExplain) stages() []xplain.Stage {
	xs := x.cloneStages()
	xs = x.mergeDiffs(xs)
	if x.needOutcome(xs) {
		xs = append(xs, wfExplainStage{st: x.outcomeStage()})
	}
	out := make([]xplain.Stage, 0, len(xs))
	for _, s := range xs {
		out = append(out, s.st)
	}
	return out
}

func (x wfStepExplain) cloneStages() []wfExplainStage {
	rep := x.r.Explain
	if rep == nil || len(rep.Stages) == 0 {
		return nil
	}
	out := make([]wfExplainStage, 0, len(rep.Stages))
	for _, s := range rep.Stages {
		sum := explainDisplayStageSummary(s)
		ns := append([]string(nil), explainDisplayStageNotes(s)...)
		cs := append([]xplain.Change(nil), s.Changes...)
		out = append(out, wfExplainStage{
			key: explainKey(s.Name),
			st: xplain.Stage{
				Name:    x.stageName(s.Name),
				Status:  s.Status,
				Summary: sum,
				Changes: cs,
				Notes:   ns,
			},
		})
	}
	return out
}

func (x wfStepExplain) mergeDiffs(xs []wfExplainStage) []wfExplainStage {
	if x.r.Src == nil || x.r.Req == nil {
		return xs
	}
	// Source-to-executed changes are not always present in the step's own report.
	// Fold them into the closest stage so the workflow report does not lose them.
	cs := explainReqChanges(x.r.Src, x.r.Req)
	if len(cs) == 0 {
		return xs
	}
	sc, ac, pc := x.splitDiffs(cs)
	var pre []wfExplainStage
	xs, pre = x.mergeStage(
		xs,
		pre,
		explainStageSettings,
		wfExplainStageText(explainStageSettings, explainSummarySettingsMerged),
		sc,
	)
	xs, pre = x.mergeStage(
		xs,
		pre,
		explainStageAuth,
		wfExplainStageText(explainStageAuth, explainSummaryAuthPrepared),
		ac,
	)
	k := x.protoStageKey()
	xs, pre = x.mergeStage(xs, pre, k, wfExplainProtoStageSummary(k), pc)
	return append(pre, xs...)
}

func (x wfStepExplain) splitDiffs(cs []xplain.Change) (sc, ac, pc []xplain.Change) {
	for _, c := range cs {
		switch {
		case strings.HasPrefix(c.Field, "setting."):
			sc = append(sc, c)
		case x.isAuthChange(c):
			ac = append(ac, c)
		default:
			pc = append(pc, c)
		}
	}
	return sc, ac, pc
}

func (x wfStepExplain) isAuthChange(c xplain.Change) bool {
	if !strings.HasPrefix(c.Field, "header.") {
		return false
	}
	h := strings.TrimSpace(strings.TrimPrefix(c.Field, "header."))
	if strings.EqualFold(h, "authorization") {
		return true
	}
	req := x.r.Src
	if req == nil || req.Metadata.Auth == nil {
		return false
	}
	a := req.Metadata.Auth
	switch strings.ToLower(strings.TrimSpace(a.Type)) {
	case "header":
		return strings.EqualFold(h, strings.TrimSpace(a.Params["header"]))
	case "apikey", "api-key":
		if !strings.EqualFold(strings.TrimSpace(a.Params["placement"]), "header") {
			return false
		}
		n := strings.TrimSpace(a.Params["name"])
		if n == "" {
			n = "X-API-Key"
		}
		return strings.EqualFold(h, n)
	default:
		return false
	}
}

func (x wfStepExplain) mergeStage(
	xs []wfExplainStage,
	pre []wfExplainStage,
	key, sum string,
	cs []xplain.Change,
) ([]wfExplainStage, []wfExplainStage) {
	if len(cs) == 0 {
		return xs, pre
	}
	k := explainKey(key)
	for i := range xs {
		if xs[i].key != k {
			continue
		}
		xs[i].st.Changes = prependExplainChangesUnique(xs[i].st.Changes, cs)
		if strings.TrimSpace(xs[i].st.Summary) == "" {
			xs[i].st.Summary = sum
		}
		return xs, pre
	}
	pre = append(pre, wfExplainStage{
		key: k,
		st: xplain.Stage{
			Name:    x.stageName(key),
			Status:  xplain.StageOK,
			Summary: sum,
			Changes: append([]xplain.Change(nil), cs...),
		},
	})
	return xs, pre
}

func (x wfStepExplain) stageName(key string) string {
	name := strings.TrimSpace(explainDisplayStageName(key))
	if name == "" {
		name = strings.TrimSpace(key)
	}
	lbl := strings.TrimSpace(x.lbl)
	switch {
	case lbl == "":
		return name
	case name == "":
		return lbl
	default:
		return lbl + " / " + name
	}
}

func wfExplainStageText(key, sum string) string {
	st := xplain.Stage{Name: key, Summary: sum}
	txt := strings.TrimSpace(explainDisplayStageSummary(st))
	if txt != "" {
		return txt
	}
	return strings.TrimSpace(sum)
}

func (x wfStepExplain) protoStageKey() string {
	switch {
	case x.r.Req != nil && x.r.Req.GRPC != nil:
		return explainStageGRPCPrepare
	case x.r.Req != nil && x.r.Req.WebSocket != nil:
		return explainStageWebSocketPrepare
	default:
		return explainStageHTTPPrepare
	}
}

func wfExplainProtoStageSummary(key string) string {
	switch explainKey(key) {
	case explainKey(explainStageGRPCPrepare):
		return wfExplainStageText(explainStageGRPCPrepare, explainSummaryGRPCRequestPrepared)
	case explainKey(explainStageWebSocketPrepare):
		return wfExplainStageText(
			explainStageWebSocketPrepare,
			explainSummaryWebSocketRequestPrepared,
		)
	default:
		return wfExplainStageText(explainStageHTTPPrepare, explainSummaryHTTPRequestPrepared)
	}
}

func (x wfStepExplain) needOutcome(xs []wfExplainStage) bool {
	if len(xs) == 0 {
		return true
	}
	// Failed and skipped steps need a stage that carries their final status.
	// A successful step with existing stages does not need another OK row.
	want := x.outcomeStatus()
	for _, s := range xs {
		if s.st.Status == want {
			return false
		}
	}
	return want != xplain.StageOK
}

func (x wfStepExplain) outcomeStage() xplain.Stage {
	sum := x.outcome()
	return xplain.Stage{
		Name:    strings.TrimSpace(x.lbl),
		Status:  x.outcomeStatus(),
		Summary: sum,
		Notes:   x.outcomeNotes(sum),
	}
}

func (x wfStepExplain) outcomeStatus() xplain.StageStatus {
	switch {
	case x.r.Skipped:
		return xplain.StageSkipped
	case x.r.Canceled, !x.r.Success:
		return xplain.StageError
	default:
		return xplain.StageOK
	}
}

func (x wfStepExplain) outcome() string {
	switch {
	case x.r.Canceled:
		if msg := strings.TrimSpace(x.r.Message); msg != "" {
			return msg
		}
		return "canceled"
	case x.r.Skipped:
		if msg := strings.TrimSpace(x.r.Message); msg != "" {
			return msg
		}
		return "skipped"
	case !x.r.Success:
		if msg := strings.TrimSpace(x.r.Message); msg != "" {
			return msg
		}
		if status := strings.TrimSpace(x.r.Status); status != "" {
			return status
		}
		return "failed"
	default:
		if status := strings.TrimSpace(x.r.Status); status != "" {
			return status
		}
		if rep := x.r.Explain; rep != nil {
			if decision := strings.TrimSpace(rep.Decision); decision != "" {
				return decision
			}
		}
		return "completed"
	}
}

func (x wfStepExplain) outcomeNotes(sum string) []string {
	rep := x.r.Explain
	if rep == nil {
		return nil
	}
	var notes []string
	if decision := strings.TrimSpace(rep.Decision); decision != "" && decision != sum {
		notes = appendExplainNote(notes, decision)
	}
	if failure := strings.TrimSpace(rep.Failure); failure != "" {
		notes = appendExplainNote(notes, "Failure: "+failure)
	}
	for _, warn := range rep.Warnings {
		warn = strings.TrimSpace(warn)
		if warn == "" {
			continue
		}
		notes = appendExplainNote(notes, "Warning: "+warn)
	}
	return notes
}

func appendExplainNote(out []string, note string) []string {
	note = strings.TrimSpace(note)
	if note == "" {
		return out
	}
	if slices.Contains(out, note) {
		return out
	}
	return append(out, note)
}

func prependExplainChangesUnique(dst, src []xplain.Change) []xplain.Change {
	if len(src) == 0 {
		return dst
	}
	out := make([]xplain.Change, 0, len(src)+len(dst))
	out = append(out, src...)
	for _, d := range dst {
		if hasExplainChange(out, d) {
			continue
		}
		out = append(out, d)
	}
	return out
}

func hasExplainChange(xs []xplain.Change, want xplain.Change) bool {
	for _, x := range xs {
		if x.Field == want.Field && x.Before == want.Before && x.After == want.After {
			return true
		}
	}
	return false
}

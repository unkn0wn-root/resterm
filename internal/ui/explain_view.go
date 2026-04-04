package ui

import (
	"fmt"
	"strings"

	xplain "github.com/unkn0wn-root/resterm/internal/explain"
)

type explainView struct {
	Result   string
	Title    string
	Summary  []explainField
	Decision []string
	Final    *explainFinalView
	Stages   []explainStageView
	Vars     []explainVarView
	Warnings []string
}

type explainField struct {
	Label string
	Value string
}

type explainFinalView struct {
	Headline string
	Fields   []explainField
	Details  []explainField
	Headers  []explainField
	BodyNote string
	Body     string
	Steps    []string
}

type explainStageView struct {
	Index   int
	Name    string
	Status  xplain.StageStatus
	Summary string
	Changes []explainChangeView
	Notes   []string
}

type explainChangeKind int

const (
	explainChangeUpdate explainChangeKind = iota
	explainChangeAdd
	explainChangeRemove
)

type explainChangeView struct {
	Kind explainChangeKind
	Text string
}

type explainVarView struct {
	Name     string
	Source   string
	Value    string
	Shadowed string
	Uses     int
	Missing  bool
	Dynamic  bool
}

func (s *explainState) ensureView() explainView {
	if s == nil || s.report == nil {
		return explainView{}
	}
	if s.view == nil {
		v := buildExplainView(s.report)
		s.view = &v
	}
	return *s.view
}

func buildExplainView(rep *xplain.Report) explainView {
	v := explainView{
		Result: explainResult(rep),
		Title:  explainReqLabel(rep),
	}
	if rep == nil {
		return v
	}
	if v.Title == "" {
		v.Title = explainRequestLine(rep.Method, rep.URL)
	}

	appendExplainField(&v.Summary, "Environment", rep.Env)
	appendExplainField(&v.Summary, "Source", explainRequestLine(rep.Method, rep.URL))
	if rep.Final != nil {
		appendExplainField(&v.Summary, "Final", explainRequestLine(rep.Final.Method, rep.Final.URL))
		appendExplainField(&v.Summary, "Route", explainRouteLabel(rep.Final.Route))
	}
	appendExplainField(&v.Summary, "Pipeline", explainStageCounts(rep.Stages))
	appendExplainField(&v.Summary, "Variables", explainVarCounts(rep.Vars))
	if len(rep.Warnings) > 0 {
		appendExplainField(&v.Summary, "Warnings", fmt.Sprintf("%d", len(rep.Warnings)))
	}

	if decision := strings.TrimSpace(rep.Decision); decision != "" {
		v.Decision = append(v.Decision, decision)
	}
	if failure := strings.TrimSpace(rep.Failure); failure != "" {
		v.Decision = append(v.Decision, "Failure: "+failure)
	}

	if rep.Final != nil {
		fv := &explainFinalView{
			Headline: explainRequestLine(rep.Final.Method, rep.Final.URL),
			BodyNote: strings.TrimSpace(rep.Final.BodyNote),
			Body:     strings.TrimSpace(rep.Final.Body),
			Steps:    append([]string(nil), rep.Final.Steps...),
		}
		appendExplainField(&fv.Fields, "Mode", rep.Final.Mode)
		appendExplainField(&fv.Fields, "Protocol", rep.Final.Protocol)
		appendExplainField(&fv.Fields, "Route", explainRouteLabel(rep.Final.Route))
		if rep.Final.Route != nil {
			appendExplainField(&fv.Fields, "Route Notes", strings.Join(rep.Final.Route.Notes, ", "))
		}
		appendExplainField(&fv.Fields, "Settings", explainPairsLabel(rep.Final.Settings))
		for _, d := range rep.Final.Details {
			appendExplainField(&fv.Details, d.Key, d.Value)
		}
		for _, h := range rep.Final.Headers {
			appendExplainField(&fv.Headers, h.Name, h.Value)
		}
		v.Final = fv
	}

	for i, st := range rep.Stages {
		sv := explainStageView{
			Index:   i + 1,
			Name:    explainDisplayStageName(st.Name),
			Status:  st.Status,
			Summary: explainDisplayStageSummary(st),
			Notes:   explainDisplayStageNotes(st),
		}
		if sv.Name == "" {
			sv.Name = "Stage"
		}
		for _, ch := range st.Changes {
			text := strings.TrimSpace(explainChangeLine(ch))
			if text == "" {
				continue
			}
			sv.Changes = append(sv.Changes, explainChangeView{
				Kind: explainChangeKindOf(ch),
				Text: text,
			})
		}
		v.Stages = append(v.Stages, sv)
	}

	for _, it := range rep.Vars {
		name := strings.TrimSpace(it.Name)
		if name == "" {
			continue
		}
		vv := explainVarView{
			Name:     name,
			Source:   strings.TrimSpace(it.Source),
			Value:    strings.TrimSpace(it.Value),
			Shadowed: strings.Join(it.Shadowed, ", "),
			Uses:     it.Uses,
			Missing:  it.Missing,
			Dynamic:  it.Dynamic,
		}
		if vv.Source == "" && !vv.Missing {
			vv.Source = "unknown"
		}
		v.Vars = append(v.Vars, vv)
	}

	for _, msg := range rep.Warnings {
		msg = strings.TrimSpace(msg)
		if msg == "" {
			continue
		}
		v.Warnings = append(v.Warnings, msg)
	}

	return v
}

func appendExplainField(out *[]explainField, label, value string) {
	label = strings.TrimSpace(label)
	value = strings.TrimSpace(value)
	if label == "" || value == "" {
		return
	}
	*out = append(*out, explainField{Label: label, Value: value})
}

func explainChangeKindOf(ch xplain.Change) explainChangeKind {
	before := strings.TrimSpace(ch.Before)
	after := strings.TrimSpace(ch.After)
	switch {
	case before == "" && after != "":
		return explainChangeAdd
	case before != "" && after == "":
		return explainChangeRemove
	default:
		return explainChangeUpdate
	}
}

package rts

import "github.com/unkn0wn-root/resterm/internal/diag"

func (e *ParseError) Diagnostic() diag.Report {
	if e == nil {
		return diag.Report{}
	}
	return diag.Report{Items: []diag.Diagnostic{{
		Class:    diag.ClassScript,
		Severity: diag.SeverityError,
		Message:  e.Msg,
		Span:     diag.Span{Start: diagPos(e.Pos)},
	}}}
}

func (e *RuntimeError) Diagnostic() diag.Report {
	if e == nil {
		return diag.Report{}
	}
	return diag.Report{Items: []diag.Diagnostic{{
		Class:    diag.ClassScript,
		Severity: diag.SeverityError,
		Message:  e.Msg,
		Span:     diag.Span{Start: diagPos(e.Pos)},
	}}}
}

func (e *AbortError) Diagnostic() diag.Report {
	if e == nil || e.RuntimeError == nil {
		return diag.Report{}
	}
	rep := e.RuntimeError.Diagnostic()
	if len(rep.Items) > 0 {
		rep.Items[0].Class = abortDiagClass(e.Kind)
	}
	return rep
}

func (e *StackError) Diagnostic() diag.Report {
	if e == nil || e.Err == nil {
		return diag.Report{}
	}
	rep, ok := e.Err.(interface{ Diagnostic() diag.Report })
	if ok {
		out := rep.Diagnostic()
		if len(out.Items) > 0 {
			out.Items[0].Frames = append(out.Items[0].Frames, diagFrames(e.Frames)...)
			return out
		}
	}
	return diag.Report{Items: []diag.Diagnostic{{
		Class:    diag.ClassScript,
		Severity: diag.SeverityError,
		Message:  e.Err.Error(),
		Frames:   diagFrames(e.Frames),
	}}}
}

func diagPos(p Pos) diag.Pos {
	return diag.Pos{Path: p.Path, Line: p.Line, Col: p.Col}
}

func diagFrames(src []Frame) []diag.StackFrame {
	out := make([]diag.StackFrame, 0, len(src))
	for _, f := range src {
		name := f.Name
		if name == "" {
			name = "<fn>"
		}
		out = append(out, diag.StackFrame{Name: name, Pos: diagPos(f.Pos)})
	}
	return out
}

func abortDiagClass(kind AbortKind) diag.Class {
	switch kind {
	case AbortTimeout:
		return diag.ClassTimeout
	case AbortCanceled:
		return diag.ClassCanceled
	default:
		return diag.ClassScript
	}
}

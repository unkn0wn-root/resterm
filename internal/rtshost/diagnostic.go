package rtshost

import (
	"slices"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

// diagnosticError gives Resterm diagnostics to an RTS error without
// changing its message or hiding it from errors.Is and errors.As.
type diagnosticError struct {
	err    error
	report diag.Report
}

func (e *diagnosticError) Error() string           { return e.err.Error() }
func (e *diagnosticError) Unwrap() error           { return e.err }
func (e *diagnosticError) Diagnostic() diag.Report { return e.report }

// diagnose translates language errors at the Resterm host boundary.
func diagnose(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(diag.Reporter); ok {
		return err
	}
	report, ok := reportOf(err)
	if !ok {
		return err
	}
	return &diagnosticError{err: err, report: report}
}

func reportOf(err error) (diag.Report, bool) {
	switch e := err.(type) {
	case *rts.ParseError:
		return errorReport(diag.ClassScript, e.Msg, e.Pos), true
	case *rts.AbortError:
		return errorReport(abortClass(e.Kind), e.Msg, e.Pos), true
	case *rts.RuntimeError:
		return errorReport(diag.ClassScript, e.Msg, e.Pos), true
	case *rts.StackError:
		return stackReport(e), true
	default:
		return diag.Report{}, false
	}
}

func stackReport(e *rts.StackError) diag.Report {
	report := baseReport(e.Err)
	// Copy before appending: the reporter branch hands back a report the
	// failing error still owns.
	items := slices.Clone(report.Items)
	items[0].Frames = slices.Concat(items[0].Frames, diagFrames(e.Frames))
	report.Items = items
	return report
}

// baseReport is the non-empty report stack frames attach to: the RTS mapping
// when there is one, then any report the error already carries, then its text.
func baseReport(err error) diag.Report {
	if report, ok := reportOf(err); ok {
		return report
	}
	if reporter, ok := err.(diag.Reporter); ok {
		if report := reporter.Diagnostic(); len(report.Items) > 0 {
			return report
		}
	}
	return errorReport(diag.ClassScript, err.Error(), rts.Pos{})
}

func errorReport(class diag.Class, message string, pos rts.Pos) diag.Report {
	return diag.Report{Items: []diag.Diagnostic{{
		Class:    class,
		Severity: diag.SeverityError,
		Message:  message,
		Span:     diag.Span{Start: diagPos(pos)},
	}}}
}

func diagPos(pos rts.Pos) diag.Pos {
	return diag.Pos{Path: pos.Path, Line: pos.Line, Col: pos.Col}
}

func diagFrames(frames []rts.Frame) []diag.StackFrame {
	out := make([]diag.StackFrame, 0, len(frames))
	for _, frame := range frames {
		name := frame.Name
		if name == "" {
			name = "<fn>"
		}
		out = append(out, diag.StackFrame{
			Name: name,
			Pos:  diagPos(frame.Pos),
		})
	}
	return out
}

func abortClass(kind rts.AbortKind) diag.Class {
	switch kind {
	case rts.AbortTimeout:
		return diag.ClassTimeout
	case rts.AbortCanceled:
		return diag.ClassCanceled
	default:
		return diag.ClassScript
	}
}

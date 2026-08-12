package rtshost

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

func TestDiagnosePreservesRTSErrorAndReport(t *testing.T) {
	original := &rts.StackError{
		Err: &rts.RuntimeError{
			Pos: rts.Pos{Path: "hook.rts", Line: 3, Col: 7},
			Msg: "boom",
		},
		Frames: []rts.Frame{
			{
				Kind: rts.FrameFn,
				Pos:  rts.Pos{Path: "hook.rts", Line: 2, Col: 1},
				Name: "sign",
			},
			{
				Kind: rts.FrameFn,
				Pos:  rts.Pos{Path: "hook.rts", Line: 1, Col: 1},
			},
		},
	}

	err := diagnose(original)
	if err.Error() != original.Error() {
		t.Fatalf("diagnose(error).Error() = %q, want %q", err.Error(), original.Error())
	}
	if got := diagnose(err); got != err {
		t.Fatalf("diagnose(diagnose(error)) returned a new error")
	}
	var stackErr *rts.StackError
	if !errors.As(err, &stackErr) || stackErr != original {
		t.Fatalf("errors.As(error, *rts.StackError) = %v, want original", stackErr)
	}

	wrapped := diag.WrapAs(diag.ClassScript, err, "pre-request rts script")
	got := diag.Render(wrapped)
	for _, want := range []string{
		"error[script]: boom",
		"--> hook.rts:3:7",
		"pre-request rts script",
		"Stack:",
		"at hook.rts:2:1 in sign",
		"at hook.rts:1:1 in <fn>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Render() missing %q in %q", want, got)
		}
	}
}

func TestEngineDiagnosesCoreErrors(t *testing.T) {
	eng := NewEngine(func() map[string]rts.Value { return nil })
	pos := rts.Pos{Path: "boundary.rts", Line: 4, Col: 6}
	tests := []struct {
		name string
		eval func() error
	}{
		{
			name: "expression",
			eval: func() error {
				_, err := eng.Eval(context.Background(), Runtime{}, "(", pos)
				return err
			},
		},
		{
			name: "assertion",
			eval: func() error {
				_, err := eng.EvalAssertion(context.Background(), Runtime{}, "(", pos)
				return err
			},
		},
		{
			name: "string",
			eval: func() error {
				_, err := eng.EvalStr(context.Background(), Runtime{}, "(", pos)
				return err
			},
		},
		{
			name: "module",
			eval: func() error {
				_, err := eng.ExecModule(context.Background(), Runtime{}, "(", pos)
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.eval()
			if err == nil {
				t.Fatal("evaluation succeeded, want parse error")
			}
			var parseErr *rts.ParseError
			if !errors.As(err, &parseErr) {
				t.Fatalf("errors.As(error, *rts.ParseError) failed for %T", err)
			}
			report := diag.ReportOf(err)
			if len(report.Items) != 1 {
				t.Fatalf("ReportOf(error) has %d items, want 1", len(report.Items))
			}
			item := report.Items[0]
			if item.Class != diag.ClassScript || item.Span.Start.Path != pos.Path {
				t.Fatalf("ReportOf(error) = %+v, want script diagnostic in %q", item, pos.Path)
			}
		})
	}
}

func TestDiagnoseAbortClasses(t *testing.T) {
	tests := []struct {
		name string
		kind rts.AbortKind
		want diag.Class
	}{
		{name: "script", kind: rts.AbortScript, want: diag.ClassScript},
		{name: "timeout", kind: rts.AbortTimeout, want: diag.ClassTimeout},
		{name: "canceled", kind: rts.AbortCanceled, want: diag.ClassCanceled},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := diagnose(&rts.AbortError{
				RuntimeError: &rts.RuntimeError{
					Pos: rts.Pos{Path: "hook.rts", Line: 2, Col: 3},
					Msg: "stopped",
				},
				Kind: test.kind,
			})
			if got := diag.ClassOf(err); got != test.want {
				t.Fatalf("ClassOf(error) = %q, want %q", got, test.want)
			}
		})
	}
}

type reportedError struct {
	msg    string
	report diag.Report
}

func (e *reportedError) Error() string           { return e.msg }
func (e *reportedError) Diagnostic() diag.Report { return e.report }

func TestDiagnoseStackErrorKeepsInnerReport(t *testing.T) {
	inner := &reportedError{
		msg: "inner failed",
		report: diag.Report{Items: []diag.Diagnostic{{
			Class:    diag.ClassAuth,
			Severity: diag.SeverityError,
			Message:  "inner failed",
			Frames:   []diag.StackFrame{{Name: "inner", Pos: diag.Pos{Path: "inner.rts", Line: 9}}},
		}}},
	}

	err := diagnose(&rts.StackError{
		Err:    inner,
		Frames: []rts.Frame{{Pos: rts.Pos{Path: "hook.rts", Line: 2, Col: 1}, Name: "sign"}},
	})

	report := diag.ReportOf(err)
	if len(report.Items) != 1 {
		t.Fatalf("ReportOf(error) has %d items, want 1", len(report.Items))
	}
	item := report.Items[0]
	if item.Class != diag.ClassAuth || item.Message != "inner failed" {
		t.Fatalf("ReportOf(error) = %+v, want the inner report", item)
	}
	want := []diag.StackFrame{
		{Name: "inner", Pos: diag.Pos{Path: "inner.rts", Line: 9}},
		{Name: "sign", Pos: diag.Pos{Path: "hook.rts", Line: 2, Col: 1}},
	}
	if !slices.Equal(item.Frames, want) {
		t.Fatalf("frames = %+v, want %+v", item.Frames, want)
	}
	if len(inner.report.Items[0].Frames) != 1 {
		t.Fatalf("inner report frames = %+v, want the original single frame", inner.report.Items[0].Frames)
	}
}

func TestDiagnoseStackErrorFallsBackToMessage(t *testing.T) {
	err := diagnose(&rts.StackError{
		Err:    errors.New("boom"),
		Frames: []rts.Frame{{Pos: rts.Pos{Path: "hook.rts", Line: 5, Col: 2}}},
	})

	report := diag.ReportOf(err)
	if len(report.Items) != 1 {
		t.Fatalf("ReportOf(error) has %d items, want 1", len(report.Items))
	}
	item := report.Items[0]
	if item.Class != diag.ClassScript || item.Message != "boom" {
		t.Fatalf("ReportOf(error) = %+v, want a script diagnostic for %q", item, "boom")
	}
	want := []diag.StackFrame{{Name: "<fn>", Pos: diag.Pos{Path: "hook.rts", Line: 5, Col: 2}}}
	if !slices.Equal(item.Frames, want) {
		t.Fatalf("frames = %+v, want %+v", item.Frames, want)
	}
}

func TestDiagnoseLeavesNonRTSErrorUnchanged(t *testing.T) {
	original := errors.New("boom")
	if got := diagnose(original); got != original {
		t.Fatalf("diagnose(error) = %T %v, want original", got, got)
	}
}

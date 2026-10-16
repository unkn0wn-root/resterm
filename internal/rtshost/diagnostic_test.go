package rtshost

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
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

	eng := NewEngine(nil)
	err := eng.diagnose(original)
	if err.Error() != original.Error() {
		t.Fatalf("diagnose(error).Error() = %q, want %q", err.Error(), original.Error())
	}
	if got := eng.diagnose(err); got != err {
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

func TestDiagnoseUndefinedName(t *testing.T) {
	err := NewEngine(nil).diagnose(&rts.StackError{Err: &rts.UndefinedNameError{
		RuntimeError: &rts.RuntimeError{Pos: rts.Pos{Path: "wf.http", Line: 2, Col: 1}, Msg: `undefined name "user"`},
		Name:         "user",
	}})
	got := diag.Render(err)
	for _, want := range []string{`error[script]: undefined name "user"`, "--> wf.http:2:1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Render() missing %q in %q", want, got)
		}
	}
	var undef *rts.UndefinedNameError
	if !errors.As(err, &undef) || undef.Name != "user" {
		t.Fatalf("errors.As(%v) found no undefined name user", err)
	}
}

func TestDiagnoseWrappedAbort(t *testing.T) {
	abort := &rts.AbortError{
		RuntimeError: &rts.RuntimeError{Pos: rts.Pos{Path: "hook.rts", Line: 3, Col: 7}, Msg: "timeout exceeded"},
		Kind:         rts.AbortTimeout,
	}
	for name, err := range map[string]error{
		"bare":       fmt.Errorf("native: %w", abort),
		"with stack": fmt.Errorf("native: %w", &rts.StackError{Err: abort}),
	} {
		rep := diag.ReportOf(NewEngine(nil).diagnose(err))
		if len(rep.Items) == 0 || rep.Class() != diag.ClassTimeout {
			t.Fatalf("%s: report = %+v, want a timeout", name, rep)
		}
		if got := rep.Items[0].Span.Start; got.Path != "hook.rts" || got.Line != 3 || got.Col != 7 {
			t.Fatalf("%s: position = %+v, want hook.rts:3:7", name, got)
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
			err := NewEngine(nil).diagnose(&rts.AbortError{
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

	err := NewEngine(nil).diagnose(&rts.StackError{
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
	err := NewEngine(nil).diagnose(&rts.StackError{
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
	if got := NewEngine(nil).diagnose(original); got != original {
		t.Fatalf("diagnose(error) = %T %v, want original", got, got)
	}
}

func TestErrorsShowTheLineOfTheFileTheyPointAt(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "m.rts", "module m\n\nexport fn boom(x) {\n  return x.name\n}\n")
	writeModule(t, dir, "a.rts", "module a\n\nexport fn call(f) {\n  return f(null)\n}\n")
	writeModule(t, dir, "setup.rts", "module setup\n\nlet cfg = null\nlet v = cfg.name\n")
	writeModule(t, dir, "bad.rts", "module bad\n\nexport fn f() {\n  return 1 +\n}\n")
	writeModule(t, dir, "late.rts", "let x = 1\nmodule late\n")
	writeModule(t, dir, "x.rts", "let a = 1\nlet b = null\nb.name\n")
	writeModule(t, dir, "empty.rts", "")
	doc := &restfile.Document{
		Path: testDoc,
		Raw: []byte(
			"### One\n# @use ./m.rts\n# @rts pre-request\n> let y = null\n> y.name\nGET https://example.test\n",
		),
	}
	eng := NewEngine(nil)
	pre := func(rt Runtime, b restfile.ScriptBlock) error {
		b.Kind = restfile.ScriptKindPreRequest
		return RunPreRequest(t.Context(), eng, PreRequest{
			Doc:     doc,
			Scripts: []restfile.ScriptBlock{b},
			BaseDir: dir,
			Runtime: func() (Runtime, error) { return rt, nil },
		})
	}
	file := func(rt Runtime, b restfile.ScriptBlock) error {
		b.Body, b.FilePath = "", "x.rts"
		return pre(rt, b)
	}
	test := func(rt Runtime, b restfile.ScriptBlock) error {
		b.Kind = restfile.ScriptKindTest
		_, err := RunTests(t.Context(), eng, Tests{Doc: doc, Scripts: []restfile.ScriptBlock{b}, Runtime: rt})
		return err
	}
	assert := func(rt Runtime, b restfile.ScriptBlock) error {
		_, err := eng.EvalAssertion(t.Context(), rt, b.Body, rts.Pos{Path: testDoc, Line: 4, Col: 3})
		return err
	}
	moduleLine := "   4 |   return x.name\n     |           ^"
	blockLine := "   5 | > y.name\n     |    ^"
	tests := []struct {
		name string
		run  func(Runtime, restfile.ScriptBlock) error
		uses []string
		body string
		want string
	}{
		{"module function", pre, []string{"m.rts"}, "m.boom(null)", moduleLine},
		{"function passed to another module", pre, []string{"a.rts", "m.rts"}, "a.call(m.boom)", moduleLine},
		{"module top level", pre, []string{"setup.rts"}, "1", "   4 | let v = cfg.name\n     |            ^"},
		{"module parse error", pre, []string{"bad.rts"}, "1", "   5 | }\n     | ^"},
		{"module header", pre, []string{"late.rts"}, "1", "   2 | module late\n     | ^"},
		{"empty module", pre, []string{"empty.rts"}, "1", ""},
		{"request block", pre, []string{"m.rts"}, "let y = null\ny.name", blockLine},
		{"block read from a file", file, nil, "", "   3 | b.name\n     |  ^"},
		{"test block calling a module", test, []string{"m.rts"}, "m.boom(null)", moduleLine},
		{"test block", test, []string{"m.rts"}, "let y = null\ny.name", blockLine},
		{"assertion calling a module", assert, []string{"m.rts"}, "m.boom(null) == 1", moduleLine},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := Runtime{BaseDir: dir}
			for _, path := range tt.uses {
				rt.Uses = append(rt.Uses, rts.Use{Path: path})
			}
			err := tt.run(rt, restfile.ScriptBlock{
				Lang:       restfile.ScriptLangRTS,
				Body:       tt.body,
				SourcePath: testDoc,
				Lines:      []restfile.ScriptLine{{Line: 4, Col: 3}, {Line: 5, Col: 3}},
			})
			if err == nil {
				t.Fatal("run succeeded, want an error")
			}
			var excerpt []string
			for _, l := range diag.Lines(diag.ReportOf(err)) {
				if l.Kind == diag.LineSrc || l.Kind == diag.LineMark {
					excerpt = append(excerpt, l.Text)
				}
			}
			if got := strings.Join(excerpt, "\n"); got != tt.want {
				t.Fatalf("excerpt =\n%s\nwant\n%s\nin:\n%s", got, tt.want, diag.Render(err))
			}
		})
	}
}

package rtshost

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rts/native"
	"github.com/unkn0wn-root/resterm/internal/scriptapi"
)

type Tests struct {
	Doc     *restfile.Document
	Scripts []restfile.ScriptBlock
	// Leave Mutator nil so tests cannot change request state.
	Runtime Runtime
	// UsesFailed fails every block without running it. Set it when the
	// caller could not load Runtime.Uses and has reported why.
	UsesFailed bool
}

const testSteps = 100_000

func RunTests(ctx context.Context, eng *Engine, in Tests) ([]scriptapi.TestResult, error) {
	var (
		out  []scriptapi.TestResult
		errs []error
		cut  bool
	)
	nth := 0
	tag := directive.RTS.Tag() + " " + string(restfile.ScriptKindTest)
	for _, block := range in.Scripts {
		if !block.Is(restfile.ScriptKindTest, restfile.ScriptLangRTS) {
			continue
		}
		if ctx.Err() != nil {
			if !cut {
				errs = append(errs, context.Cause(ctx))
			}
			break
		}
		nth++
		site, name := tag, block.Name
		if name == "" {
			name = site
		} else {
			site += " " + directive.Quote(name)
		}
		start := time.Now()
		var msgs []string
		src, err := loadSource(in.Doc, block, in.Runtime.BaseDir)
		switch {
		case err != nil:
			err = diag.WrapAsf(diag.ClassScript, err, "rts test script %d", nth)
		case strings.TrimSpace(src.Text) == "":
			continue
		case in.UsesFailed:
		default:
			rt := in.Runtime
			rt.Site = site
			ctx, st := withRuntime(ctx, rt)
			t := newTestObj(st)
			cfg := evalConfig(st)
			cfg.MaxSteps = testSteps
			cfg.Overrides = rts.Local("t", rts.Obj(t))
			_, err = eng.core.ExecModule(ctx, cfg, src.Text, src.Pos)
			st.end()
			msgs = t.msgs
			var abort *rts.AbortError
			cut = errors.As(err, &abort) && (abort.Kind == rts.AbortCanceled || abort.Kind == rts.AbortTimeout)
			if errors.Is(err, rts.ErrHalt) {
				err = nil
			}
			err = src.annotate(eng.diagnose(err))
		}
		out = append(out, scriptapi.TestResult{
			Name:    name,
			Passed:  err == nil && len(msgs) == 0 && !in.UsesFailed,
			Message: strings.Join(msgs, "\n"),
			Elapsed: time.Since(start),
		})
		if err != nil {
			errs = append(errs, err)
		}
	}
	return out, errors.Join(errs...)
}

type testObj struct {
	evalObj
	msgs    []string
	members map[string]rts.Value
}

func newTestObj(st *evalState) *testObj {
	o := &testObj{evalObj: evalObj{st, "t"}}
	o.members = map[string]rts.Value{
		"error": o.method(o.reportDef("t.error", nil)),
		"fatal": o.method(o.reportDef("t.fatal", rts.ErrHalt)),
	}
	return o
}

func (o *testObj) Member(_ *rts.Ctx, _ rts.Pos, name string) (rts.Value, bool, error) {
	v, ok := o.members[name]
	return v, ok, nil
}

func (*testObj) Index(*rts.Ctx, rts.Pos, rts.Value) (rts.Value, error) {
	return rts.Null(), nil
}

func (o *testObj) reportDef(name string, end error) native.Def {
	return native.Raw(name, func(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
		parts := make([]string, len(args))
		for i, arg := range args {
			s, err := rts.ToStr(ctx, pos, arg)
			if err != nil {
				return rts.Null(), err
			}
			parts[i] = s
		}
		text := strings.Join(parts, " ")
		if err := rts.CheckStr(ctx, pos, text); err != nil {
			return rts.Null(), err
		}
		if err := rts.CheckList(ctx, pos, len(o.msgs)+1); err != nil {
			return rts.Null(), err
		}
		msg := fmt.Sprintf("line %d", pos.Line)
		if pos.Path != "" {
			msg = fmt.Sprintf("%s:%d", filepath.Base(pos.Path), pos.Line)
		}
		if text != "" {
			msg += ": " + text
		}
		o.msgs = append(o.msgs, msg)
		return rts.Null(), end
	})
}

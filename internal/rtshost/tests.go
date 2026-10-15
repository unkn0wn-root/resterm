package rtshost

import (
	"context"
	"errors"
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
}

func RunTests(ctx context.Context, eng *Engine, in Tests) ([]scriptapi.TestResult, error) {
	var (
		out  []scriptapi.TestResult
		errs []error
	)
	nth := 0
	tag := directive.RTS.Tag() + " " + string(restfile.ScriptKindTest)
	for _, block := range in.Scripts {
		if !block.Is(restfile.ScriptKindTest, restfile.ScriptLangRTS) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return out, errors.Join(append(errs, err)...)
		}
		nth++
		site, name := tag, block.Name
		if name == "" {
			name = site
		} else {
			site += " " + directive.Quote(name)
		}
		start := time.Now()
		t := newTestObj()
		src, err := loadSource(in.Doc, block, in.Runtime.BaseDir)
		switch {
		case err != nil:
			err = diag.WrapAsf(diag.ClassScript, err, "rts test script %d", nth)
		case strings.TrimSpace(src.Text) == "":
			continue
		default:
			rt := in.Runtime
			rt.Site = site
			cfg := blockConfig(rt)
			cfg.Overrides = rts.Local("t", rts.Obj(t))
			_, err = eng.core.ExecModule(withRuntime(ctx, rt), cfg, src.Text, src.Pos)
			if errors.Is(err, rts.ErrHalt) {
				err = nil
			}
			err = src.annotate(err)
		}
		t.done = true
		out = append(out, scriptapi.TestResult{
			Name:    name,
			Passed:  err == nil && len(t.msgs) == 0,
			Message: strings.Join(t.msgs, "\n"),
			Elapsed: time.Since(start),
		})
		if err != nil {
			errs = append(errs, err)
		}
	}
	return out, errors.Join(errs...)
}

type testObj struct {
	msgs    []string
	done    bool
	members map[string]rts.Value
}

func newTestObj() *testObj {
	o := &testObj{}
	o.members = map[string]rts.Value{
		"error": o.reportDef("t.error", nil).Value(),
		"fatal": o.reportDef("t.fatal", rts.ErrHalt).Value(),
	}
	return o
}

func (*testObj) TypeName() string { return "t" }

const testEnded = "t belongs to a test block that has ended"

func (o *testObj) Unbound() string {
	if o.done {
		return testEnded
	}
	return ""
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
		// A saved t.error or t.fatal bypasses Unbound.
		if o.done {
			return rts.Null(), rts.Abortf(ctx, pos, testEnded)
		}
		parts := make([]string, len(args))
		for i, arg := range args {
			s, err := rts.ToStr(ctx, pos, arg)
			if err != nil {
				return rts.Null(), err
			}
			parts[i] = s
		}
		at := diag.Pos{Line: pos.Line}
		if pos.Path != "" {
			at.Path = filepath.Base(pos.Path)
		}
		msg := at.String()
		if text := strings.Join(parts, " "); text != "" {
			msg += ": " + text
		}
		o.msgs = append(o.msgs, msg)
		return rts.Null(), end
	})
}

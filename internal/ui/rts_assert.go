package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

func rtsStream(info *scripts.StreamInfo) *rts.Stream {
	if info == nil {
		return nil
	}
	sum := make(map[string]any, len(info.Summary))
	for k, v := range info.Summary {
		sum[k] = v
	}
	ev := make([]map[string]any, len(info.Events))
	for i, item := range info.Events {
		if item == nil {
			continue
		}
		cp := make(map[string]any, len(item))
		for k, v := range item {
			cp[k] = v
		}
		ev[i] = cp
	}
	return &rts.Stream{Kind: info.Kind, Summary: sum, Events: ev}
}

func (m *Model) assertPos(doc *restfile.Document, req *restfile.Request, line int) rts.Pos {
	path := m.documentRuntimePath(doc)
	if strings.TrimSpace(path) == "" {
		path = m.currentFile
	}
	if line <= 0 && req != nil && req.LineRange.Start > 0 {
		line = req.LineRange.Start
	}
	if line <= 0 {
		line = 1
	}
	return rts.Pos{Path: path, Line: line, Col: 1}
}

func (m *Model) runAsserts(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	envName, base string,
	vars map[string]string,
	extraVals map[string]rts.Value,
	resp *rts.Resp,
	trace *rts.Trace,
	stream *rts.Stream,
) ([]scripts.TestResult, error) {
	if req == nil || len(req.Metadata.Asserts) == 0 {
		return nil, nil
	}
	if m.rtsEng == nil {
		m.rtsEng = rts.NewEng()
	}
	base = m.rtsBase(doc, base)
	if vars == nil {
		vars = m.rtsVars(doc, req, envName)
	}

	extra := make(map[string]rts.Value)
	for k, v := range extraVals {
		if k == "" {
			continue
		}
		extra[k] = v
	}
	for k, v := range rts.AssertExtra(resp) {
		extra[k] = v
	}

	rt := rts.RT{
		Env:         m.rtsEnv(envName),
		Vars:        vars,
		Resp:        resp,
		Res:         resp,
		Trace:       trace,
		Stream:      stream,
		Req:         m.rtsReq(req),
		BaseDir:     base,
		ReadFile:    os.ReadFile,
		AllowRandom: true,
		Uses:        m.rtsUses(doc, req),
		Extra:       extra,
	}

	results := make([]scripts.TestResult, 0, len(req.Metadata.Asserts))
	for _, as := range req.Metadata.Asserts {
		expr := strings.TrimSpace(as.Expression)
		if expr == "" {
			continue
		}
		rt.Site = "@assert " + expr
		start := time.Now()
		val, err := m.rtsEng.Eval(ctx, rt, expr, m.assertPos(doc, req, as.Line))
		if err != nil {
			return results, err
		}
		msg := strings.TrimSpace(as.Message)
		results = append(results, scripts.TestResult{
			Name:    expr,
			Message: msg,
			Passed:  val.IsTruthy(),
			Elapsed: time.Since(start),
		})
	}

	if len(results) == 0 {
		return nil, nil
	}
	return results, nil
}

func mergeErr(a, b error) error {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return fmt.Errorf("%v; %v", a, b)
}

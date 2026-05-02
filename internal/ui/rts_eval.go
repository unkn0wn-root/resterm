package ui

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rts/stdlib"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (m *Model) rtsPos(doc *restfile.Document, req *restfile.Request) vars.ExprPos {
	path := m.documentRuntimePath(doc)
	line := 1
	if req != nil && req.LineRange.Start > 0 {
		line = req.LineRange.Start
	}
	if strings.TrimSpace(path) == "" {
		path = m.currentFile
	}
	return vars.ExprPos{Path: path, Line: line, Col: 1}
}

func (m *Model) rtsPosForLine(doc *restfile.Document, req *restfile.Request, line int) rts.Pos {
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

func (m *Model) rtsBase(doc *restfile.Document, base string) string {
	if strings.TrimSpace(base) != "" {
		return base
	}
	path := m.documentRuntimePath(doc)
	if strings.TrimSpace(path) != "" {
		return filepath.Dir(path)
	}
	if strings.TrimSpace(m.currentFile) != "" {
		return filepath.Dir(m.currentFile)
	}
	return ""
}

func (m *Model) rtsEnv(envName string) map[string]string {
	res := make(map[string]string)
	if env := vars.EnvValues(m.cfg.EnvironmentSet, envName); len(env) > 0 {
		for k, v := range env {
			res[k] = v
		}
	}
	if strings.TrimSpace(envName) != "" {
		res["name"] = envName
	}
	return res
}

func (m *Model) rtsReq(req *restfile.Request) *rts.Req {
	if req == nil {
		return nil
	}
	out := &rts.Req{
		Method: strings.TrimSpace(req.Method),
		URL:    strings.TrimSpace(req.URL),
	}
	if len(req.Headers) > 0 {
		h := make(map[string][]string, len(req.Headers))
		for k, v := range req.Headers {
			if len(v) == 0 {
				continue
			}
			key := strings.ToLower(k)
			h[key] = append([]string(nil), v...)
		}
		if len(h) > 0 {
			out.H = h
		}
	}
	if q := requestQuery(out.URL); len(q) > 0 {
		out.Q = q
	}
	return out
}

func requestQuery(raw string) map[string][]string {
	if raw == "" {
		return nil
	}
	idx := strings.Index(raw, "?")
	if idx < 0 {
		return nil
	}
	q := raw[idx+1:]
	if cut := strings.Index(q, "#"); cut >= 0 {
		q = q[:cut]
	}
	if strings.TrimSpace(q) == "" {
		return nil
	}
	vals, err := url.ParseQuery(q)
	if err != nil || len(vals) == 0 {
		return nil
	}
	out := make(map[string][]string, len(vals))
	for k, v := range vals {
		if len(v) == 0 {
			continue
		}
		out[k] = append([]string(nil), v...)
	}
	return out
}

func (m *Model) rtsUses(doc *restfile.Document, req *restfile.Request) []rts.Use {
	var uses []rts.Use
	if doc != nil {
		for _, spec := range doc.Uses {
			path := strings.TrimSpace(spec.Path)
			alias := strings.TrimSpace(spec.Alias)
			if path == "" {
				continue
			}
			uses = append(uses, rts.Use{Path: path, Alias: alias})
		}
	}
	if req != nil {
		for _, spec := range req.Metadata.Uses {
			path := strings.TrimSpace(spec.Path)
			alias := strings.TrimSpace(spec.Alias)
			if path == "" {
				continue
			}
			uses = append(uses, rts.Use{Path: path, Alias: alias})
		}
	}
	if len(uses) == 0 {
		return nil
	}
	return uses
}

func (m *Model) rtsVars(
	doc *restfile.Document,
	req *restfile.Request,
	envName string,
	extras ...map[string]string,
) map[string]string {
	res := m.collectVariables(doc, req, envName)
	for _, extra := range extras {
		for k, v := range extra {
			res[k] = v
		}
	}
	return res
}

func (m *Model) rtsVarsSafe(
	doc *restfile.Document,
	req *restfile.Request,
	envName string,
	extras ...map[string]string,
) map[string]string {
	res := make(map[string]string)
	if env := vars.EnvValues(m.cfg.EnvironmentSet, envName); len(env) > 0 {
		for k, v := range env {
			res[k] = v
		}
	}

	if doc != nil {
		for _, v := range doc.Variables {
			if v.Secret {
				continue
			}
			res[v.Name] = v.Value
		}
		for _, v := range doc.Globals {
			if v.Secret {
				continue
			}
			res[v.Name] = v.Value
		}
	}

	m.mergeFileRuntimeVarsSafe(res, doc, envName)

	if gs := m.globalsStore(); gs != nil {
		if snap := gs.Snapshot(envName); len(snap) > 0 {
			for k, e := range snap {
				if e.Secret {
					continue
				}
				name := strings.TrimSpace(e.Name)
				if name == "" {
					name = k
				}
				res[name] = e.Value
			}
		}
	}

	if req != nil {
		for _, v := range req.Variables {
			if v.Secret {
				continue
			}
			res[v.Name] = v.Value
		}
	}

	for _, extra := range extras {
		for k, v := range extra {
			res[k] = v
		}
	}
	return res
}

func rtsHTTP(resp *httpclient.Response) *rts.Resp {
	if resp == nil {
		return nil
	}
	h := make(map[string][]string, len(resp.Headers))
	for k, v := range resp.Headers {
		vv := append([]string(nil), v...)
		h[k] = vv
	}
	return &rts.Resp{
		Status: resp.Status,
		Code:   resp.StatusCode,
		H:      h,
		Body:   resp.Body,
		URL:    resp.EffectiveURL,
	}
}

func rtsTrace(resp *httpclient.Response) *rts.Trace {
	if resp == nil || resp.TraceReport == nil {
		return nil
	}
	return &rts.Trace{Rep: resp.TraceReport.Clone()}
}

func rtsGRPC(resp *grpcclient.Response) *rts.Resp {
	if resp == nil {
		return nil
	}
	h := make(map[string][]string, len(resp.Headers)+len(resp.Trailers))
	for k, v := range resp.Headers {
		vv := append([]string(nil), v...)
		h[k] = vv
	}
	for k, v := range resp.Trailers {
		vv := append([]string(nil), v...)
		h[k] = vv
	}
	return &rts.Resp{Status: resp.StatusMessage, Code: int(resp.StatusCode), H: h, Body: resp.Body}
}

func rtsScriptResp(resp *scripts.Response) *rts.Resp {
	if resp == nil {
		return nil
	}
	h := make(map[string][]string, len(resp.Header))
	for k, v := range resp.Header {
		vv := append([]string(nil), v...)
		h[k] = vv
	}
	b := append([]byte(nil), resp.Body...)
	return &rts.Resp{
		Status: resp.Status,
		Code:   resp.Code,
		H:      h,
		Body:   b,
		URL:    resp.URL,
	}
}

func (m *Model) rtsLast() *rts.Resp {
	if m.lastResponse != nil {
		return rtsHTTP(m.lastResponse)
	}
	if m.lastGRPC != nil {
		return rtsGRPC(m.lastGRPC)
	}
	return nil
}

func (m *Model) rtsTrace() *rts.Trace {
	if m.lastResponse != nil {
		return rtsTrace(m.lastResponse)
	}
	return nil
}

type rtsRTIn struct {
	doc  *restfile.Document
	req  *restfile.Request
	env  string
	base string
	v    map[string]string
	x    map[string]rts.Value
	site string
	safe bool
	resp *rts.Resp
	res  *rts.Resp
	tr   *rts.Trace
	st   *rts.Stream
}

func (m *Model) rtsRT(in rtsRTIn) rts.RT {
	base := m.rtsBase(in.doc, in.base)
	resp := in.resp
	if resp == nil {
		resp = m.rtsLast()
	}
	res := in.res
	if res == nil {
		res = resp
	}
	tr := in.tr
	if tr == nil {
		tr = m.rtsTrace()
	}
	return rts.RT{
		Env:         m.rtsEnv(in.env),
		Vars:        in.v,
		Globals:     globalValues(m.collectGlobalValues(in.doc, in.env), in.safe),
		Resp:        resp,
		Res:         res,
		Trace:       tr,
		Stream:      in.st,
		Req:         m.rtsReq(in.req),
		BaseDir:     base,
		ReadFile:    os.ReadFile,
		AllowRandom: true,
		Site:        in.site,
		Uses:        m.rtsUses(in.doc, in.req),
		Extra:       in.x,
	}
}

func (m *Model) rtsEval(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	envName, base string,
	safe bool,
	extraVals map[string]rts.Value,
	extras ...map[string]string,
) vars.ExprEval {
	if m.rtsEng == nil {
		m.rtsEng = rts.NewEng(stdlib.New)
	}
	var v map[string]string
	if safe {
		v = m.rtsVarsSafe(doc, req, envName, extras...)
	} else {
		v = m.rtsVars(doc, req, envName, extras...)
	}
	return func(expr string, pos vars.ExprPos) (string, error) {
		rt := m.rtsRT(rtsRTIn{
			doc:  doc,
			req:  req,
			env:  envName,
			base: base,
			v:    v,
			x:    extraVals,
			site: "{{= " + expr + " }}",
			safe: safe,
		})
		rp := rts.Pos{Path: pos.Path, Line: pos.Line, Col: pos.Col}
		return m.rtsEng.EvalStr(ctx, rt, expr, rp)
	}
}

func (m *Model) rtsEvalValue(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	envName, base, expr, site string,
	pos rts.Pos,
	vars map[string]string,
	extraVals map[string]rts.Value,
) (rts.Value, error) {
	if m.rtsEng == nil {
		m.rtsEng = rts.NewEng(stdlib.New)
	}
	if vars == nil {
		vars = m.rtsVars(doc, req, envName)
	}
	rt := m.rtsRT(rtsRTIn{
		doc:  doc,
		req:  req,
		env:  envName,
		base: base,
		v:    vars,
		x:    extraVals,
		site: site,
	})
	return m.rtsEng.Eval(ctx, rt, expr, pos)
}

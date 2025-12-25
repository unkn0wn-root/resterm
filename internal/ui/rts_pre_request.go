package ui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

type rtsPreMut struct {
	out   *scripts.PreRequestOutput
	req   *rts.Req
	vars  map[string]string
	globs map[string]string
}

func newRTSPreMut(
	out *scripts.PreRequestOutput,
	req *rts.Req,
	vars map[string]string,
	globs map[string]string,
) *rtsPreMut {
	return &rtsPreMut{out: out, req: req, vars: vars, globs: globs}
}

func (m *Model) runRTSPreRequest(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	envName, base string,
	vars map[string]string,
	globals map[string]scripts.GlobalValue,
) (scripts.PreRequestOutput, error) {
	out := scripts.PreRequestOutput{}
	if req == nil {
		return out, nil
	}
	eng := m.rtsEng
	if eng == nil {
		eng = rts.NewEng()
		m.rtsEng = eng
	}
	uses := m.rtsUses(doc, req)
	env := m.rtsEnv(envName)
	baseDir := m.rtsBase(doc, base)
	globs := globalValues(globals, false)
	mut := newRTSPreMut(&out, m.rtsReq(req), vars, globs)
	emptyResp := &rts.Resp{}

	for idx, block := range req.Metadata.Scripts {
		if !isRTSPre(block) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return out, err
		}
		src, path, err := loadRTSScript(block, baseDir)
		if err != nil {
			return out, fmt.Errorf("rts pre-request script %d: %w", idx+1, err)
		}
		if strings.TrimSpace(src) == "" {
			continue
		}
		pos := m.rtsPosForLine(doc, req, 0)
		if path != "" {
			pos.Path = path
			pos.Line = 1
			pos.Col = 1
		}
		rt := rts.RT{
			Env:         env,
			Vars:        vars,
			Globals:     globs,
			Resp:        m.rtsLast(),
			Res:         emptyResp,
			Trace:       m.rtsTrace(),
			Req:         mut.req,
			ReqMut:      mut,
			VarsMut:     mut,
			GlobalMut:   mut,
			Uses:        uses,
			BaseDir:     baseDir,
			ReadFile:    os.ReadFile,
			AllowRandom: true,
			Site:        "@script pre-request",
		}
		if _, err := eng.ExecModule(ctx, rt, src, pos); err != nil {
			return out, err
		}
	}

	trimPreOutput(&out)
	return out, nil
}

func isRTSPre(block restfile.ScriptBlock) bool {
	if strings.ToLower(block.Kind) != "pre-request" {
		return false
	}
	return scriptLang(block.Lang) == "rts"
}

func scriptLang(lang string) string {
	val := strings.ToLower(strings.TrimSpace(lang))
	switch val {
	case "", "javascript":
		return "js"
	case "restermlang":
		return "rts"
	default:
		return val
	}
}

func loadRTSScript(block restfile.ScriptBlock, base string) (string, string, error) {
	if block.FilePath == "" {
		return block.Body, "", nil
	}
	path := strings.TrimSpace(block.FilePath)
	if path == "" {
		return "", "", nil
	}
	if !filepath.IsAbs(path) && base != "" {
		path = filepath.Join(base, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	return string(data), path, nil
}

func trimPreOutput(out *scripts.PreRequestOutput) {
	if out == nil {
		return
	}
	if len(out.Headers) == 0 {
		out.Headers = nil
	}
	if len(out.Query) == 0 {
		out.Query = nil
	}
	if len(out.Variables) == 0 {
		out.Variables = nil
	}
	if len(out.Globals) == 0 {
		out.Globals = nil
	}
}

func globalValues(globals map[string]scripts.GlobalValue, safe bool) map[string]string {
	if len(globals) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(globals))
	for key, entry := range globals {
		if safe && entry.Secret {
			continue
		}
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			name = key
		}
		out[lowerKey(name)] = entry.Value
	}
	return out
}

func (m *rtsPreMut) SetMethod(value string) {
	val := strings.ToUpper(strings.TrimSpace(value))
	setPtr(&m.out.Method, val)
	if m.req != nil {
		m.req.Method = val
	}
}

func (m *rtsPreMut) SetURL(value string) {
	val := strings.TrimSpace(value)
	setPtr(&m.out.URL, val)
	if m.req != nil {
		m.req.URL = val
		m.req.Q = nil
	}
}

func (m *rtsPreMut) SetHeader(name, value string) {
	if m.out.Headers == nil {
		m.out.Headers = make(http.Header)
	}
	m.out.Headers.Set(name, value)
	if m.req != nil {
		setReqHeader(m.req, name, value, false)
	}
}

func (m *rtsPreMut) AddHeader(name, value string) {
	if m.out.Headers == nil {
		m.out.Headers = make(http.Header)
	}
	m.out.Headers.Add(name, value)
	if m.req != nil {
		setReqHeader(m.req, name, value, true)
	}
}

func (m *rtsPreMut) DelHeader(name string) {
	if m.out.Headers != nil {
		m.out.Headers.Del(name)
	}
	if m.req != nil && m.req.H != nil {
		key := lowerKey(name)
		delete(m.req.H, key)
	}
}

func (m *rtsPreMut) SetQuery(name, value string) {
	if m.out.Query == nil {
		m.out.Query = make(map[string]string)
	}
	m.out.Query[name] = value
	if m.req != nil {
		setReqQuery(m.req, name, value)
	}
}

func (m *rtsPreMut) SetBody(value string) {
	setPtr(&m.out.Body, value)
}

func (m *rtsPreMut) SetVar(name, value string) {
	if m.out.Variables == nil {
		m.out.Variables = make(map[string]string)
	}
	m.out.Variables[name] = value
	if m.vars != nil {
		m.vars[name] = value
	}
}

func (m *rtsPreMut) SetGlobal(name, value string, secret bool) {
	if m.out.Globals == nil {
		m.out.Globals = make(map[string]scripts.GlobalValue)
	}
	m.out.Globals[name] = scripts.GlobalValue{Name: name, Value: value, Secret: secret}
	if m.globs != nil {
		key := lowerKey(name)
		m.globs[key] = value
	}
}

func (m *rtsPreMut) DelGlobal(name string) {
	if m.out.Globals == nil {
		m.out.Globals = make(map[string]scripts.GlobalValue)
	}
	m.out.Globals[name] = scripts.GlobalValue{Name: name, Delete: true}
	if m.globs != nil {
		key := lowerKey(name)
		delete(m.globs, key)
	}
}

func setPtr(dst **string, value string) {
	if dst == nil {
		return
	}
	val := value
	*dst = &val
}

func setReqHeader(req *rts.Req, name, value string, appendValue bool) {
	if req == nil {
		return
	}
	if req.H == nil {
		req.H = make(map[string][]string)
	}
	key := lowerKey(name)
	if appendValue {
		req.H[key] = append(req.H[key], value)
		return
	}
	req.H[key] = []string{value}
}

func setReqQuery(req *rts.Req, name, value string) {
	if req == nil {
		return
	}
	if req.Q == nil {
		req.Q = make(map[string][]string)
	}
	req.Q[name] = []string{value}
	if req.URL == "" {
		return
	}
	parsed, err := url.Parse(req.URL)
	if err != nil {
		return
	}
	query := parsed.Query()
	query.Set(name, value)
	parsed.RawQuery = query.Encode()
	req.URL = parsed.String()
}

func lowerKey(name string) string {
	return strings.ToLower(name)
}

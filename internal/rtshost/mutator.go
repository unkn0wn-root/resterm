package rtshost

import (
	"net/http"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/prerequest"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/urltpl"
	"github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

var (
	_ rts.ReqMut    = (*Mutator)(nil)
	_ rts.VarsMut   = (*Mutator)(nil)
	_ rts.GlobalMut = (*Mutator)(nil)
)

// Mutator records script mutations into a prerequest.Output and mirrors them
// onto the runtime views so later statements read what earlier ones wrote.
type Mutator struct {
	out  *prerequest.Output
	req  *rts.Req
	vars map[string]string
	glob map[string]string
	sec  *vars.Secrets
}

func NewMutator(
	out *prerequest.Output,
	req *rts.Req,
	vals, globals map[string]string,
	sec *vars.Secrets,
) *Mutator {
	if req == nil {
		req = &rts.Req{}
	}
	return &Mutator{out: out, req: req, vars: vals, glob: globals, sec: sec}
}

func (m *Mutator) Request() *rts.Req { return m.req }

func (m *Mutator) SetMethod(value string) {
	val := util.UpperTrim(value)
	m.out.Method = &val
	m.req.Method = val
}

func (m *Mutator) SetURL(value string) {
	val := strings.TrimSpace(value)
	m.out.URL = &val
	m.req.URL = val
	// Drop the parsed query so the request view re-reads it from the new URL.
	m.req.Q = nil
}

func (m *Mutator) SetHeader(name, value string) {
	m.outHeaders().Set(name, value)
	m.reqHeaders()[util.Lower(name)] = []string{value}
}

func (m *Mutator) AddHeader(name, value string) {
	m.outHeaders().Add(name, value)
	h, key := m.reqHeaders(), util.Lower(name)
	h[key] = append(h[key], value)
}

func (m *Mutator) DelHeader(name string) {
	if m.out.Headers != nil {
		m.out.Headers.Del(name)
	}
	delete(m.req.H, util.Lower(name))
}

func (m *Mutator) SetQuery(name, value string) {
	if m.out.Query == nil {
		m.out.Query = make(map[string]string)
	}
	m.out.Query[name] = value
	m.setReqQuery(name, value)
}

func (m *Mutator) SetBody(value string) {
	m.out.Body = &value
}

// SetVar records the write and updates the host's plain-map view without
// leaving duplicate name forms.
func (m *Mutator) SetVar(name, value string) {
	if !m.out.Variables.Set(name, value) {
		return
	}
	if m.vars != nil {
		vars.Upsert(m.vars, name, value)
	}
}

func (m *Mutator) SetGlobal(name, value string, secret bool) {
	if !m.out.Globals.Set(name, vars.GlobalMutation{Name: name, Value: value, Secret: secret}) {
		return
	}
	if secret {
		m.sec.Add(value)
	}
	if m.glob != nil {
		m.glob[util.Lower(name)] = value
	}
}

func (m *Mutator) DelGlobal(name string) {
	if !m.out.Globals.Set(name, vars.GlobalMutation{Name: name, Delete: true}) {
		return
	}
	delete(m.glob, util.Lower(name))
}

// setReqQuery mirrors the parameter onto the request view. A URL that cannot be
// patched keeps its old value and the recorded output still applies the parameter.
func (m *Mutator) setReqQuery(name, value string) {
	if m.req.Q == nil {
		m.req.Q = make(map[string][]string)
	}
	m.req.Q[name] = []string{value}
	if m.req.URL == "" {
		return
	}
	url, err := urltpl.PatchQuery(m.req.URL, map[string]*string{name: &value})
	if err != nil {
		return
	}
	m.req.URL = url
}

func (m *Mutator) outHeaders() http.Header {
	if m.out.Headers == nil {
		m.out.Headers = make(http.Header)
	}
	return m.out.Headers
}

func (m *Mutator) reqHeaders() map[string][]string {
	if m.req.H == nil {
		m.req.H = make(map[string][]string)
	}
	return m.req.H
}

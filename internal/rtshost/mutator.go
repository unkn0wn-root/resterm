package rtshost

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/httpheader"
	"github.com/unkn0wn-root/resterm/internal/prerequest"
	"github.com/unkn0wn-root/resterm/internal/queryparams"
	"github.com/unkn0wn-root/resterm/internal/urltpl"
	"github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

var (
	_ RequestMutator = (*Mutator)(nil)
	_ VarsMutator    = (*Mutator)(nil)
	_ GlobalMutator  = (*Mutator)(nil)
)

// Mutator records script mutations into a prerequest.Output and mirrors them
// onto the runtime views so later statements read what earlier ones wrote.
type Mutator struct {
	out  *prerequest.Output
	req  *Request
	vars map[string]string
	glob map[string]string
	sec  *vars.Secrets
}

func NewMutator(
	out *prerequest.Output,
	req *Request,
	vals, globals map[string]string,
	sec *vars.Secrets,
) *Mutator {
	if req == nil {
		req = &Request{}
	}
	return &Mutator{out: out, req: req, vars: vals, glob: globals, sec: sec}
}

func (m *Mutator) Request() *Request { return m.req }

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
	m.req.Query = nil
}

func (m *Mutator) SetHeader(name httpheader.Name, value string) {
	m.out.SetHeader(name.Key(), value)
	m.reqHeaders()[name.Key()] = []string{value}
}

func (m *Mutator) AddHeader(name httpheader.Name, value string) {
	m.out.AddHeader(name.Key(), value)
	h := m.reqHeaders()
	h[name.Key()] = append(h[name.Key()], value)
}

func (m *Mutator) DelHeader(name httpheader.Name) {
	m.out.DelHeader(name.Key())
	delete(m.req.Headers, name.Key())
}

func (m *Mutator) SetQuery(name, value string) {
	m.out.SetQuery(name, value)
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
		m.glob[vars.NameKey(name)] = value
	}
}

func (m *Mutator) DelGlobal(name string) {
	if !m.out.Globals.Set(name, vars.GlobalMutation{Name: name, Delete: true}) {
		return
	}
	delete(m.glob, vars.NameKey(name))
}

// setReqQuery mirrors the parameter onto the request view. A URL that cannot be
// patched keeps its old value and the recorded output still applies the parameter.
func (m *Mutator) setReqQuery(name, value string) {
	if m.req.Query == nil {
		q, err := targetQuery(m.req.URL)
		if err != nil {
			q = make(queryparams.Values)
		}
		m.req.Query = q
	}
	m.req.Query[name] = []string{value}
	if m.req.URL == "" {
		return
	}
	url, err := urltpl.PatchQuery(m.req.URL, map[string]*string{name: &value})
	if err != nil {
		return
	}
	m.req.URL = url
}

func (m *Mutator) reqHeaders() httpheader.Values {
	if m.req.Headers == nil {
		m.req.Headers = make(httpheader.Values)
	}
	return m.req.Headers
}

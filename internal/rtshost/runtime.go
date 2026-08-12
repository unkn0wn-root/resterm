package rtshost

import (
	"fmt"

	"github.com/unkn0wn-root/resterm/internal/http/header"
	"github.com/unkn0wn-root/resterm/internal/http/query"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// EnvMeta describes an environment selection without occupying names in the
// environment value map.
type EnvMeta struct {
	Name   string
	Groups vars.NameView[string]
}

// Scope contains the environment, variables, and globals for one evaluation.
// Its maps are read-only so prepared data can be shared between bindings.
type Scope struct {
	Env     vars.NameView[string]
	Meta    EnvMeta
	Vars    vars.NameView[string]
	Globals vars.NameView[string]
}

// PreparedScope stores validated environment data that can be reused while
// binding different variable maps.
type PreparedScope struct {
	env     vars.NameView[string]
	meta    EnvMeta
	globals vars.NameView[string]
}

// ScopeInput contains the environment and globals validated by PrepareScope.
// Runtime variables are passed separately to BindVars.
type ScopeInput struct {
	EnvName string
	Env     map[string]string
	Groups  map[string]string
	Globals map[string]string
}

// PrepareScope validates environment and global names for reuse across
// bindings. It checks environment values, groups, and globals in that order so
// errors are deterministic when several layers are invalid.
func PrepareScope(in ScopeInput) (PreparedScope, error) {
	ev, err := strictNames("env", in.Env)
	if err != nil {
		return PreparedScope{}, err
	}
	gr, err := strictNames("env.meta.groups", in.Groups)
	if err != nil {
		return PreparedScope{}, err
	}
	gv, err := strictNames("vars.global", in.Globals)
	if err != nil {
		return PreparedScope{}, err
	}
	return PreparedScope{
		env:     ev.View(),
		meta:    EnvMeta{Name: in.EnvName, Groups: gr.View()},
		globals: gv.View(),
	}, nil
}

// BindVars validates values and returns a Scope that shares the prepared
// read-only maps.
func (p PreparedScope) BindVars(values map[string]string) (Scope, error) {
	vv, err := strictNames("vars", values)
	if err != nil {
		return Scope{}, err
	}
	return Scope{Env: p.env, Meta: p.meta, Vars: vv.View(), Globals: p.globals}, nil
}

// NewScope validates all scope layers for one evaluation. Use PrepareScope and
// BindVars when environment data is reused with different variable maps.
func NewScope(in ScopeInput, values map[string]string) (Scope, error) {
	p, err := PrepareScope(in)
	if err != nil {
		return Scope{}, err
	}
	return p.BindVars(values)
}

func strictNames(kind string, src map[string]string) (vars.NameMap[string], error) {
	out, err := vars.NewNameMap(src)
	if err != nil {
		return vars.NameMap[string]{}, fmt.Errorf("%s: %w", kind, err)
	}
	return out, nil
}

// RequestMutator changes a request through the owning host.
type RequestMutator interface {
	SetMethod(value string)
	SetURL(value string)
	SetHeader(name header.Name, value string)
	AddHeader(name header.Name, value string)
	DelHeader(name header.Name)
	SetQuery(name, value string)
	SetBody(value string)
}

// VarsMutator persists a runtime variable write.
type VarsMutator interface {
	SetVar(name, value string)
}

// GlobalMutator persists a global variable mutation.
type GlobalMutator interface {
	SetGlobal(name, value string, secret bool)
	DelGlobal(name string)
}

// Request is the typed host request view.
type Request struct {
	Method  string
	URL     string
	Headers header.Values
	Query   query.Values
}

// NewRequest validates and copies a request view. A nil query is derived lazily
// from rawURL. An empty non-nil query represents an explicitly empty query.
func NewRequest(method, rawURL string, headers, rawQuery map[string][]string) (*Request, error) {
	h, err := header.Normalize(headers)
	if err != nil {
		return nil, fmt.Errorf("request headers: %w", err)
	}
	var q query.Values
	if rawQuery != nil {
		q = query.Clone(rawQuery)
	}
	return &Request{Method: method, URL: rawURL, Headers: h, Query: q}, nil
}

// Response is the typed host response view.
type Response struct {
	Status  string
	Code    int
	Headers header.Values
	Body    []byte
	URL     string
}

// NewResponse validates and copies a response view.
func NewResponse(
	status string,
	code int,
	headers map[string][]string,
	body []byte,
	rawURL string,
) (*Response, error) {
	h, err := header.Normalize(headers)
	if err != nil {
		return nil, fmt.Errorf("response headers: %w", err)
	}
	return &Response{
		Status:  status,
		Code:    code,
		Headers: h,
		Body:    append([]byte(nil), body...),
		URL:     rawURL,
	}, nil
}

// Stream is the host stream view.
type Stream struct {
	Kind    string
	Summary map[string]any
	Events  []map[string]any
}

// Runtime contains the host data and policy for one evaluation. A nil ReadFile
// uses the RTS default filesystem reader.
//
// Mutator is nil for read-only evaluations. Keep it as a pointer until a
// specific mutator interface is needed because a nil pointer stored in an
// interface is non-nil.
type Runtime struct {
	Scope       Scope
	Last        *Response
	Response    *Response
	Trace       *Trace
	Stream      *Stream
	Request     *Request
	Mutator     *Mutator
	Uses        []rts.Use
	BaseDir     string
	ReadFile    func(string) ([]byte, error)
	AllowRandom bool
	Site        string
	Extensions  rts.Extensions
	Locals      rts.Locals
}

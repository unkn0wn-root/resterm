package rtshost

import (
	"fmt"
	"os"

	"github.com/unkn0wn-root/resterm/internal/httpheader"
	"github.com/unkn0wn-root/resterm/internal/queryparams"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// EnvMeta describes an environment selection without occupying names in the
// environment value map.
type EnvMeta struct {
	Name   string
	Groups vars.NameMap[string]
}

// Scope is the name-bearing host state shared by an evaluation.
type Scope struct {
	Env     vars.NameMap[string]
	Meta    EnvMeta
	Vars    vars.NameMap[string]
	Globals vars.NameMap[string]
}

// NewScope validates raw host maps. Equivalent or blank names are rejected at
// the boundary instead of being resolved by map iteration order.
func NewScope(
	env map[string]string,
	envName string,
	groups map[string]string,
	values map[string]string,
	globals map[string]string,
) (Scope, error) {
	ev, err := strictNames("env", env)
	if err != nil {
		return Scope{}, err
	}
	gr, err := strictNames("env.meta.groups", groups)
	if err != nil {
		return Scope{}, err
	}
	vv, err := strictNames("vars", values)
	if err != nil {
		return Scope{}, err
	}
	gv, err := strictNames("vars.global", globals)
	if err != nil {
		return Scope{}, err
	}
	return Scope{
		Env:     ev,
		Meta:    EnvMeta{Name: envName, Groups: gr},
		Vars:    vv,
		Globals: gv,
	}, nil
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
	SetHeader(name httpheader.Name, value string)
	AddHeader(name httpheader.Name, value string)
	DelHeader(name httpheader.Name)
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
	Headers httpheader.Values
	Query   queryparams.Values
}

// NewRequest validates and copies a request view. A nil query asks the RTS
// request object to derive it lazily from rawURL; an empty non-nil query is an
// explicit empty view.
func NewRequest(
	method, rawURL string,
	headers map[string][]string,
	query map[string][]string,
) (*Request, error) {
	h, err := httpheader.Normalize(headers)
	if err != nil {
		return nil, fmt.Errorf("request headers: %w", err)
	}
	var q queryparams.Values
	if query != nil {
		q = queryparams.Clone(query)
	}
	return &Request{Method: method, URL: rawURL, Headers: h, Query: q}, nil
}

// Response is the typed host response view.
type Response struct {
	Status  string
	Code    int
	Headers httpheader.Values
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
	h, err := httpheader.Normalize(headers)
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

// Runtime contains all host policy translated for one evaluation.
type Runtime struct {
	Scope       Scope
	Last        *Response
	Response    *Response
	Trace       *Trace
	Stream      *Stream
	Request     *Request
	RequestMut  RequestMutator
	VarsMut     VarsMutator
	GlobalMut   GlobalMutator
	Uses        []rts.Use
	BaseDir     string
	ReadFile    func(string) ([]byte, error)
	AllowRandom bool
	Site        string
	Extensions  rts.Extensions
	Locals      rts.Locals
}

func (rt Runtime) readFile() func(string) ([]byte, error) {
	if rt.ReadFile != nil {
		return rt.ReadFile
	}
	return os.ReadFile
}

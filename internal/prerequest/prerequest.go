package prerequest

import (
	"context"
	"net/http"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/http/urltpl"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// Input is the host state available to a pre-request script runner.
type Input struct {
	Request   *restfile.Request
	Variables map[string]string
	Globals   vars.Globals
	BaseDir   string
	Context   context.Context
	Secrets   *vars.Secrets
	// Expand renders the authored values the script reads. A nil Expand shows
	// them as written.
	Expand ExpandFunc
}

// ExpandFunc renders an authored request value as a script reads it.
// Variables and expressions see the script's own writes in set.
type ExpandFunc func(text string, set vars.NameMap[string]) (string, error)

// Bind returns f for a host that keeps its writes in set. set is read on each
// call, so later writes are seen. A nil f binds to nil.
func (f ExpandFunc) Bind(set *vars.NameMap[string]) func(string) (string, error) {
	if f == nil {
		return nil
	}
	return func(text string) (string, error) {
		var s vars.NameMap[string]
		if set != nil {
			s = *set
		}
		return f(text, s)
	}
}

// Output is a set of request changes from pre-request scripts or @apply patches.
type Output struct {
	Headers http.Header
	// Removals are tracked separately because headers declared in the file are
	// not part of Headers and would otherwise survive the script.
	HeaderDels map[string]struct{}
	// Query values are set, or removed when nil.
	Query  map[string]*string
	Body   *string
	URL    *string
	Method *string
	// Variables contains script writes, normalized to one entry per name.
	Variables vars.NameMap[string]
	Globals   vars.Globals
}

func (o *Output) SetHeader(name, value string) {
	o.headers().Set(name, value)
}

func (o *Output) AddHeader(name, value string) {
	o.headers().Add(name, value)
}

func (o *Output) DelHeader(name string) {
	o.Headers.Del(name)
	if o.HeaderDels == nil {
		o.HeaderDels = make(map[string]struct{})
	}
	o.HeaderDels[http.CanonicalHeaderKey(name)] = struct{}{}
}

func (o *Output) SetQuery(name, value string) {
	if o.Query == nil {
		o.Query = make(map[string]*string)
	}
	o.Query[name] = &value
}

func (o *Output) headers() http.Header {
	if o.Headers == nil {
		o.Headers = make(http.Header)
	}
	return o.Headers
}

// Apply writes script or patch output onto req. The output is data, so only
// its dynamic helpers are rendered, and req records what was written so
// template expansion leaves it unchanged.
func Apply(req *restfile.Request, out Output) error {
	if req == nil {
		return nil
	}
	if out.Method != nil {
		req.Method = *out.Method
	}
	if out.URL != nil {
		u, err := renderHelpers("url", *out.URL)
		if err != nil {
			return err
		}
		req.SetURL(u)
		req.Written.URL = true
	}
	if len(out.Query) > 0 {
		if err := applyQuery(req, out.Query); err != nil {
			return err
		}
	}
	if err := applyHeaders(req, out.Headers, out.HeaderDels); err != nil {
		return err
	}
	if out.Body != nil {
		body, err := renderHelpers("body", *out.Body)
		if err != nil {
			return err
		}
		req.SetBodyText(body)
		req.Written.Body = true
	}
	SetRequestVars(req, out.Variables)
	return nil
}

func renderHelpers(field, value string) (string, error) {
	out, err := vars.ExpandHelpers(value)
	if err != nil {
		return "", diag.WrapAs(diag.ClassScript, err, "render "+field)
	}
	return out, nil
}

func Normalize(out *Output) {
	if out == nil {
		return
	}
	out.Headers = nilIfEmpty(out.Headers)
	out.HeaderDels = nilIfEmpty(out.HeaderDels)
	out.Query = nilIfEmpty(out.Query)
}

func nilIfEmpty[M ~map[K]V, K comparable, V any](m M) M {
	if len(m) != 0 {
		return m
	}
	var zero M
	return zero
}

// Apply removals first so a value set later in the same batch is kept.
func applyHeaders(req *restfile.Request, set http.Header, del map[string]struct{}) error {
	for name := range del {
		req.DelHeader(name)
	}
	for name, values := range set {
		out := make([]string, len(values))
		for i, value := range values {
			v, err := renderHelpers("header "+name, value)
			if err != nil {
				return err
			}
			out[i] = v
		}
		req.SetWrittenHeader(name, out...)
	}
	return nil
}

func applyQuery(req *restfile.Request, q map[string]*string) error {
	raw := req.URL
	patch := make(map[string]*string, len(q))
	for key, value := range q {
		if value == nil {
			patch[key] = nil
			continue
		}
		v, err := renderHelpers("query "+key, *value)
		if err != nil {
			return err
		}
		patch[key] = &v
	}
	updated, err := urltpl.PatchQuery(raw, patch)
	if err != nil {
		return diag.WrapAs(diag.ClassScript, err, "invalid url after request changes")
	}
	if raw == "" && updated == "" {
		return nil
	}
	req.URL = updated
	return nil
}

// SetRequestVars merges script writes into request-scoped variables. Names are
// matched case-insensitively after trimming whitespace.
// New variables are appended in deterministic order.
func SetRequestVars(req *restfile.Request, variables vars.NameMap[string]) {
	if req == nil || variables.Len() == 0 {
		return
	}
	idxs := make(map[string]int)
	for i, variable := range req.Variables {
		idxs[vars.NameKey(variable.Name)] = i
	}
	for name, value := range variables.Sorted() {
		key := vars.NameKey(name)
		if idx, ok := idxs[key]; ok {
			req.Variables[idx].SetRuntimeValue(value)
			continue
		}
		idxs[key] = len(req.Variables)
		req.Variables = append(req.Variables, restfile.Variable{
			Name:  name,
			Value: value,
			Scope: directive.ScopeRequest,
		})
	}
}

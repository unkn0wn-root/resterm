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
}

// Output is the request mutation set produced by pre-request scripts.
type Output struct {
	Headers http.Header
	// Removals are tracked separately because headers declared in the file are
	// not part of Headers and would otherwise survive the script.
	HeaderDels map[string]struct{}
	Query      map[string]string
	Body       *string
	URL        *string
	Method     *string
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
		o.Query = make(map[string]string)
	}
	o.Query[name] = value
}

func (o *Output) headers() http.Header {
	if o.Headers == nil {
		o.Headers = make(http.Header)
	}
	return o.Headers
}

// Apply mutates req with pre-request script output.
func Apply(req *restfile.Request, out Output) error {
	if req == nil {
		return nil
	}
	if out.Method != nil {
		req.Method = *out.Method
	}
	if out.URL != nil {
		req.URL = *out.URL
	}
	if len(out.Query) > 0 {
		if err := applyQuery(req, out.Query); err != nil {
			return diag.WrapAs(diag.ClassScript, err, "invalid url after script")
		}
	}
	applyHeaders(req, out.Headers, out.HeaderDels)
	if out.Body != nil {
		req.Body.FilePath = ""
		req.Body.Text = *out.Body
		req.Body.GraphQL = nil
	}
	SetRequestVars(req, out.Variables)
	return nil
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

// Apply removals first so a value set later in the same script batch is kept.
func applyHeaders(req *restfile.Request, set http.Header, del map[string]struct{}) {
	for name := range del {
		req.Headers.Del(name)
	}
	if len(set) == 0 {
		return
	}
	if req.Headers == nil {
		req.Headers = make(http.Header)
	}
	for name, values := range set {
		req.Headers.Del(name)
		for _, value := range values {
			req.Headers.Add(name, value)
		}
	}
}

func applyQuery(req *restfile.Request, q map[string]string) error {
	if req == nil || len(q) == 0 {
		return nil
	}
	raw := req.URL
	patch := make(map[string]*string, len(q))
	for key, value := range q {
		val := value
		patch[key] = &val
	}
	updated, err := urltpl.PatchQuery(raw, patch)
	if err != nil {
		return err
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
			req.Variables[idx].Value = value
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

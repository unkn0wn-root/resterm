package prerequest

import (
	"context"
	"net/http"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/urltpl"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// Input is the host state available to a pre-request script runner.
type Input struct {
	Request   *restfile.Request
	Variables map[string]string
	Globals   map[string]vars.GlobalMutation
	BaseDir   string
	Context   context.Context
}

// Output is the request mutation set produced by pre-request scripts.
type Output struct {
	Headers   http.Header
	Query     map[string]string
	Body      *string
	URL       *string
	Method    *string
	Variables map[string]string
	Globals   map[string]vars.GlobalMutation
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
	if out.Headers != nil {
		if req.Headers == nil {
			req.Headers = make(http.Header)
		}
		for name, values := range out.Headers {
			req.Headers.Del(name)
			for _, value := range values {
				req.Headers.Add(name, value)
			}
		}
	}
	if out.Body != nil {
		req.Body.FilePath = ""
		req.Body.Text = *out.Body
		req.Body.GraphQL = nil
	}
	setRequestVars(req, out.Variables)
	return nil
}

func Normalize(out *Output) {
	if out == nil {
		return
	}
	out.Headers = nilIfEmpty(out.Headers)
	out.Query = nilIfEmpty(out.Query)
	out.Variables = nilIfEmpty(out.Variables)
	out.Globals = nilIfEmpty(out.Globals)
}

func nilIfEmpty[M ~map[K]V, K comparable, V any](m M) M {
	if len(m) != 0 {
		return m
	}
	var zero M
	return zero
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

func setRequestVars(req *restfile.Request, variables map[string]string) {
	if req == nil || len(variables) == 0 {
		return
	}
	existing := make(map[string]int)
	for i, variable := range req.Variables {
		existing[strings.ToLower(variable.Name)] = i
	}
	for name, value := range variables {
		key := strings.ToLower(name)
		if idx, ok := existing[key]; ok {
			req.Variables[idx].Value = value
			continue
		}
		req.Variables = append(req.Variables, restfile.Variable{
			Name:  name,
			Value: value,
			Scope: restfile.ScopeRequest,
		})
	}
}

package mock

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"golang.org/x/net/http/httpguts"

	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type renderedResponse struct {
	headers http.Header
	body    []byte
}

type requestTemplate struct {
	source     string
	subject    string
	encodeJSON bool
}

func parseRequestTemplate(name string) (requestTemplate, bool) {
	var ref requestTemplate
	if inner, ok := strings.CutPrefix(name, "json."); ok {
		ref.encodeJSON = true
		name = inner
	}

	var ok bool
	ref.source, ref.subject, ok = strings.Cut(name, ".")
	if !ok || invalidTemplateSubject(ref.subject) {
		return requestTemplate{}, false
	}
	return ref, true
}

// compileTemplates validates every placeholder against the route's path params
// and records which parts of the response need per-request interpolation.
func (r *response) compileTemplates(pathParams map[string]string) error {
	for name, values := range r.headers {
		for _, value := range values {
			has, err := validateTemplateString(value, pathParams)
			if err != nil {
				return fmt.Errorf("response header %q: %w", name, err)
			}
			r.interpHeaders = r.interpHeaders || has
		}
	}
	body := string(r.body)
	has, err := validateTemplateString(body, pathParams)
	if err != nil {
		return fmt.Errorf("response body: %w", err)
	}
	if has {
		r.interpBody = true
		r.bodyTpl = vars.CompileTemplate(body)
	}
	return nil
}

// Use the same template scanner during validation and rendering so both paths
// agree on what counts as a placeholder. An unterminated "{{" or a blank
// "{{ }}" stays literal, exactly as the resolver renders it.
func validateTemplateString(input string, pathParams map[string]string) (bool, error) {
	has := false
	var err error
	vars.ReplaceTemplateVars(input, func(match, name string) string {
		if name == "" {
			return match
		}
		has = true
		if err == nil {
			err = validateTemplateName(name, pathParams)
		}
		return match
	})
	return has, err
}

func validateTemplateName(name string, pathParams map[string]string) error {
	if strings.HasPrefix(name, "=") {
		return fmt.Errorf("response templates do not support expressions")
	}
	if strings.HasPrefix(name, "$") {
		if vars.IsDynamic(name) {
			return nil
		}
		return fmt.Errorf("unsupported dynamic response template %q", name)
	}

	ref, ok := parseRequestTemplate(name)
	if !ok {
		return fmt.Errorf("unsupported response template %q", name)
	}
	switch ref.source {
	case "path":
		if _, exists := pathParams[ref.subject]; !exists {
			return fmt.Errorf("mock path has no parameter %q", ref.subject)
		}
	case "query":
		return nil
	case "headers":
		if !httpguts.ValidHeaderFieldName(ref.subject) {
			return fmt.Errorf("invalid request header template %q", ref.subject)
		}
	case "body":
		if !rts.ValidJSONPath(ref.subject) {
			return fmt.Errorf("invalid JSON body template path %q", ref.subject)
		}
	default:
		return fmt.Errorf("unsupported response template namespace %q", ref.source)
	}
	return nil
}

func invalidTemplateSubject(subject string) bool {
	if subject == "" || subject != strings.TrimSpace(subject) {
		return true
	}
	return strings.IndexFunc(subject, func(r rune) bool {
		return unicode.IsControl(r) || r == '{' || r == '}'
	}) >= 0
}

func (v *variant) render(resp *response, p *probe) (renderedResponse, *problem) {
	out := renderedResponse{headers: resp.headers, body: resp.body}
	if !resp.interpHeaders && !resp.interpBody {
		return out, nil
	}

	provider := &requestProvider{probe: p, pathParams: v.pathParams}
	resolver := vars.NewResolver(provider)
	if resp.interpHeaders {
		headers := make(http.Header, len(resp.headers))
		for name, values := range resp.headers {
			for _, value := range values {
				expanded, err := resolver.ExpandTemplates(value)
				if err != nil {
					return renderedResponse{}, provider.renderProblem(err)
				}
				if !httpguts.ValidHeaderFieldValue(expanded) {
					return renderedResponse{}, &problem{
						status: http.StatusInternalServerError,
						detail: fmt.Sprintf(
							"mock response interpolation produced an invalid value for header %q",
							name,
						),
					}
				}
				headers.Add(name, expanded)
			}
		}
		out.headers = headers
	}
	if resp.interpBody {
		body, err := resp.bodyTpl.Render(resolver)
		if err != nil {
			return renderedResponse{}, provider.renderProblem(err)
		}
		out.body = []byte(body)
	}
	return out, nil
}

type requestProvider struct {
	probe      *probe
	pathParams map[string]string
	problem    *problem
}

func (p *requestProvider) Resolve(name string) (string, bool) {
	ref, ok := parseRequestTemplate(name)
	if !ok {
		return "", false
	}
	value, ok := p.resolveValue(ref, name)
	if !ok {
		return "", false
	}
	if text, ok := value.(string); ok && !ref.encodeJSON {
		return text, true
	}
	data, err := json.Marshal(value)
	if err != nil {
		p.setProblem(&problem{
			status: http.StatusInternalServerError,
			detail: fmt.Sprintf("encode mock response template {{%s}} as JSON: %v", name, err),
		})
		return "", false
	}
	return string(data), true
}

func (p *requestProvider) resolveValue(ref requestTemplate, name string) (any, bool) {
	switch ref.source {
	case "path":
		return p.resolvePath(ref.subject)
	case "query":
		return p.resolveQuery(ref.subject)
	case "headers":
		return p.resolveHeader(ref.subject)
	case "body":
		return p.resolveBody(ref.subject, name)
	default:
		return "", false
	}
}

func (p *requestProvider) Label() string {
	return "mock request"
}

func (p *requestProvider) resolvePath(name string) (string, bool) {
	wildcard, ok := p.pathParams[name]
	if !ok {
		return "", false
	}
	return p.probe.r.PathValue(wildcard), true
}

func (p *requestProvider) resolveQuery(name string) (string, bool) {
	values, ok := p.probe.query()[name]
	if !ok || len(values) == 0 {
		p.missing("query value", name)
		return "", false
	}
	return values[0], true
}

func (p *requestProvider) resolveHeader(name string) (string, bool) {
	values := headerValues(p.probe.r, name)
	if len(values) == 0 {
		p.missing("request header", name)
		return "", false
	}
	return values[0], true
}

func (p *requestProvider) resolveBody(path, name string) (any, bool) {
	body, ok, err := p.probe.json()
	if err != nil {
		p.setProblem(err)
		return "", false
	}
	if !ok {
		p.setProblem(&problem{
			status: http.StatusBadRequest,
			detail: "request must have a JSON body to interpolate {{" + name + "}}",
		})
		return "", false
	}
	value, found := rts.JSONPathGet(body, path)
	if !found {
		p.missing("JSON body field", path)
		return "", false
	}
	return value, true
}

func (p *requestProvider) missing(kind, name string) {
	p.setProblem(&problem{
		status: http.StatusBadRequest,
		detail: fmt.Sprintf("missing %s %q required by mock response", kind, name),
	})
}

func (p *requestProvider) setProblem(err *problem) {
	if p.problem == nil {
		p.problem = err
	}
}

func (p *requestProvider) renderProblem(err error) *problem {
	if p.problem != nil {
		return p.problem
	}
	return &problem{
		status: http.StatusInternalServerError,
		detail: "mock response interpolation failed: " + err.Error(),
	}
}

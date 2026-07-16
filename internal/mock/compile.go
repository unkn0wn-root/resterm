package mock

import (
	"cmp"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"golang.org/x/net/http/httpguts"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type fixtureReader func(path, ref string) ([]byte, string, error)

type compiler struct {
	read   fixtureReader
	index  map[string]*route // by pattern, merged scenarios keep declaration order
	routes []*route
}

func Compile(docs []*restfile.Document) (*Handler, error) {
	return compile(docs, rejectFixture)
}

func compile(docs []*restfile.Document, read fixtureReader) (*Handler, error) {
	c := &compiler{read: read, index: make(map[string]*route)}
	for _, doc := range sortDocs(docs) {
		if err := c.addDoc(doc); err != nil {
			return nil, err
		}
	}
	return c.handler()
}

func (c *compiler) addDoc(doc *restfile.Document) error {
	if err := docError(doc); err != nil {
		return err
	}
	for _, spec := range doc.Mocks {
		if spec == nil {
			continue
		}
		if err := c.addMock(doc, spec); err != nil {
			return err
		}
	}
	return nil
}

func (c *compiler) addMock(doc *restfile.Document, spec *restfile.Mock) error {
	src := loc{doc.Path, spec.LineRange.Start}
	pat, err := restfile.CompileMockPathPattern(spec.Path)
	if err != nil {
		return fmt.Errorf("%s: %w", src, err)
	}
	if spec.Method == "" || !httpguts.ValidHeaderFieldName(spec.Method) {
		return fmt.Errorf("%s: invalid mock method %q", src, spec.Method)
	}

	v, err := c.newVariant(doc.Path, spec, src)
	if err != nil {
		return fmt.Errorf("%s: %w", src, err)
	}
	key := spec.Method + " " + pat
	if rt := c.index[key]; rt != nil {
		rt.variants = append(rt.variants, v)
		return nil
	}
	rt := &route{
		method:   spec.Method,
		path:     spec.Path,
		pattern:  key,
		label:    spec.Method + " " + spec.Path,
		variants: []*variant{v},
	}
	c.index[key] = rt
	c.routes = append(c.routes, rt)
	return nil
}

func (c *compiler) newVariant(path string, spec *restfile.Mock, src loc) (*variant, error) {
	switch {
	case !restfile.ValidMockStatus(spec.Response.Status):
		return nil, fmt.Errorf("mock response status must be between 200 and 599")
	case spec.Latency < 0:
		return nil, fmt.Errorf("mock latency cannot be negative")
	case spec.Name != "" && !restfile.ValidMockName(spec.Name):
		return nil, fmt.Errorf("invalid mock scenario name")
	case spec.Default && spec.Match.HasConditions():
		return nil, fmt.Errorf("default mock scenario cannot have @match conditions")
	}
	if err := checkMatch(spec.Match); err != nil {
		return nil, err
	}

	headers, err := respHeaders(spec.Response.Headers)
	if err != nil {
		return nil, err
	}
	body, fixture := []byte(spec.Response.Body.Text), ""
	if ref := spec.Response.Body.FilePath; ref != "" {
		if body, fixture, err = c.read(path, ref); err != nil {
			return nil, err
		}
	}
	if !restfile.ResponseAllowsBody(spec.Response.Status) && len(body) > 0 {
		return nil, fmt.Errorf("status %d cannot have a response body", spec.Response.Status)
	}

	ms, err := newMatchers(spec.Match)
	if err != nil {
		return nil, err
	}

	return &variant{
		name:     spec.Name,
		def:      spec.Default,
		latency:  spec.Latency,
		match:    spec.Match,
		matchers: ms,
		status:   spec.Response.Status,
		headers:  headers,
		body:     body,
		fixture:  fixture,
		src:      src,
	}, nil
}

func (c *compiler) handler() (*Handler, error) {
	mux := http.NewServeMux()
	methods := make([]string, 0, len(c.routes))
	var fixtures []string
	scenarios := 0
	for _, rt := range c.routes {
		if err := rt.validate(); err != nil {
			return nil, err
		}
		if err := rt.register(mux); err != nil {
			return nil, fmt.Errorf("%s: %w", rt.variants[0].src, err)
		}
		scenarios += len(rt.variants)
		if !slices.Contains(methods, rt.method) {
			methods = append(methods, rt.method)
		}
		for _, v := range rt.variants {
			if v.fixture != "" && !slices.Contains(fixtures, v.fixture) {
				fixtures = append(fixtures, v.fixture)
			}
		}
	}

	return &Handler{
		mux:       mux,
		routes:    len(c.routes),
		scenarios: scenarios,
		digest:    digest(c.routes),
		methods:   methods,
		fixtures:  fixtures,
	}, nil
}

func sortDocs(docs []*restfile.Document) []*restfile.Document {
	sorted := slices.DeleteFunc(slices.Clone(docs), func(doc *restfile.Document) bool {
		return doc == nil
	})
	slices.SortStableFunc(sorted, func(a, b *restfile.Document) int {
		return cmp.Compare(a.Path, b.Path)
	})
	return sorted
}

func docError(doc *restfile.Document) error {
	for _, e := range doc.Errors {
		if !e.Mock && !inMocks(doc.Mocks, e.Line) {
			continue
		}
		return fmt.Errorf("%s: %s", loc{doc.Path, e.Line}, e.Message)
	}
	return nil
}

func inMocks(mocks []*restfile.Mock, line int) bool {
	for _, m := range mocks {
		if m != nil && line >= m.LineRange.Start && line <= m.LineRange.End {
			return true
		}
	}
	return false
}

func checkMatch(m restfile.MockMatch) error {
	for name := range m.Query {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("mock query matcher name cannot be empty")
		}
	}
	for name, values := range m.Headers {
		if !httpguts.ValidHeaderFieldName(name) {
			return fmt.Errorf("invalid mock header matcher %q", name)
		}
		if isSelectorHeader(name) {
			return fmt.Errorf("mock selector header %q cannot be used as a matcher", name)
		}
		for _, val := range values {
			if !httpguts.ValidHeaderFieldValue(val) {
				return fmt.Errorf("invalid value for mock header matcher %q", name)
			}
		}
	}
	return nil
}

func respHeaders(src http.Header) (http.Header, error) {
	headers := make(http.Header, len(src))
	for name, values := range src {
		if !httpguts.ValidHeaderFieldName(name) {
			return nil, fmt.Errorf("invalid response header %q", name)
		}
		if restfile.IsManagedMockResponseHeader(name) {
			return nil, fmt.Errorf("response header %q is managed by the HTTP server", name)
		}
		for _, val := range values {
			if !httpguts.ValidHeaderFieldValue(val) {
				return nil, fmt.Errorf("invalid value for response header %q", name)
			}
			headers.Add(name, val)
		}
	}
	return headers, nil
}

func rejectFixture(_, ref string) ([]byte, string, error) {
	return nil, "", fmt.Errorf("mock response body file %q requires loading from a mock source", ref)
}

func (rt *route) validate() error {
	names := make(map[string]loc)
	var def *loc
	for _, v := range rt.variants {
		if prev, ok := names[v.name]; v.name != "" && ok {
			return fmt.Errorf("%s: mock scenario name %q is already used at %s", v.src, v.name, prev)
		} else if v.name != "" {
			names[v.name] = v.src
		}
		if !v.def {
			continue
		}
		if def != nil {
			return fmt.Errorf("%s: mock route already has a default at %s", v.src, *def)
		}
		src := v.src
		def = &src
	}
	return nil
}

func (rt *route) register(mux *http.ServeMux) (err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("conflicting or invalid mock route %s: %v", rt.pattern, v)
		}
	}()
	mux.HandleFunc(rt.pattern, rt.serveHTTP)
	return nil
}

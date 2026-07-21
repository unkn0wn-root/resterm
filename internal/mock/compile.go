package mock

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"golang.org/x/net/http/httpguts"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type fixtureReader func(path, ref string) ([]byte, string, error)

type compiler struct {
	read         fixtureReader
	index        map[string]*route // by pattern, merged scenarios keep declaration order
	routes       []*route
	expectations []Expectation
}

func Compile(docs []*restfile.Document) (*Handler, error) {
	return compile(docs, rejectFixture)
}

func compile(docs []*restfile.Document, read fixtureReader) (*Handler, error) {
	c := &compiler{read: read, index: make(map[string]*route)}
	docs = sortDocs(docs)
	for _, doc := range docs {
		if err := c.addDoc(doc); err != nil {
			return nil, err
		}
	}
	h, err := c.handler()
	if err != nil {
		return nil, err
	}
	h.digest = digest(docs, c.routes)
	return h, nil
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
	pat, params, err := restfile.CompileMockPath(spec.Path)
	if err != nil {
		return fmt.Errorf("%s: %w", src, err)
	}
	if strings.HasPrefix(pat, controlNamespace) {
		return fmt.Errorf("%s: mock path is inside the reserved %s namespace", src, controlNamespace)
	}
	if spec.Method == "" || !httpguts.ValidHeaderFieldName(spec.Method) {
		return fmt.Errorf("%s: invalid mock method %q", src, spec.Method)
	}

	v, err := c.newVariant(doc.Path, spec, params, src)
	if err != nil {
		return fmt.Errorf("%s: %w", src, err)
	}
	if spec.Expectation != nil {
		expectation, err := compileExpectation(doc.Path, spec)
		if err != nil {
			return fmt.Errorf("%s: %w", src, err)
		}
		c.expectations = append(c.expectations, expectation)
	}
	key := spec.Method + " " + pat
	if rt := c.index[key]; rt != nil {
		rt.variants = append(rt.variants, v)
		return nil
	}
	rt := &route{
		method:   spec.Method,
		pattern:  key,
		label:    spec.Method + " " + spec.Path,
		variants: []*variant{v},
	}
	c.index[key] = rt
	c.routes = append(c.routes, rt)
	return nil
}

func (c *compiler) newVariant(
	path string,
	spec *restfile.Mock,
	pathParams map[string]string,
	src loc,
) (*variant, error) {
	if err := spec.CheckShape(); err != nil {
		return nil, err
	}
	switch {
	case spec.Latency < 0:
		return nil, errors.New("mock latency cannot be negative")
	case spec.Name != "" && !restfile.ValidMockName(spec.Name):
		return nil, errors.New("invalid mock scenario name")
	case spec.Sequence != "" && !restfile.ValidMockName(spec.Sequence):
		return nil, errors.New("invalid mock sequence name")
	case spec.Default && spec.Match.HasConditions():
		return nil, errors.New("default mock scenario cannot have @match conditions")
	}
	if err := checkMatch(spec.Match); err != nil {
		return nil, err
	}
	sequenceKey, err := spec.SequenceKey.Check(pathParams)
	if err != nil {
		return nil, fmt.Errorf("mock sequence key: %w", err)
	}

	responses := make([]response, 0, len(spec.Responses))
	for _, specResponse := range spec.Responses {
		resp, err := c.newResponse(path, specResponse)
		if err != nil {
			return nil, err
		}
		if !spec.DisableInterpolation {
			if err := resp.compileTemplates(pathParams); err != nil {
				return nil, err
			}
		}
		responses = append(responses, resp)
	}

	ms, err := newMatchers(spec.Match)
	if err != nil {
		return nil, err
	}

	return &variant{
		name:            cmp.Or(spec.Sequence, spec.Name),
		sequence:        spec.Sequence,
		def:             spec.Default,
		latency:         spec.Latency,
		matchers:        ms,
		responses:       responses,
		pathParams:      pathParams,
		sequenceKeySpec: sequenceKey,
		src:             src,
	}, nil
}

func (c *compiler) newResponse(path string, spec restfile.MockResponse) (response, error) {
	if !restfile.ValidMockStatus(spec.Status) {
		return response{}, fmt.Errorf("mock response status must be between 200 and 599")
	}
	headers, err := respHeaders(spec.Headers)
	if err != nil {
		return response{}, err
	}
	body, fixture := []byte(spec.Body.Text), ""
	if ref := spec.Body.FilePath; ref != "" {
		if body, fixture, err = c.read(path, ref); err != nil {
			return response{}, err
		}
	}
	if !restfile.ResponseAllowsBody(spec.Status) && len(body) > 0 {
		return response{}, fmt.Errorf("status %d cannot have a response body", spec.Status)
	}
	return response{status: spec.Status, headers: headers, body: body, fixture: fixture}, nil
}

func (c *compiler) handler() (*Handler, error) {
	mux := http.NewServeMux()
	methods := make([]string, 0, len(c.routes))
	var fixtures []string
	sequences := make(map[string][]*sequenceCursor)
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
			if v.sequence != "" {
				sequences[v.sequence] = append(sequences[v.sequence], &v.cursor)
			}
			for _, resp := range v.responses {
				if resp.fixture != "" && !slices.Contains(fixtures, resp.fixture) {
					fixtures = append(fixtures, resp.fixture)
				}
			}
		}
	}

	h := &Handler{
		mux:          mux,
		routes:       len(c.routes),
		scenarios:    scenarios,
		methods:      methods,
		fixtures:     fixtures,
		sequences:    sequences,
		expectations: c.expectations,
	}
	h.setSequenceKeyLimit(DefaultSequenceKeyLimit)
	return h, nil
}

func compileExpectation(path string, spec *restfile.Mock) (Expectation, error) {
	pattern, err := compileRequestPattern(RequestPattern{
		Method:  spec.Method,
		Path:    spec.Path,
		Query:   spec.Match.Query,
		Headers: spec.Match.Headers,
		JSON:    spec.Match.JSON,
	})
	if err != nil {
		return Expectation{}, fmt.Errorf("invalid mock expectation: %w", err)
	}
	line := spec.Expectation.Line
	if line <= 0 {
		line = spec.LineRange.Start
	}
	return Expectation{
		Pattern: pattern.pattern,
		Calls:   spec.Expectation.Calls,
		Source:  path,
		Line:    line,
		Title:   spec.Title,
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
		if m == nil {
			continue
		}
		if line >= m.LineRange.Start && line <= m.LineRange.End {
			return true
		}
	}
	return false
}

func checkMatch(m restfile.MockMatch) error {
	if err := checkQueryRules(m.Query); err != nil {
		return err
	}
	for name := range m.Headers {
		if isSelectorHeader(name) {
			return fmt.Errorf("mock selector header %q cannot be used as a matcher", name)
		}
	}
	_, err := canonHeaderRules(m.Headers)
	return err
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
		if v.name != "" {
			if prev, ok := names[v.name]; ok {
				return fmt.Errorf("%s: mock scenario name %q is already used at %s", v.src, v.name, prev)
			}
			names[v.name] = v.src
		}
		if !v.def {
			continue
		}
		if def != nil {
			return fmt.Errorf("%s: mock route already has a default at %s", v.src, *def)
		}
		def = &v.src
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

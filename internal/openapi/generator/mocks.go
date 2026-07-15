package generator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/http/httpguts"

	"github.com/unkn0wn-root/resterm/internal/openapi/model"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/restwriter"
)

type mockCandidate struct {
	mock      *restfile.Mock
	status    int
	mediaRank int
}

// mockWarnf goes through noteWarning so repeated warnings collapse into one entry.
func (b *Builder) mockWarnf(op model.Operation, format string, args ...any) {
	b.noteWarning(fmt.Sprintf("OpenAPI %s %s "+format, append([]any{op.Method, op.Path}, args...)...))
}

func (b *Builder) buildMocks(op model.Operation) []*restfile.Mock {
	if err := restfile.ValidateMockPath(op.Path); err != nil {
		b.mockWarnf(op, "cannot be represented as a mock route and was skipped: %v", err)
		return nil
	}

	var candidates []mockCandidate
	used := make(map[string]struct{})
	for _, resp := range sortedMockResponses(op.Responses) {
		status, ok := concreteStatus(resp.StatusCode)
		if !ok {
			b.mockWarnf(op, "response %q is not a concrete 200-599 status and was skipped for mocks", resp.StatusCode)
			continue
		}
		candidates = append(candidates, b.mockCandidatesForResponse(op, resp, status, used)...)
	}

	if len(candidates) == 0 {
		return nil
	}
	best := 0
	for i := 1; i < len(candidates); i++ {
		if betterDefault(candidates[i], candidates[best]) {
			best = i
		}
	}
	candidates[best].mock.Default = true

	out := make([]*restfile.Mock, len(candidates))
	for i := range candidates {
		out[i] = candidates[i].mock
	}
	return out
}

func sortedMockResponses(responses []model.Response) []model.Response {
	responses = append([]model.Response(nil), responses...)
	sort.SliceStable(responses, func(i, j int) bool {
		l, lok := concreteStatus(responses[i].StatusCode)
		r, rok := concreteStatus(responses[j].StatusCode)
		if lok != rok {
			return lok
		}
		if lok && l != r {
			return l < r
		}
		return responses[i].StatusCode < responses[j].StatusCode
	})
	return responses
}

func sortedMockMediaTypes(mts []model.MediaType) []model.MediaType {
	mts = append([]model.MediaType(nil), mts...)
	sort.SliceStable(mts, func(i, j int) bool {
		lr := mockMediaRank(mts[i].ContentType)
		rr := mockMediaRank(mts[j].ContentType)
		if lr != rr {
			return lr < rr
		}
		return mts[i].ContentType < mts[j].ContentType
	})
	return mts
}

func (b *Builder) mockCandidatesForResponse(
	op model.Operation,
	resp model.Response,
	status int,
	used map[string]struct{},
) []mockCandidate {
	hdrs := b.mockResponseHeaders(op, resp)
	mts := sortedMockMediaTypes(resp.MediaTypes)
	if len(mts) == 0 {
		spec := b.mockForResponse(op, model.MediaType{}, model.Example{}, hdrs, status, false)
		spec.Name = restwriter.UniqueMockName("status-"+strconv.Itoa(status), used)
		return []mockCandidate{{mock: spec, status: status, mediaRank: 99}}
	}

	multi := len(mts) > 1
	var out []mockCandidate
	for _, mt := range mts {
		examples := b.mockExamples(mt)
		if len(examples) == 0 {
			examples = []model.Example{{}}
		}
		for _, ex := range examples {
			spec := b.mockForResponse(op, mt, ex, hdrs, status, multi)
			spec.Name = restwriter.UniqueMockName(mockScenarioName(status, mt.ContentType, ex, multi), used)
			out = append(out, mockCandidate{
				mock:      spec,
				status:    status,
				mediaRank: mockMediaRank(mt.ContentType),
			})
		}
	}
	return out
}

func (b *Builder) mockExamples(media model.MediaType) []model.Example {
	if len(media.Examples) > 0 {
		return media.Examples
	}
	if value, ok := b.samples.sample(media.Schema, sampleResponse); ok {
		return []model.Example{{Value: value, Source: model.ExampleFromSchema, HasValue: true}}
	}
	return nil
}

func (b *Builder) mockForResponse(
	op model.Operation,
	media model.MediaType,
	example model.Example,
	hdrs http.Header,
	status int,
	multi bool,
) *restfile.Mock {
	body, ct := b.mockBodyFor(op, media, example, status)
	headers := hdrs.Clone()
	if ct != "" {
		headers.Set("Content-Type", ct)
	}
	return &restfile.Mock{
		Title:  mockTitle(op, status, example, ct, multi),
		Method: string(op.Method),
		Path:   op.Path,
		Response: restfile.MockResponse{
			Status:  status,
			Headers: headers,
			Body:    restfile.BodySource{Text: body, MimeType: ct},
		},
	}
}

// mockBodyFor warns on an unsafe content type even when the body is later omitted.
func (b *Builder) mockBodyFor(
	op model.Operation,
	media model.MediaType,
	example model.Example,
	status int,
) (string, string) {
	ct, ok := safeContentType(media.ContentType)
	if !ok {
		b.mockWarnf(
			op,
			"response %d has an invalid content type %q; Content-Type was omitted",
			status,
			media.ContentType,
		)
	}
	body := ""
	if restfile.ResponseAllowsBody(status) && example.HasValue {
		var err error
		body, err = mockExampleBody(ct, example)
		if err != nil {
			b.mockWarnf(op, "response %d %s example %q was omitted: %v", status, ct, example.Name, err)
		}
	}
	if body == "" {
		ct = ""
	}
	return body, ct
}

func (b *Builder) mockResponseHeaders(op model.Operation, resp model.Response) http.Header {
	headers := make(http.Header)
	for _, h := range resp.Headers {
		name := strings.TrimSpace(h.Name)
		if !httpguts.ValidHeaderFieldName(name) {
			b.mockWarnf(op, "response header %q has an invalid name and was skipped", name)
			continue
		}
		if restfile.IsManagedMockResponseHeader(name) {
			b.mockWarnf(op, "response header %q is managed by the mock server and was skipped", name)
			continue
		}
		value := h.Example.Value
		ok := h.Example.HasValue
		if !ok {
			value, ok = b.samples.sample(h.Schema, sampleResponse)
		}
		if !ok {
			continue
		}
		v := mockHeaderValue(value)
		if !httpguts.ValidHeaderFieldValue(v) {
			b.mockWarnf(op, "response header %q example has an invalid value and was skipped", name)
			continue
		}
		headers.Set(name, v)
	}
	return headers
}

func concreteStatus(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if len(raw) != 3 {
		return 0, false
	}
	status, err := strconv.Atoi(raw)
	return status, err == nil && restfile.ValidMockStatus(status)
}

func mockExampleBody(contentType string, example model.Example) (string, error) {
	if !example.Serialized {
		return mockBody(contentType, example.Value)
	}
	body, ok := example.Value.(string)
	if !ok {
		return "", fmt.Errorf("serialized example has type %T, want string", example.Value)
	}
	return checkedMockBody(body)
}

func mockBody(contentType string, value any) (string, error) {
	mt := baseMediaType(contentType)
	if isBinaryMedia(mt) {
		return "", fmt.Errorf("binary media type %s requires an external body file", mt)
	}
	if isJSONMedia(mt) {
		return jsonMockBody(value)
	}
	switch v := value.(type) {
	case string:
		return checkedMockBody(v)
	case []byte:
		return checkedMockBody(string(v))
	case nil:
		return "", nil
	case bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64,
		json.Number:
		return checkedMockBody(fmt.Sprint(v))
	default:
		if mt != "" {
			return "", fmt.Errorf("cannot encode %T as %s", value, mt)
		}
		return jsonMockBody(value)
	}
}

func jsonMockBody(value any) (string, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return checkedMockBody(string(data))
}

func checkedMockBody(body string) (string, error) {
	return restwriter.NormalizeMockBody(body)
}

func checkedBodyText(body string) (string, error) {
	return restwriter.NormalizeInlineBody(body)
}

func mockHeaderValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []string:
		return strings.Join(v, ",")
	case []any:
		parts := make([]string, len(v))
		for i := range v {
			parts[i] = fmt.Sprint(v[i])
		}
		return strings.Join(parts, ",")
	case map[string]string, map[string]any:
		return joinObjectKeyValueList(ensureStringMap(value), ",")
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func mockScenarioName(status int, contentType string, example model.Example, multi bool) string {
	parts := []string{"status", strconv.Itoa(status)}
	if multi {
		parts = append(parts, restwriter.MockNameSlug(contentType))
	}
	if example.Name != "" {
		parts = append(parts, restwriter.MockNameSlug(example.Name))
	}
	return strings.Join(parts, "-")
}

func mockTitle(op model.Operation, status int, example model.Example, contentType string, multi bool) string {
	base := singleLine(op.Summary)
	if base == "" {
		base = singleLine(op.ID)
	}
	if base == "" {
		base = fmt.Sprintf("%s %s", op.Method, op.Path)
	}
	parts := []string{base, strconv.Itoa(status)}
	if text := http.StatusText(status); text != "" {
		parts = append(parts, text)
	}
	if name := singleLine(example.Name); name != "" {
		parts = append(parts, "-", name)
	} else if summary := singleLine(example.Summary); summary != "" {
		parts = append(parts, "-", summary)
	} else if multi && contentType != "" {
		parts = append(parts, "-", contentType)
	}
	return strings.Join(parts, " ")
}

func singleLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func mockMediaRank(contentType string) int {
	media := baseMediaType(contentType)
	switch {
	case media == "application/json":
		return 0
	case strings.HasSuffix(media, "+json"):
		return 1
	case strings.HasPrefix(media, "text/"):
		return 2
	default:
		return 3
	}
}

func betterDefault(l, r mockCandidate) bool {
	ls := l.status >= 200 && l.status < 300
	rs := r.status >= 200 && r.status < 300
	if ls != rs {
		return ls
	}
	if l.status != r.status {
		return l.status < r.status
	}
	if l.mediaRank != r.mediaRank {
		return l.mediaRank < r.mediaRank
	}
	return false
}

func isBinaryMedia(media string) bool {
	category, _, _ := strings.Cut(media, "/")
	switch category {
	case "audio", "font", "image", "video":
		return true
	}
	switch media {
	case "application/gzip",
		"application/octet-stream",
		"application/pdf",
		"application/vnd.rar",
		"application/x-7z-compressed",
		"application/x-rar-compressed",
		"application/x-tar",
		"application/zip":
		return true
	default:
		return strings.HasSuffix(media, "+zip")
	}
}

package mock

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestResponseInterpolation(t *testing.T) {
	handler := compileSource(t, `# @mock method=POST path=/users/{id}/{rest...}
HTTP/1.1 200 OK
Content-Type: application/json
X-Resolved-Tenant: {{headers.X-Tenant}}

{"id":{{json.path.id}},"rest":{{json.path.rest}},"tag":{{json.query.tag}},"tenant":{{json.headers.X-Tenant}},"host":{{json.headers.Host}},"name":{{json.body.user.name}},"count":{{json.body.count}},"large":{{json.body.large}},"active":{{json.body.active}},"none":{{json.body.none}},"meta":{{json.body.meta}},"items":{{json.body.items}},"uuid":"{{$uuid}}","timestamp":"{{$timestampISO8601}}"}`)

	req := httptest.NewRequest(
		http.MethodPost,
		"http://mock.example/users/user%22one/files/a/b?tag=first%20%22quoted%22&tag=second",
		strings.NewReader(
			`{"user":{"name":"Ada \"admin\"\nline"},"count":3,"large":9007199254740993,"active":true,"none":null,"meta":{"role":"admin"},"items":[1,"two"]}`,
		),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Add("X-Tenant", `acme\west "admin"`)
	req.Header.Add("X-Tenant", "ignored")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"large":9007199254740993`) {
		t.Fatalf("large integer was not preserved: %s", response.Body.String())
	}
	if got := response.Header().Get("X-Resolved-Tenant"); got != `acme\west "admin"` {
		t.Fatalf("resolved header = %q", got)
	}

	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v\n%s", err, response.Body.String())
	}
	for key, want := range map[string]string{
		"id":     `user"one`,
		"rest":   "files/a/b",
		"tag":    `first "quoted"`,
		"tenant": `acme\west "admin"`,
		"host":   "mock.example",
		"name":   "Ada \"admin\"\nline",
	} {
		if got := body[key]; got != want {
			t.Fatalf("body[%q] = %#v, want %q", key, got, want)
		}
	}
	if body["count"] != float64(3) || body["active"] != true || body["none"] != nil {
		t.Fatalf("JSON scalar interpolation = %#v", body)
	}
	if meta, ok := body["meta"].(map[string]any); !ok || meta["role"] != "admin" {
		t.Fatalf("meta = %#v", body["meta"])
	}
	if items, ok := body["items"].([]any); !ok || len(items) != 2 || items[1] != "two" {
		t.Fatalf("items = %#v", body["items"])
	}
	generatedUUID, ok := body["uuid"].(string)
	if !ok {
		t.Fatalf("uuid = %#v", body["uuid"])
	}
	if _, err := uuid.Parse(generatedUUID); err != nil {
		t.Fatalf("uuid = %q: %v", generatedUUID, err)
	}
	generatedTimestamp, ok := body["timestamp"].(string)
	if !ok {
		t.Fatalf("timestamp = %#v", body["timestamp"])
	}
	if _, err := time.Parse(time.RFC3339, generatedTimestamp); err != nil {
		t.Fatalf("timestamp = %q: %v", generatedTimestamp, err)
	}
}

func TestJSONResponseInterpolationPreventsStructureInjection(t *testing.T) {
	handler := compileSource(t, `# @mock method=POST path=/x
HTTP/1.1 200 OK
Content-Type: application/json

{"name":{{json.body.name}}}`)

	const name = `Ada","admin":true,"ignored":"`
	req := httptest.NewRequest(
		http.MethodPost,
		"/x",
		strings.NewReader(`{"name":"Ada\",\"admin\":true,\"ignored\":\""}`),
	)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v\n%s", err, response.Body.String())
	}
	if len(body) != 1 || body["name"] != name {
		t.Fatalf("response body = %#v, want one name field containing %q", body, name)
	}
}

func TestJSONInterpolationProducesSafeHeaderValue(t *testing.T) {
	handler := compileSource(t, `# @mock method=GET path=/x
HTTP/1.1 200 OK
X-Encoded: {{json.query.value}}`)
	response := httptest.NewRecorder()
	handler.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/x?value=line%0D%0Abreak", nil),
	)
	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("X-Encoded"); got != `"line\r\nbreak"` {
		t.Fatalf("X-Encoded = %q", got)
	}
}

func TestResponseTextInterpolationRemainsRaw(t *testing.T) {
	handler := compileSource(t, `# @mock method=POST path=/x
HTTP/1.1 200 OK

Hello {{body.name}}`)
	req := httptest.NewRequest(
		http.MethodPost,
		"/x",
		strings.NewReader(`{"name":"Ada \"admin\"\nline"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	assertResponse(t, handler, req, http.StatusOK, "Hello Ada \"admin\"\nline")
}

func TestResponseInterpolationUsesVariantPathNames(t *testing.T) {
	handler := compileSource(t, `# @mock method=GET path=/users/{userID} name=first
HTTP/1.1 200 OK

{{path.userID}}
### Second
# @mock method=GET path=/users/{id} name=second
HTTP/1.1 200 OK

{{path.id}}`)

	for _, name := range []string{"first", "second"} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/users/123", nil)
			req.Header.Set(selectorNameHeader, name)
			assertResponse(t, handler, req, http.StatusOK, "123")
		})
	}
}

func TestResponseInterpolationFailures(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		request func() *http.Request
		status  int
		detail  string
	}{
		{
			name:    "missing query",
			source:  "# @mock method=GET path=/x\nHTTP/1.1 200 OK\n\n{{query.page}}",
			request: func() *http.Request { return httptest.NewRequest(http.MethodGet, "/x", nil) },
			status:  http.StatusBadRequest,
			detail:  "missing query value",
		},
		{
			name:    "missing JSON-encoded query",
			source:  "# @mock method=GET path=/x\nHTTP/1.1 200 OK\n\n{{json.query.page}}",
			request: func() *http.Request { return httptest.NewRequest(http.MethodGet, "/x", nil) },
			status:  http.StatusBadRequest,
			detail:  "missing query value",
		},
		{
			name:    "missing header",
			source:  "# @mock method=GET path=/x\nHTTP/1.1 200 OK\n\n{{headers.X-Tenant}}",
			request: func() *http.Request { return httptest.NewRequest(http.MethodGet, "/x", nil) },
			status:  http.StatusBadRequest,
			detail:  "missing request header",
		},
		{
			name:   "missing JSON field",
			source: "# @mock method=POST path=/x\nHTTP/1.1 200 OK\n\n{{body.user.id}}",
			request: func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"user":{}}`))
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			status: http.StatusBadRequest,
			detail: "missing JSON body field",
		},
		{
			name:   "invalid JSON",
			source: "# @mock method=POST path=/x\nHTTP/1.1 200 OK\n\n{{body.user.id}}",
			request: func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("{"))
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			status: http.StatusBadRequest,
			detail: "invalid JSON request body",
		},
		{
			name:   "multiple JSON values",
			source: "# @mock method=POST path=/x\nHTTP/1.1 200 OK\n\n{{body.value}}",
			request: func() *http.Request {
				req := httptest.NewRequest(
					http.MethodPost,
					"/x",
					strings.NewReader(`{"value":1} {"value":2}`),
				)
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			status: http.StatusBadRequest,
			detail: "invalid JSON request body",
		},
		{
			name:   "non-JSON body",
			source: "# @mock method=POST path=/x\nHTTP/1.1 200 OK\n\n{{body.user.id}}",
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("id=1"))
			},
			status: http.StatusBadRequest,
			detail: "must have a JSON body",
		},
		{
			name:   "oversized JSON",
			source: "# @mock method=POST path=/x\nHTTP/1.1 200 OK\n\n{{body.value}}",
			request: func() *http.Request {
				req := httptest.NewRequest(
					http.MethodPost,
					"/x",
					strings.NewReader(`{"value":"`+strings.Repeat("x", maxMockRequestBody)+`"}`),
				)
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			status: http.StatusRequestEntityTooLarge,
			detail: "exceeds 4 MiB",
		},
		{
			name:   "invalid rendered header",
			source: "# @mock method=GET path=/x\nHTTP/1.1 200 OK\nX-Value: {{query.value}}",
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/x?value=line%0D%0Abreak", nil)
			},
			status: http.StatusInternalServerError,
			detail: "invalid value for header",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := compileSource(t, test.source)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, test.request())
			if response.Code != test.status || !strings.Contains(response.Body.String(), test.detail) {
				t.Fatalf(
					"response = %d %q, want %d containing %q",
					response.Code,
					response.Body.String(),
					test.status,
					test.detail,
				)
			}
			if got := response.Header().Get("Content-Type"); got != "application/problem+json" {
				t.Fatalf("Content-Type = %q", got)
			}
		})
	}
}

func TestHeadInterpolatesBodyLength(t *testing.T) {
	handler := compileSource(t, `# @mock method=GET path=/x
HTTP/1.1 200 OK

hello {{query.name}}`)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodHead, "/x?name=world", nil))
	if response.Code != http.StatusOK || response.Body.Len() != 0 {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Length"); got != strconv.Itoa(len("hello world")) {
		t.Fatalf("Content-Length = %q", got)
	}
}

func TestHeadCalculatesJSONEncodedBodyLength(t *testing.T) {
	handler := compileSource(t, `# @mock method=GET path=/x
HTTP/1.1 200 OK

{{json.query.name}}`)
	response := httptest.NewRecorder()
	handler.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodHead, "/x?name=Ada%20%22admin%22", nil),
	)
	if response.Code != http.StatusOK || response.Body.Len() != 0 {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
	if got, want := response.Header().Get("Content-Length"), strconv.Itoa(len(`"Ada \"admin\""`)); got != want {
		t.Fatalf("Content-Length = %q, want %q", got, want)
	}
}

func TestResponseInterpolationCanBeDisabled(t *testing.T) {
	handler := compileSource(t, `# @mock method=GET path=/x interpolate=false
HTTP/1.1 200 OK
X-Literal: {{json.query.value}}

{{unterminated`)
	req := httptest.NewRequest(http.MethodGet, "/x?value=expanded", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK || response.Header().Get("X-Literal") != "{{json.query.value}}" ||
		response.Body.String() != "{{unterminated" {
		t.Fatalf("response = %d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
	}
}

func TestResponseInterpolationServesUnterminatedPlaceholderLiterally(t *testing.T) {
	handler := compileSource(t, `# @mock method=GET path=/x
HTTP/1.1 200 OK

{{query.v}} {{oops`)
	req := httptest.NewRequest(http.MethodGet, "/x?v=expanded", nil)
	assertResponse(t, handler, req, http.StatusOK, "expanded {{oops")
}

func TestCompileRejectsInvalidResponseTemplates(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "unknown namespace", body: "{{env.token}}", want: "namespace"},
		{name: "unknown path parameter", body: "{{path.other}}", want: "no parameter"},
		{name: "empty JSON modifier", body: "{{json.}}", want: "unsupported"},
		{name: "unknown JSON source", body: "{{json.env.token}}", want: "namespace"},
		{name: "JSON dynamic helper", body: "{{json.$uuid}}", want: "unsupported"},
		{name: "unknown encoded path parameter", body: "{{json.path.other}}", want: "no parameter"},
		{name: "expression", body: "{{= 1 + 1}}", want: "do not support expressions"},
		{name: "invalid body path", body: "{{body.items[nope]}}", want: "invalid JSON body"},
		{name: "invalid encoded body path", body: "{{json.body.items[nope]}}", want: "invalid JSON body"},
		{name: "unsupported dynamic", body: "{{$uuid + 1s}}", want: "unsupported dynamic"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := "# @mock method=GET path=/users/{id}\nHTTP/1.1 200 OK\n\n" + test.body
			_, err := Compile([]*restfile.Document{parser.Parse("bad.http", []byte(source))})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Compile() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestFixtureResponseInterpolation(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "body.json"), `{"id":{{json.path.id}}}`)
	writeFile(t, filepath.Join(root, "mocks.http"), `# @mock method=GET path=/users/{id}
HTTP/1.1 200 OK
Content-Type: application/json

< ./body.json`)
	handler, err := Load(root, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertResponse(
		t,
		handler,
		httptest.NewRequest(http.MethodGet, "/users/42", nil),
		http.StatusOK,
		`{"id":"42"}`,
	)
}

func TestInterpolationReusesJSONBodyUsedForMatching(t *testing.T) {
	handler := compileSource(t, `# @mock method=POST path=/x
# @match json={"kind":"matched"}
HTTP/1.1 200 OK

{{body.value}}`)
	req := httptest.NewRequest(
		http.MethodPost,
		"/x",
		strings.NewReader(`{"kind":"matched","value":"same body"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	assertResponse(t, handler, req, http.StatusOK, "same body")
}

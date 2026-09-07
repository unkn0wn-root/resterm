package recorder

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/mock"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestRecordForwardScrubAndExport(t *testing.T) {
	const secret = "top-secret-value"
	const payload = `{"name":"Ada","token":"top-secret-value","n":9007199254740993}`
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		if string(data) != payload || r.Header.Get("Authorization") != "Bearer "+secret ||
			r.Header.Get("Cookie") != "session="+secret ||
			r.URL.RequestURI() != "/users%2f42?q=a%20b&q=a+b&token="+secret {
			t.Errorf("wire request changed: %s %s %v", data, r.URL, r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "session="+secret+"; Path=/; SameSite=Lax")
		w.Header().Set("Location", "/next?token="+secret)
		w.WriteHeader(302)
		_, _ = io.WriteString(w, `{"echo":"`+secret+`","ok":true}`)
	}))
	defer up.Close()
	s := startTest(t, up.URL, nil)
	req, _ := http.NewRequestWithContext(
		t.Context(),
		"POST",
		"http://"+s.Addr()+"/users%2f42?q=a%20b&q=a+b&token="+secret,
		strings.NewReader(payload),
	)
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Cookie", "session="+secret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connection", "X-Hop")
	req.Header.Set("X-Hop", "dropped")
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 302 || res.Header.Get("Location") != "/next?token="+secret ||
		!bytes.Contains(data, []byte(secret)) || res.Header.Get("Set-Cookie") == "" {
		t.Fatal("live response was scrubbed")
	}
	e := entriesAfter(t, s, 1)[0]
	if strings.Contains(e.URL, secret) || bytes.Contains(e.Request.Body, []byte(secret)) ||
		strings.Contains(fmt.Sprintf("%v%v", e.Request.Headers, e.Response.Headers), secret) {
		t.Fatalf("named credential retained: %+v", e)
	}
	if e.Request.Headers.Get("Authorization") != redacted || e.Request.Headers.Get("Cookie") != "" ||
		e.Request.Headers.Get("X-Hop") != "" || e.Response.Headers.Get("Location") != "/next?token=REDACTED" {
		t.Fatalf("recorded headers: %v %v", e.Request.Headers, e.Response.Headers)
	}
	// The echo field is not a known secret field, so its value is preserved.
	if !bytes.Contains(e.Response.Body, []byte(secret)) {
		t.Fatalf("response body was rewritten: %s", e.Response.Body)
	}
	if !bytes.Contains(e.Request.Body, []byte("9007199254740993")) {
		t.Fatal("JSON number lost precision")
	}
	if bytes.Contains(e.Request.MatchJSON, []byte("token")) {
		t.Fatal("secret became a matcher")
	}
	out := newTestOutput(t)
	p := buildExport(t, []Entry{e}, ExportOptions{Mode: Both, Path: out.path, Upstream: up.URL})
	if len(p.Excluded) != 0 {
		t.Fatalf("exclusions: %+v", p.Excluded)
	}
	if err := out.Append(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	doc := readTestOutput(t, out)
	if len(doc.Requests) != 1 || len(doc.Mocks) != 1 {
		t.Fatalf("wrong export counts: %+v", doc)
	}
	h, err := mock.Load(mock.Sources{Path: out.path}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(
		"POST",
		"/users%2f42?q=a+b&q=a%20b",
		strings.NewReader(`{"name":"Ada","token":"different","n":9007199254740993}`),
	)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 302 || w.Body.String() != string(e.Response.Body) {
		t.Fatalf("mock response: %d %s", w.Code, w.Body.String())
	}
}

func TestLiteralFixturesAndNamedResponses(t *testing.T) {
	const body = "  \r\n### injected\r\n# @auth command argv=bad\r\n{{= bad() }}\r\n@/private/key\r\n"
	entries := []Entry{textEntry(1, "/x", body), textEntry(2, "/x", body)}
	entries[1].Status = 201
	out := newTestOutput(t)
	p := buildExport(t, entries, ExportOptions{Mode: Both, Path: out.path})
	if len(p.Fixtures) != 4 || len(p.Shadowed) != 1 || p.Shadowed[0] != (Shadow{ID: 2, By: 1}) {
		t.Fatalf("fixtures or shadow warning missing: %+v", p)
	}
	if err := out.Append(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	doc := readTestOutput(t, out)
	if len(doc.Requests) != 2 || len(doc.Mocks) != 2 {
		t.Fatalf("wrong export counts: %+v", doc)
	}
	for _, r := range doc.Requests {
		if r.Body.FilePath == "" || r.Body.Options.ExpandTemplates || len(r.Metadata.Scripts) != 0 {
			t.Fatal("literal body was parsed as request syntax")
		}
	}
	echo := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.Copy(w, r.Body) }),
	)
	defer echo.Close()
	for _, request := range doc.Requests {
		request.URL = echo.URL
		response, err := httpx.NewClient(nil).
			Execute(t.Context(), request, vars.NewResolver(), httpx.Options{BaseDir: filepath.Dir(out.path)})
		if err != nil {
			t.Fatal(err)
		}
		if string(response.Body) != body {
			t.Fatalf("literal fixture executed or changed: %q", response.Body)
		}
	}
	h, err := mock.Load(mock.Sources{Path: out.path}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, m := range doc.Mocks {
		r := httptest.NewRequest("POST", "/x", nil)
		r.Header.Set("X-Resterm-Mock", m.Name)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200+i || w.Body.String() != body {
			t.Fatalf("literal mock: %d %q", w.Code, w.Body.String())
		}
	}
	entries[0].Request.MatchJSON = []byte(`{"a":1}`)
	entries[1].Request.MatchJSON = []byte(`{"a":2}`)
	distinct := buildExport(t, entries, ExportOptions{Mode: Mocks})
	if len(distinct.Document.Mocks) != 2 || len(distinct.Shadowed) != 0 {
		t.Fatalf("distinct matchers reported as shadowed: %+v", distinct)
	}
}

func TestExportExcludesOnlyUnsupportedOutput(t *testing.T) {
	big := `{"pad":"` + strings.Repeat("x", maxMatchBytes) + `"}`
	for _, tt := range []struct {
		method, path, match string
		requests, mocks     int
	}{
		{"GET", "/.resterm/mock/v1/state", "", 1, 0},
		{"GET", "/a//b", "", 1, 0},
		{"GET", "/a/%2e%2e/b", "", 1, 0},
		{"CUSTOM", "/ok", "", 0, 1},
		{"get", "/ok", "", 0, 0},
		{"POST", "/big", big, 1, 0},
	} {
		e := textEntry(1, tt.path, tt.match)
		e.Method, e.Request.MatchJSON = tt.method, []byte(tt.match)
		p := buildExport(t, []Entry{e, textEntry(2, "/valid", "ok")}, ExportOptions{Mode: Both})
		if len(p.Document.Requests) != tt.requests+1 || len(p.Document.Mocks) != tt.mocks+1 ||
			len(p.Excluded) != 2-tt.requests-tt.mocks {
			t.Fatalf("%s %s: wrong export counts: %+v", tt.method, tt.path, p)
		}
		for _, x := range p.Excluded {
			if x.ID != 1 || x.Reason == "" || (tt.match != "" && x.Reason != issueMockMatchLimit) {
				t.Fatalf("wrong exclusion: %+v", x)
			}
		}
		if len(p.Exported) == 0 || p.Exported[len(p.Exported)-1].ID != 2 {
			t.Fatal("valid companion recording was not exported")
		}
	}
}

func TestBaseVariableDoesNotShadowExistingBindings(t *testing.T) {
	const other = "http://other.invalid"
	for _, tt := range []struct {
		name     string
		existing *restfile.Document
		want     string
		declares bool
	}{
		{name: "empty", want: baseName, declares: true},
		{
			name:     "other case",
			existing: &restfile.Document{Variables: []restfile.Variable{{Name: "RECORDEDBASEURL", Value: other}}},
			want:     baseName + "2",
			declares: true,
		},
		{
			name:     "constant",
			existing: &restfile.Document{Constants: []restfile.Constant{{Name: baseName, Value: other}}},
			want:     baseName + "2",
			declares: true,
		},
		{
			name: "constant shadows matching file variable",
			existing: &restfile.Document{
				Variables: []restfile.Variable{{Name: baseName, Value: "http://example.invalid"}},
				Constants: []restfile.Constant{{Name: baseName, Value: other}},
			},
			want:     baseName + "2",
			declares: true,
		},
		{
			name:     "reuses the upstream binding",
			existing: &restfile.Document{Constants: []restfile.Constant{{Name: "RecordedBaseUrl", Value: "http://example.invalid"}}},
			want:     baseName,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := buildExport(t, []Entry{textEntry(1, "/x", "ok")}, ExportOptions{Mode: Requests, Existing: tt.existing})
			if url := p.Document.Requests[0].URL; url != "{{"+tt.want+"}}/x" {
				t.Fatalf("request URL = %q, want base %q", url, tt.want)
			}
			if declared := len(p.Document.Variables) == 1; declared != tt.declares {
				t.Fatalf("declared variables: %+v", p.Document.Variables)
			}
			if tt.declares && p.Document.Variables[0].Name != tt.want {
				t.Fatalf("declared %q, want %q", p.Document.Variables[0].Name, tt.want)
			}
		})
	}
}

func TestAppendKeepsHeaderMatchedMocks(t *testing.T) {
	const existing = "### tenant\n# @mock method=GET path=/existing name=tenant\n" +
		"# @match headers={\"X-Tenant\":\"demo\"}\nHTTP/1.1 200 OK\nContent-Type: text/plain\n\noriginal\n"
	p := buildExport(t, []Entry{textEntry(1, "/new", "new")}, ExportOptions{Mode: Requests})
	text, err := AppendText("buffer.http", existing, p.Text)
	if err != nil {
		t.Fatal(err)
	}
	doc := parser.Parse("buffer.http", []byte(text))
	if err := parser.Check(doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Mocks) != 1 || len(doc.Mocks[0].Match.Headers) != 1 || len(doc.Requests) != 1 {
		t.Fatalf("append changed the header-matched mock: %s", text)
	}
}

func TestNullRequestBodyStaysAMatcher(t *testing.T) {
	e := textEntry(1, "/users", "ok")
	e.Request.Headers.Set("Content-Type", "application/json")
	e.Request.Body = []byte("null")
	newPolicy(DefaultConfig()).sanitize(&e, DefaultBodyLimit)
	if string(e.Request.MatchJSON) != "null" {
		t.Fatalf("match JSON = %q, want null", e.Request.MatchJSON)
	}

	p := buildExport(t, []Entry{e}, ExportOptions{Mode: Mocks})
	h, err := mock.Compile([]*restfile.Document{p.Document})
	if err != nil {
		t.Fatal(err)
	}
	for body, want := range map[string]int{"null": 200, `{"unrelated":true}`: 404} {
		r := httptest.NewRequest("POST", "/users", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("body %s: status %d, want %d", body, w.Code, want)
		}
	}
}

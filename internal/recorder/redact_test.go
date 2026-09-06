package recorder

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRedactBodies(t *testing.T) {
	for _, tt := range []struct {
		name, ct, body string
		issue          bool
	}{
		{"duplicate", "application/json", `{"token":"one","token":"two"}`, true},
		{"malformed", "application/json", `{"token":"one"`, true},
		{"binary", "application/octet-stream", "payload", true},
		{"multipart", "multipart/form-data; boundary=x", "--x", true},
		{"form", "application/x-www-form-urlencoded", "password=hide&name=Ada", false},
		{"array", "application/json", `{"items":[{"token":"hide"}],"n":1}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := Entry{
				URL:     "http://example.invalid/",
				Request: Message{Headers: http.Header{"Content-Type": {tt.ct}}, Body: []byte(tt.body)},
			}
			newPolicy(DefaultConfig()).sanitize(&e, 1024)
			if (e.Request.Issue != "") != tt.issue {
				t.Fatalf("issue %q", e.Request.Issue)
			}
			if tt.issue && len(e.Request.Body) != 0 {
				t.Fatal("unavailable body retained")
			}
			if bytes.Contains(e.Request.Body, []byte("hide")) {
				t.Fatalf("named field kept its value: %s", e.Request.Body)
			}
			if tt.name == "array" && bytes.Contains(e.Request.MatchJSON, []byte("items")) {
				t.Fatal("redacted array was matched")
			}
		})
	}
}

func TestUnredactedBodiesKeepTheirBytes(t *testing.T) {
	for _, tt := range []struct{ contentType, body string }{
		{"application/json", `{ "b": 2, "a": 1, "nested": [1, {"c": null}] }`},
		{"application/x-www-form-urlencoded", "b=2&a=1"},
		{"text/plain", "Authorization: Bearer abcdefghijkl\nsee eyJhbGciOiJIUzI1NiJ9.eyJhIjoxfQ.sig\n"},
	} {
		t.Run(tt.contentType, func(t *testing.T) {
			e := Entry{URL: "http://example.invalid/", Request: Message{
				Headers: http.Header{"Content-Type": {tt.contentType}}, Body: []byte(tt.body),
			}}
			newPolicy(DefaultConfig()).sanitize(&e, 1024)
			if e.Request.Issue != "" || e.Request.Redactions != 0 || string(e.Request.Body) != tt.body {
				t.Fatalf("unredacted body changed: %+v", e.Request)
			}
		})
	}
}

func TestGzipAndLimits(t *testing.T) {
	var compressed bytes.Buffer
	z := gzip.NewWriter(&compressed)
	_, _ = z.Write([]byte(`{"token":"secretvalue"}`))
	_ = z.Close()
	e := Entry{
		URL: "http://example.invalid/",
		Request: Message{
			Headers: http.Header{"Content-Type": {"application/json"}, "Content-Encoding": {"gzip"}},
			Body:    compressed.Bytes(),
		},
	}
	newPolicy(DefaultConfig()).sanitize(&e, 1024)
	if e.Request.Issue != "" || e.Request.Headers.Get("Content-Encoding") != "" ||
		!bytes.Contains(e.Request.Body, []byte(redacted)) {
		t.Fatalf("gzip: %+v", e.Request)
	}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, strings.Repeat("x", 512))
	}))
	defer up.Close()
	s := startTest(t, up.URL, func(c *Config) { c.BodyLimit = 64; c.MaxEntries = 1 })
	for range 2 {
		res, err := http.Get("http://" + s.Addr() + "/x")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if len(b) != 512 {
			t.Fatal("limit changed forwarded body")
		}
		entriesAfter(t, s, 1)
	}
	if e := s.Snapshot(0)[0]; e.Response.Issue == "" || len(e.Response.Body) != 0 {
		t.Fatal("truncated body exported")
	}
	if stats := s.Stats(); stats.Excluded != 1 || stats.Limit != LimitEntries {
		t.Fatalf("stats %+v", stats)
	}
}

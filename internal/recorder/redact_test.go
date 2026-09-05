package recorder

import (
	"bytes"
	"net/http"
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

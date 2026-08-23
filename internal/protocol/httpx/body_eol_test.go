package httpx

import (
	"context"
	"net/http"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestBuildHTTPRequestKeepsBodyLineEndings(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "lf", body: "line1\nline2"},
		{name: "crlf", body: "line1\r\nline2"},
		{name: "mixed", body: "line1\ncrlf\r\nlast"},
		{name: "trailing", body: "line1\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &restfile.Request{
				Method: http.MethodPost,
				URL:    "http://example.com/",
				Body:   restfile.BodySource{Text: tt.body},
			}

			_, _, body, err := NewClient(nil).BuildHTTPRequest(
				context.Background(),
				req,
				nil,
				Options{},
			)
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			if string(body) != tt.body {
				t.Fatalf("body = %q, want %q", body, tt.body)
			}
		})
	}
}

func TestBuildHTTPRequestKeepsCRLFAroundExpansionsAndIncludes(t *testing.T) {
	req := &restfile.Request{
		Method: http.MethodPost,
		URL:    "http://example.com/",
		Body:   restfile.BodySource{Text: "head {{env.num}}\r\n@part.txt\r\ntail"},
	}
	resolver := vars.NewResolver(vars.NewMapProvider("env", map[string]string{"num": "500"}))
	client := NewClient(mapFS{"part.txt": []byte("included")})

	_, _, body, err := client.BuildHTTPRequest(context.Background(), req, resolver, Options{})
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if want := "head 500\r\nincluded\r\ntail"; string(body) != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
}

func TestBuildHTTPRequestKeepsBodyFileLineEndingsWhileExpanding(t *testing.T) {
	req := &restfile.Request{
		Method: http.MethodPost,
		URL:    "http://example.com/",
		Body: restfile.BodySource{
			FilePath: "payload.txt",
			Options:  restfile.BodyOptions{ExpandTemplates: true},
		},
	}
	resolver := vars.NewResolver(vars.NewMapProvider("env", map[string]string{"num": "7"}))
	client := NewClient(mapFS{"payload.txt": []byte("a {{env.num}}\r\nb\r\n")})

	_, _, body, err := client.BuildHTTPRequest(context.Background(), req, resolver, Options{})
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if want := "a 7\r\nb\r\n"; string(body) != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
}

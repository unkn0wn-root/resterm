package parser

import (
	"strings"
	"testing"
)

func TestInlineBodyKeepsSourceLineEndings(t *testing.T) {
	tests := []struct {
		name string
		term string
	}{
		{name: "lf", term: "\n"},
		{name: "crlf", term: "\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := strings.Join([]string{
				"POST http://example.com/ HTTP/1.1",
				"Content-Type: text/plain",
				"",
				"line1",
				"line2",
				"",
			}, tt.term)

			doc := Parse("body."+tt.name+".http", []byte(src))
			if len(doc.Requests) != 1 {
				t.Fatalf("expected 1 request, got %d", len(doc.Requests))
			}
			want := "line1" + tt.term + "line2"
			if got := doc.Requests[0].Body.Text; got != want {
				t.Fatalf("body = %q, want %q", got, want)
			}
		})
	}
}

// The newline before a separator ends the body. An extra blank line stays in it.
func TestInlineBodyDropsItsTrailingTerminator(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "separator follows the body",
			src:  "POST http://example.com/ HTTP/1.1\r\n\r\nonly\r\n### next\r\nGET http://example.com/x\r\n",
			want: "only",
		},
		{
			name: "blank line follows the body",
			src:  "POST http://example.com/ HTTP/1.1\r\n\r\nonly\r\n\r\n### next\r\nGET http://example.com/x\r\n",
			want: "only\r\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := Parse("trailing.http", []byte(tt.src))
			if len(doc.Requests) != 2 {
				t.Fatalf("expected 2 requests, got %d", len(doc.Requests))
			}
			if got := doc.Requests[0].Body.Text; got != tt.want {
				t.Fatalf("body = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInlineBodyKeepsMixedLineEndings(t *testing.T) {
	src := "POST http://example.com/ HTTP/1.1\n\nlf\ncrlf\r\nlast\n"

	doc := Parse("mixed.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	if got := doc.Requests[0].Body.Text; got != "lf\ncrlf\r\nlast" {
		t.Fatalf("body = %q, want %q", got, "lf\ncrlf\r\nlast")
	}
}

func TestCRLFSourceParsesLikeLF(t *testing.T) {
	src := "# @name crlf\r\n" +
		"GET http://example.com/games HTTP/1.1\r\n" +
		"Accept: application/json\r\n"

	doc := Parse("headers.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("unexpected parse errors: %#v", doc.Errors)
	}
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}

	req := doc.Requests[0]
	if req.Metadata.Name != "crlf" {
		t.Fatalf("name = %q, want %q", req.Metadata.Name, "crlf")
	}
	if req.URL != "http://example.com/games" {
		t.Fatalf("url = %q", req.URL)
	}
	if got := req.Headers.Get("Accept"); got != "application/json" {
		t.Fatalf("accept = %q", got)
	}
	if req.Settings["http-version"] != "1.1" {
		t.Fatalf("http-version = %q", req.Settings["http-version"])
	}
	if strings.Contains(req.OriginalText, "\r") {
		t.Fatalf("original text should stay normalized, got %q", req.OriginalText)
	}
}

package ui

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestRenderHTTPResponseCmdRawWrappedPreservesRawBody(t *testing.T) {
	body := []byte("{\"value\":\"" + strings.Repeat("a", 48) + "\"}")
	resp := &httpclient.Response{
		Status:       "200 OK",
		StatusCode:   200,
		Headers:      http.Header{"Content-Type": {"application/json"}},
		Body:         body,
		Duration:     12 * time.Millisecond,
		EffectiveURL: "https://example.com/items",
	}

	cmd := renderHTTPResponseCmd("token", resp, nil, nil, 12)
	if cmd == nil {
		t.Fatalf("expected command")
	}

	msgVal := cmd()
	msg, ok := msgVal.(responseRenderedMsg)
	if !ok {
		t.Fatalf("unexpected message type %T", msgVal)
	}

	_, rawView, _ := buildHTTPResponseViews(resp, nil, nil)
	expectedWrapped := wrapContentForTab(responseTabRaw, rawView, 12)
	if msg.rawWrapped != expectedWrapped {
		t.Fatalf("expected rawWrapped to match formatted raw view, got %q want %q", msg.rawWrapped, expectedWrapped)
	}
	lines := strings.Split(msg.rawWrapped, "\n")
	var (
		indent      string
		indentIndex = -1
	)
	for i, line := range lines {
		if strings.Contains(line, "\"value\"") {
			for _, r := range line {
				if r == ' ' || r == '\t' {
					indent += string(r)
					continue
				}
				break
			}
			indentIndex = i
			break
		}
	}
	if indentIndex == -1 {
		t.Fatalf("expected wrapped content to include value line, got %v", lines)
	}
	if indent == "" {
		t.Fatalf("expected value line to be indented, got %q", lines[indentIndex])
	}
	if indentIndex+1 >= len(lines) {
		t.Fatalf("expected continuation line after value segment, got %v", lines)
	}
	if !strings.HasPrefix(lines[indentIndex+1], indent) {
		t.Fatalf("expected continuation line to retain indentation %q, got %q", indent, lines[indentIndex+1])
	}
}

func TestBuildHTTPResponseViewsPreservesLeadingWhitespace(t *testing.T) {
	body := []byte("  leading line\n    indented line")
	resp := &httpclient.Response{
		Status:       "200 OK",
		StatusCode:   200,
		Headers:      http.Header{"Content-Type": {"text/plain"}},
		Body:         body,
		Duration:     5 * time.Millisecond,
		EffectiveURL: "https://example.com/whitespace",
	}

	pretty, raw, _ := buildHTTPResponseViews(resp, nil, nil)
	if !strings.Contains(pretty, "  leading line") {
		t.Fatalf("expected pretty view to retain leading spaces, got %q", pretty)
	}
	if !strings.Contains(raw, "  leading line") {
		t.Fatalf("expected raw view to retain leading spaces, got %q", raw)
	}
}

func TestBuildHTTPResponseViewsColorsSummaryExceptRaw(t *testing.T) {
	resp := &httpclient.Response{
		Status:     "201 Created",
		StatusCode: 201,
		Headers: http.Header{
			"Content-Type": {"application/json"},
			"X-Demo":       {"value"},
		},
		Body:         []byte(`{"id":1}`),
		Duration:     3 * time.Millisecond,
		EffectiveURL: "https://api.example.com/items",
	}

	pretty, raw, headers := buildHTTPResponseViews(resp, nil, nil)
	if !strings.Contains(pretty, statsLabelStyle.Render("Status:")) {
		t.Fatalf("expected colored status label, got %q", pretty)
	}
	if !strings.Contains(pretty, statsSuccessStyle.Render("201 Created")) {
		t.Fatalf("expected colored status value, got %q", pretty)
	}
	if !strings.Contains(pretty, statsDurationStyle.Render("3ms")) {
		t.Fatalf("expected colored duration value, got %q", pretty)
	}
	if strings.Contains(raw, "\x1b[") {
		t.Fatalf("expected raw view without ANSI codes, got %q", raw)
	}
	if !strings.Contains(headers, statsHeadingStyle.Render("Headers:")) {
		t.Fatalf("expected colored headers heading, got %q", headers)
	}
	if !strings.Contains(headers, statsLabelStyle.Render("Content-Type:")) {
		t.Fatalf("expected colored header names, got %q", headers)
	}
	if !strings.Contains(headers, statsHeaderValueStyle.Render("application/json")) {
		t.Fatalf("expected colored header values, got %q", headers)
	}
}

func TestBuildHTTPRequestHeadersViewUsesExecutedRequest(t *testing.T) {
	resp := &httpclient.Response{
		ReqMethod:    "GET",
		EffectiveURL: "https://final.example.com/items",
		Request: &restfile.Request{
			Method: "POST",
			URL:    "https://{{env}}/items",
		},
		RequestHeaders: http.Header{"X-Test": {"1"}},
	}

	view := buildHTTPRequestHeadersView(resp)
	plain := stripANSIEscape(view)
	if !strings.Contains(plain, "GET https://final.example.com/items") {
		t.Fatalf("expected request line to use executed method/url, got %q", plain)
	}
	if strings.Contains(plain, "{{env}}") {
		t.Fatalf("expected expanded URL to omit template placeholder, got %q", plain)
	}
}

func TestBuildRequestHeaderMapAddsDefaults(t *testing.T) {
	resp := &httpclient.Response{
		ReqMethod: "GET",
		ReqHost:   "example.com",
	}
	hdrs := buildRequestHeaderMap(resp)
	if hdrs.Get("Host") != "example.com" {
		t.Fatalf("expected host to be populated from request host, got %q", hdrs.Get("Host"))
	}
}

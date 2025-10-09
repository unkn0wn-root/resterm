package ui

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
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

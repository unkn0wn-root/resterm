package ui

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
)

func TestWrapDiffContentPreservesMarkers(t *testing.T) {
	diff := "--- a\n+++ b\n-" + strings.Repeat("x", 40) + "\n+" + strings.Repeat("y", 40)
	wrapped := wrapDiffContent(diff, 12)
	lines := strings.Split(wrapped, "\n")
	for _, line := range lines {
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "---"):
			continue
		case strings.HasPrefix(line, "+++"):
			continue
		case strings.HasPrefix(line, "@@"):
			continue
		}
		marker := line[0]
		if marker != '-' && marker != '+' && marker != ' ' {
			t.Fatalf("expected diff marker prefix, got %q", line)
		}
	}
}

func TestWrapDiffContentHandlesContextLines(t *testing.T) {
	diff := " " + strings.Repeat("ctx ", 6)
	wrapped := wrapDiffContent(diff, 8)
	for _, line := range strings.Split(wrapped, "\n") {
		if line == "" {
			continue
		}
		if line[0] != ' ' {
			t.Fatalf("expected context line to retain space prefix, got %q", line)
		}
	}
}

func TestWrapDiffContentFallback(t *testing.T) {
	diff := "+short"
	wrapped := wrapDiffContent(diff, 10)
	if wrapped != diff {
		t.Fatalf("expected short diff to remain unchanged, got %q", wrapped)
	}
}

func TestComputeDiffForHeadersIncludesBody(t *testing.T) {
	model := New(Config{})
	model.responseSplit = true

	left := &responseSnapshot{
		pretty: ensureTrailingNewline(
			"Status: 201 Created\nContent-Length: 15 bytes\nURL: http://localhost/items\nDuration: 3ms\n\n{\n  \"value\": \"one\"\n}",
		),
		raw: ensureTrailingNewline(
			"Status: 201 Created\nContent-Length: 15 bytes\nURL: http://localhost/items\nDuration: 3ms\n\n{\n  \"value\": \"one\"\n}",
		),
		headers: ensureTrailingNewline(
			"Status: 201 Created\nContent-Length: 15 bytes\nURL: http://localhost/items\nDuration: 3ms\n\nHeaders:\nContent-Type: application/json",
		),
		ready: true,
	}
	right := &responseSnapshot{
		pretty: ensureTrailingNewline(
			"Status: 200 OK\nContent-Length: 15 bytes\nURL: http://localhost/items\nDuration: 4ms\n\n{\n  \"value\": \"two\"\n}",
		),
		raw: ensureTrailingNewline(
			"Status: 200 OK\nContent-Length: 15 bytes\nURL: http://localhost/items\nDuration: 4ms\n\n{\n  \"value\": \"two\"\n}",
		),
		headers: ensureTrailingNewline(
			"Status: 200 OK\nContent-Length: 15 bytes\nURL: http://localhost/items\nDuration: 4ms\n\nHeaders:\nContent-Type: application/json",
		),
		ready: true,
	}

	model.responsePanes[0].snapshot = left
	model.responsePanes[0].lastContentTab = responseTabHeaders
	model.responsePanes[1].snapshot = right
	model.responsePanes[1].lastContentTab = responseTabHeaders

	diff, ok := model.computeDiffFor(responsePanePrimary, responseTabHeaders)
	if !ok {
		t.Fatalf("expected diff availability")
	}
	plain := stripANSIEscape(diff)
	if !strings.Contains(plain, "\"value\": \"one\"") ||
		!strings.Contains(plain, "\"value\": \"two\"") {
		t.Fatalf("expected body diff, got %q", plain)
	}
	if !strings.Contains(plain, "Headers") {
		t.Fatalf("expected headers section in diff, got %q", plain)
	}
}

func TestComputeDiffRawUsesRawView(t *testing.T) {
	model := New(Config{})
	model.responseSplit = true

	left := &responseSnapshot{
		raw:    ensureTrailingNewline("raw-body-1"),
		pretty: ensureTrailingNewline("pretty-body-1"),
		ready:  true,
	}
	right := &responseSnapshot{
		raw:    ensureTrailingNewline("raw-body-2"),
		pretty: ensureTrailingNewline("pretty-body-2"),
		ready:  true,
	}

	model.responsePanes[0].snapshot = left
	model.responsePanes[1].snapshot = right

	diff, ok := model.computeDiffFor(responsePanePrimary, responseTabRaw)
	if !ok {
		t.Fatalf("expected diff availability")
	}
	pl := stripANSIEscape(diff)
	if strings.Contains(pl, "pretty-body") {
		t.Fatalf("unexpected pretty diff content: %q", pl)
	}
	if !strings.Contains(pl, "raw-body-1") || !strings.Contains(pl, "raw-body-2") {
		t.Fatalf("expected raw diff content, got %q", pl)
	}
}

func TestComputeDiffForPrettyDetectsLeadingWhitespaceChanges(t *testing.T) {
	model := New(Config{})
	model.responseSplit = true

	respA := &httpclient.Response{
		Status:       "200 OK",
		StatusCode:   200,
		Headers:      http.Header{"Content-Type": {"text/plain"}},
		Body:         []byte("  leading line\nshared line"),
		Duration:     10 * time.Millisecond,
		EffectiveURL: "https://example.com/a",
	}
	respB := &httpclient.Response{
		Status:       "200 OK",
		StatusCode:   200,
		Headers:      http.Header{"Content-Type": {"text/plain"}},
		Body:         []byte("    leading line\nshared line"),
		Duration:     12 * time.Millisecond,
		EffectiveURL: "https://example.com/b",
	}

	viewsA := buildHTTPResponseViews(respA, nil, nil)
	viewsB := buildHTTPResponseViews(respB, nil, nil)
	prettyA, rawA := viewsA.pretty, viewsA.raw
	prettyB, rawB := viewsB.pretty, viewsB.raw

	model.responsePanes[0].snapshot = &responseSnapshot{
		pretty: ensureTrailingNewline(prettyA),
		raw:    ensureTrailingNewline(rawA),
		ready:  true,
	}
	model.responsePanes[1].snapshot = &responseSnapshot{
		pretty: ensureTrailingNewline(prettyB),
		raw:    ensureTrailingNewline(rawB),
		ready:  true,
	}

	diff, ok := model.computeDiffFor(responsePanePrimary, responseTabPretty)
	if !ok {
		t.Fatalf("expected diff availability")
	}
	plain := stripANSIEscape(diff)
	if !strings.Contains(plain, "-  leading line") {
		t.Fatalf("expected diff to include removal of original indentation, got %q", plain)
	}
	if !strings.Contains(plain, "+    leading line") {
		t.Fatalf("expected diff to include addition with new indentation, got %q", plain)
	}
	if strings.Contains(plain, "shared line") {
		// shared line should appear only as context without diff markers
		for _, line := range strings.Split(plain, "\n") {
			if strings.Contains(line, "shared line") && len(line) > 0 && line[0] != ' ' &&
				line[0] != '@' {
				t.Fatalf("expected shared line to appear only in context, got %q", line)
			}
		}
	}
}

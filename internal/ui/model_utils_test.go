package ui

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

func TestWrapLineSegmentsPreservesLeadingIndent(t *testing.T) {
	line := "    indentation text"
	segments := wrapLineSegments(line, 10)
	if len(segments) < 2 {
		t.Fatalf("expected multiple segments, got %d", len(segments))
	}
	if !strings.HasPrefix(segments[0], "    ") {
		t.Fatalf("expected first segment to include indentation, got %q", segments[0])
	}
}

func TestWrapLineSegmentsSkipsLeadingWhitespaceOnContinuation(t *testing.T) {
	segments := wrapLineSegments("foo bar baz", 5)
	if len(segments) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(segments))
	}
	for i := 1; i < len(segments); i++ {
		if strings.HasPrefix(segments[i], " ") {
			t.Fatalf("segment %d unexpectedly starts with whitespace: %q", i, segments[i])
		}
	}
}

func TestWrapLineSegmentsHandlesLongTokenWithIndent(t *testing.T) {
	segments := wrapLineSegments("    Supercalifragilistic", 10)
	if len(segments) < 2 {
		t.Fatalf("expected wrapped long token, got %d segments", len(segments))
	}
	if !strings.HasPrefix(segments[0], "    ") {
		t.Fatalf("expected first segment to preserve indentation, got %q", segments[0])
	}
	if !strings.Contains(segments[0], "Super") {
		t.Fatalf("expected first segment to contain token prefix, got %q", segments[0])
	}
}

func TestWrapLineSegmentsSplitsLongWhitespace(t *testing.T) {
	line := strings.Repeat(" ", 12) + "x"
	segments := wrapLineSegments(line, 5)
	if len(segments) < 3 {
		t.Fatalf("expected whitespace to be split across segments, got %d", len(segments))
	}
	if segments[0] != strings.Repeat(" ", 5) {
		t.Fatalf("expected first segment to contain 5 spaces, got %q", segments[0])
	}
	if !strings.HasSuffix(segments[len(segments)-1], "x") {
		t.Fatalf(
			"expected final segment to include trailing content, got %q",
			segments[len(segments)-1],
		)
	}
}

func TestWrapLineSegmentsWithANSIEscape(t *testing.T) {
	line := "\x1b[31mError:\x1b[0m details"
	segments := wrapLineSegments(line, 6)
	if len(segments) < 2 {
		t.Fatalf("expected ANSI-colored text to wrap, got %d segments", len(segments))
	}
	joined := strings.Join(segments, "")
	if !strings.Contains(joined, "\x1b[31m") || !strings.Contains(joined, "\x1b[0m") {
		t.Fatalf("expected ANSI escape codes to be preserved, got %q", joined)
	}
}

func TestWrapToWidthPreservesJSONIndentation(t *testing.T) {
	json := "{\n    \"key\": \"value\",\n    \"nested\": {\n        \"deep\": \"value\"\n    }\n}"
	wrapped := wrapToWidth(json, 12)
	lines := strings.Split(wrapped, "\n")
	if len(lines) < 5 {
		t.Fatalf("expected wrapped JSON to produce multiple lines, got %d", len(lines))
	}

	var foundIndented bool
	for _, line := range lines {
		if strings.HasPrefix(line, "    \"") || strings.HasPrefix(line, "        \"") {
			foundIndented = true
		}
	}

	if !foundIndented {
		t.Fatalf("expected at least one wrapped line to retain JSON indentation, got %v", lines)
	}
	if !strings.Contains(strings.Join(lines, ""), "\"deep\"") {
		t.Fatalf("expected wrapped JSON to contain nested keys, got %q", strings.Join(lines, ""))
	}
}

func TestWrapContentForTabRawMaintainsIndentOnWrap(t *testing.T) {
	body := strings.Join([]string{
		"Status: 200 OK",
		"URL: http://example.com",
		"",
		"    \"key\": \"" + strings.Repeat("a", 32) + "\"",
	}, "\n")

	wrapped := wrapContentForTab(responseTabRaw, body, 20)
	lines := strings.Split(wrapped, "\n")
	var indentLineIndex = -1
	for i, line := range lines {
		if strings.HasPrefix(line, "    \"key\":") {
			indentLineIndex = i
			break
		}
	}

	if indentLineIndex == -1 {
		t.Fatalf("expected wrapped content to include indented key line, got %v", lines)
	}
	if indentLineIndex+1 >= len(lines) {
		t.Fatalf("expected continuation line after indented key, got %v", lines)
	}
	if !strings.HasPrefix(lines[indentLineIndex+1], "    ") {
		t.Fatalf(
			"expected continuation line to retain indentation, got %q",
			lines[indentLineIndex+1],
		)
	}
}

func TestWrapToWidthRetainsMultilineIndentationAndColor(t *testing.T) {
	content := strings.Join([]string{
		"    {",
		"        \"alpha\": \"value\",",
		"        \"beta\": \"supercalifragilisticexpialidocious\",",
		"        \"gamma\": \"\x1b[32mcolored\x1b[0m\"",
		"    }",
	}, "\n")
	lines := strings.Split(wrapToWidth(content, 18), "\n")
	if len(lines) <= 5 {
		t.Fatalf("expected wrapped multiline content to expand, got %d lines", len(lines))
	}
	if !strings.HasPrefix(lines[0], "    {") {
		t.Fatalf("expected first line to retain indentation, got %q", lines[0])
	}
	if joined := strings.Join(
		lines,
		"",
	); !strings.Contains(
		joined,
		"supercalifragilisticexpialidocious",
	) {
		t.Fatalf("expected wrapped content to retain long word, got %q", joined)
	}
}

func TestWrapStructuredLineAddsDefaultIndent(t *testing.T) {
	line := "\"message\": \"" + strings.Repeat("x", 24) + "\""
	segments := wrapStructuredLine(line, 16)
	if len(segments) < 2 {
		t.Fatalf("expected line to wrap, got %d segments", len(segments))
	}
	if !strings.HasPrefix(stripANSIEscape(segments[1]), wrapContinuationUnit) {
		t.Fatalf(
			"expected continuation to start with %q, got %q",
			wrapContinuationUnit,
			segments[1],
		)
	}
}

func TestWrapStructuredLineExtendsExistingIndent(t *testing.T) {
	line := "    \"details\": \"" + strings.Repeat("y", 30) + "\""
	segments := wrapStructuredLine(line, 18)
	if len(segments) < 2 {
		t.Fatalf("expected wrapped segments, got %d", len(segments))
	}
	second := stripANSIEscape(segments[1])
	expectedPrefix := "      "
	if !strings.HasPrefix(second, expectedPrefix) {
		t.Fatalf("expected continuation to start with %q, got %q", expectedPrefix, segments[1])
	}
}

func TestWrapStructuredLineHandlesNarrowWidth(t *testing.T) {
	line := "    \"note\": \"short\""
	segments := wrapStructuredLine(line, 4)
	if len(segments) < 2 {
		t.Fatalf("expected line to wrap with narrow width, got %d segments", len(segments))
	}
	if strings.HasPrefix(stripANSIEscape(segments[1]), wrapContinuationUnit) {
		t.Fatalf(
			"expected continuation indent to be suppressed for narrow width, got %q",
			segments[1],
		)
	}
}

func TestWrapContentForTabPrettyUsesStructuredWrap(t *testing.T) {
	content := "\"payload\": \"" + strings.Repeat("z", 28) + "\""
	wrapped := wrapContentForTab(responseTabPretty, content, 20)
	lines := strings.Split(wrapped, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected pretty content to wrap, got %v", lines)
	}
	if !strings.HasPrefix(stripANSIEscape(lines[1]), wrapContinuationUnit) {
		t.Fatalf("expected continuation line to include structured indent, got %q", lines[1])
	}
}

func TestWrapStructuredLineKeepsANSIPrefix(t *testing.T) {
	coloredIndent := "\x1b[31m    \x1b[0m"
	line := coloredIndent + "\"ansi\": \"" + strings.Repeat("q", 18) + "\""
	segments := wrapStructuredLine(line, 14)
	if len(segments) < 2 {
		t.Fatalf("expected ANSI line to wrap, got %d segments", len(segments))
	}
	if !strings.HasPrefix(segments[1], "\x1b[31m") {
		t.Fatalf("expected continuation to begin with ANSI prefix, got %q", segments[1])
	}
	if !strings.HasPrefix(stripANSIEscape(segments[1]), "      ") {
		t.Fatalf("expected continuation to retain extended indent, got %q", segments[1])
	}
}

func TestWrapStructuredLineMaintainsValueColor(t *testing.T) {
	keyColor := "\x1b[32m"
	valueColor := "\x1b[37m"
	reset := "\x1b[0m"
	line := "    " + keyColor + "\"repository_search_url\"" + reset + ": " + valueColor + "\"https://api.github.com/search/" + strings.Repeat(
		"x",
		12,
	) + "\"" + reset
	segments := wrapStructuredLine(line, 44)
	if len(segments) < 2 {
		t.Fatalf("expected wrapped segments, got %d", len(segments))
	}
	continuation := segments[1]
	if !strings.Contains(continuation, valueColor) {
		t.Fatalf("expected continuation to include value color, got %q", continuation)
	}
	if strings.Contains(continuation, keyColor) {
		t.Fatalf("expected continuation not to include key color, got %q", continuation)
	}
}

func TestRenderContentLengthLine(t *testing.T) {
	t.Run("uses numeric header value", func(t *testing.T) {
		resp := &httpclient.Response{
			Headers: http.Header{"Content-Length": {"64"}},
			Body:    []byte("payload"),
		}
		line := renderContentLengthLine(resp)
		plain := stripANSIEscape(line)
		if plain != "Content-Length: 64 bytes" {
			t.Fatalf("expected header-derived content length, got %q", plain)
		}
	})

	t.Run("falls back to body length", func(t *testing.T) {
		resp := &httpclient.Response{Body: []byte("xyz")}
		line := renderContentLengthLine(resp)
		plain := stripANSIEscape(line)
		if plain != "Content-Length: 3 bytes" {
			t.Fatalf("expected body length fallback, got %q", plain)
		}
	})

	t.Run("returns raw header when non-numeric", func(t *testing.T) {
		resp := &httpclient.Response{
			Headers: http.Header{"Content-Length": {"approx 100"}},
		}
		line := renderContentLengthLine(resp)
		plain := stripANSIEscape(line)
		if plain != "Content-Length: approx 100" {
			t.Fatalf("expected raw header value, got %q", plain)
		}
	})

	t.Run("pluralizes singular byte", func(t *testing.T) {
		resp := &httpclient.Response{Body: []byte{0x01}}
		line := renderContentLengthLine(resp)
		plain := stripANSIEscape(line)
		if plain != "Content-Length: 1 byte" {
			t.Fatalf("expected singular byte form, got %q", plain)
		}
	})
}

func TestRenderContentLengthLinePretty(t *testing.T) {
	t.Run("formats numeric header", func(t *testing.T) {
		resp := &httpclient.Response{
			Headers: http.Header{"Content-Length": {"2048"}},
		}
		line := renderContentLengthLinePretty(resp)
		plain := stripANSIEscape(line)
		if plain != "Content-Length: 2 KiB" {
			t.Fatalf("expected human readable size, got %q", plain)
		}
	})

	t.Run("falls back to body length", func(t *testing.T) {
		resp := &httpclient.Response{Body: bytes.Repeat([]byte{'x'}, 1536)}
		line := renderContentLengthLinePretty(resp)
		plain := stripANSIEscape(line)
		if plain != "Content-Length: 1.5 KiB" {
			t.Fatalf("expected body length in human readable form, got %q", plain)
		}
	})

	t.Run("handles zero length", func(t *testing.T) {
		resp := &httpclient.Response{}
		line := renderContentLengthLinePretty(resp)
		plain := stripANSIEscape(line)
		if plain != "Content-Length: 0 B" {
			t.Fatalf("expected zero length to render with unit, got %q", plain)
		}
	})

	t.Run("returns raw value when non-numeric", func(t *testing.T) {
		resp := &httpclient.Response{Headers: http.Header{"Content-Length": {"approx 100"}}}
		line := renderContentLengthLinePretty(resp)
		plain := stripANSIEscape(line)
		if plain != "Content-Length: approx 100" {
			t.Fatalf("expected non-numeric header to remain unchanged, got %q", plain)
		}
	})
}

func TestFormatTestSummaryColorsStatuses(t *testing.T) {
	results := []scripts.TestResult{
		{Name: "alpha", Passed: true, Elapsed: 1500 * time.Millisecond},
		{Name: "beta", Passed: false, Message: "boom"},
	}

	output := formatTestSummary(results, nil)

	if !strings.Contains(output, statsHeadingStyle.Render("Tests:")) {
		t.Fatalf("expected colored Tests header, got %q", output)
	}
	if !strings.Contains(output, statsSuccessStyle.Render("[PASS]")) {
		t.Fatalf("expected PASS badge to be colored, got %q", output)
	}
	if !strings.Contains(output, statsWarnStyle.Render("[FAIL]")) {
		t.Fatalf("expected FAIL badge to be colored, got %q", output)
	}
	if !strings.Contains(output, statsDurationStyle.Render("(1.5s)")) {
		t.Fatalf("expected duration to be colored, got %q", output)
	}
	if !strings.Contains(output, statsMessageStyle.Render("boom")) {
		t.Fatalf("expected message to be colored, got %q", output)
	}
}

func TestFormatTestSummaryColorsErrors(t *testing.T) {
	err := errors.New("kaboom")
	output := formatTestSummary(nil, err)

	if !strings.Contains(output, statsWarnStyle.Render("[ERROR]")) {
		t.Fatalf("expected error badge to be colored, got %q", output)
	}
	if !strings.Contains(output, statsMessageStyle.Render("kaboom")) {
		t.Fatalf("expected error message to be colored, got %q", output)
	}
}

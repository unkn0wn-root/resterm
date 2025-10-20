package ui

import (
	"strings"
	"testing"
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
		t.Fatalf("expected final segment to include trailing content, got %q", segments[len(segments)-1])
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
		t.Fatalf("expected continuation line to retain indentation, got %q", lines[indentLineIndex+1])
	}
}

func TestStripANSIEscapeExtendedSequences(t *testing.T) {
	input := "\x1b[?25l\x1b]0;title\x07hello"
	stripped := stripANSIEscape(input)
	if stripped != "hello" {
		t.Fatalf("expected stripped output to be hello, got %q", stripped)
	}
	if width := visibleWidth(input); width != len("hello") {
		t.Fatalf("expected visible width to ignore ANSI sequences, got %d", width)
	}
}

func TestWrapLineSegmentsComplexMixedContent(t *testing.T) {
	line := "\t    \x1b[32m✓\x1b[0m 你好世界 resterm supercalifragilisticexpialidocious"
	segments := wrapLineSegments(line, 14)
	if len(segments) < 4 {
		t.Fatalf("expected multiple segments, got %d", len(segments))
	}
	if !strings.HasPrefix(segments[0], "\t    ") {
		t.Fatalf("expected first segment to keep tab + spaces, got %q", segments[0])
	}
	var sawResterm bool
	for idx, segment := range segments {
		if segment == "" {
			t.Fatalf("segment %d empty", idx)
		}
		if strings.Contains(segment, "resterm") {
			sawResterm = true
			if strings.HasPrefix(segment, " ") {
				t.Fatalf("continuation segment unexpectedly starts with space: %q", segment)
			}
		}
	}
	if !sawResterm {
		t.Fatalf("expected to find continuation segment containing 'resterm', segments=%v", segments)
	}
}

func TestWrapLineSegmentsVeryNarrowUnicode(t *testing.T) {
	line := "你好世界"
	segments := wrapLineSegments(line, 2)
	if len(segments) != 4 {
		t.Fatalf("expected each rune to form its own segment, got %d", len(segments))
	}

	for _, segment := range segments {
		if strings.TrimSpace(segment) == "" {
			t.Fatalf("unexpected blank segment in %v", segments)
		}
	}

	if strings.Join(segments, "") != line {
		t.Fatalf("expected wrapped unicode content to match original, got %q", strings.Join(segments, ""))
	}
}

func TestWrapToWidthMultiLineMixedContent(t *testing.T) {
	multiline := strings.Join([]string{
		"    {",
		"        \"greeting\": \"hello\",",
		"        \"colored\": \"\x1b[36mcyan text\x1b[0m\",",
		"        \"data\": \"supercalifragilisticexpialidocious\"",
		"    }",
	}, "\n")
	wrapped := wrapToWidth(multiline, 20)
	lines := strings.Split(wrapped, "\n")
	if len(lines) <= 5 {
		t.Fatalf("expected wrapped multiline content to expand, got %d lines", len(lines))
	}
	if !strings.HasPrefix(lines[0], "    {") {
		t.Fatalf("expected first line to retain indentation, got %q", lines[0])
	}

	var continuationWithoutIndent bool
	for i := 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "        ") {
			continue
		}
		if !strings.HasPrefix(lines[i], "    ") && !strings.HasPrefix(lines[i], "        ") && strings.HasPrefix(lines[i], "\"") {
			continuationWithoutIndent = true
		}
	}
	if !continuationWithoutIndent {
		t.Fatalf("expected at least one continuation line without leading indentation, got %v", lines)
	}

	joined := strings.Join(lines, "")
	if !strings.Contains(joined, "supercalifragilisticexpialidocious") {
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
		t.Fatalf("expected continuation to start with %q, got %q", wrapContinuationUnit, segments[1])
	}
	if width := visibleWidth(segments[1]); width > 16 {
		t.Fatalf("expected continuation width <= 16, got %d", width)
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
		t.Fatalf("expected continuation indent to be suppressed for narrow width, got %q", segments[1])
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
	line := "    " + keyColor + "\"repository_search_url\"" + reset + ": " + valueColor + "\"https://api.github.com/search/" + strings.Repeat("x", 12) + "\"" + reset
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

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

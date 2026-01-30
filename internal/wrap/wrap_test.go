package wrap

import (
	"context"
	"strings"
	"testing"
)

func TestWrapCarriesResetColorSequence(t *testing.T) {
	line := "\x1b[0;31m" + strings.Repeat("X", 12) + "\x1b[0m"
	res, ok := Wrap(context.Background(), line, 5, Plain, false)
	if !ok {
		t.Fatalf("wrap failed")
	}
	lines := strings.Split(res.S, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrapping, got %d line(s): %q", len(lines), res.S)
	}
	if !strings.HasPrefix(lines[1], "\x1b[0;31m") {
		t.Fatalf("expected continuation to keep 0;31m prefix, got %q", lines[1])
	}
}

func TestWrapExtendedColorDoesNotClearOtherStyles(t *testing.T) {
	line := "\x1b[1m\x1b[38;5;0m" + strings.Repeat("Y", 12)
	res, ok := Wrap(context.Background(), line, 5, Plain, false)
	if !ok {
		t.Fatalf("wrap failed")
	}
	lines := strings.Split(res.S, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrapping, got %d line(s): %q", len(lines), res.S)
	}
	wantPrefix := "\x1b[1m\x1b[38;5;0m"
	if !strings.HasPrefix(lines[1], wantPrefix) {
		t.Fatalf("expected continuation to keep %q, got %q", wantPrefix, lines[1])
	}
}

func TestWrapPreDoesNotEmitIndentOnlyLine(t *testing.T) {
	line := "    abc"
	res, ok := Wrap(context.Background(), line, 6, Pre, false)
	if !ok {
		t.Fatalf("wrap failed")
	}
	lines := strings.Split(res.S, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrapping, got %d line(s): %q", len(lines), res.S)
	}
	if strings.TrimSpace(lines[0]) == "" {
		t.Fatalf("expected first line to include content, got %q", lines[0])
	}
}

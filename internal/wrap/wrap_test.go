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

func TestWrapStructuredContinuationAvoidsPrefixColorLeak(t *testing.T) {
	rd := "\x1b[31m"
	gr := "\x1b[32m"
	rs := "\x1b[0m"
	ln := rd + "    " + rs + gr + strings.Repeat("A", 36) + rs

	sg, ok := Line(context.Background(), ln, 16, Structured)
	if !ok {
		t.Fatalf("wrap failed")
	}
	if len(sg) < 2 {
		t.Fatalf("expected wrapped continuation, got %d segment(s)", len(sg))
	}

	c := sg[1]
	if !strings.HasPrefix(c, rd) {
		t.Fatalf("expected continuation to keep prefix ANSI, got %q", c)
	}
	i := strings.Index(c, gr)
	if i == -1 {
		t.Fatalf("expected continuation to include token color, got %q", c)
	}
	if j := strings.Index(c[i+len(gr):], rd); j != -1 {
		t.Fatalf("expected no leaked prefix color after token color, got %q", c)
	}
}

func TestWrapStructuredContinuationKeepsTokenColorAfterPrefixReset(t *testing.T) {
	rd := "\x1b[31m"
	gr := "\x1b[32m"
	rs := "\x1b[0m"
	ln := rd + rs + "    " + gr + strings.Repeat("B", 36) + rs

	sg, ok := Line(context.Background(), ln, 16, Structured)
	if !ok {
		t.Fatalf("wrap failed")
	}
	if len(sg) < 2 {
		t.Fatalf("expected wrapped continuation, got %d segment(s)", len(sg))
	}

	c := sg[1]
	if !strings.Contains(c, gr) {
		t.Fatalf("expected continuation to restore token color after prefix reset, got %q", c)
	}
	i := strings.Index(c, gr)
	if j := strings.Index(c[i+len(gr):], rd); j != -1 {
		t.Fatalf("expected no prefix color after token color, got %q", c)
	}
}

func TestWrapStructuredContinuationKeepsResetFromPrefix(t *testing.T) {
	rs := "\x1b[0m"
	cl := "\x1b[38;2;230;219;116m"
	ln := "  " + rs + cl + "\"Root=1-6998c40f-78b385122e209bd52563 3827\""

	sg, ok := Line(context.Background(), ln, 40, Structured)
	if !ok {
		t.Fatalf("wrap failed")
	}
	if len(sg) < 2 {
		t.Fatalf("expected wrapped continuation, got %d segment(s)", len(sg))
	}

	c := sg[1]
	if !strings.Contains(c, rs) {
		t.Fatalf("expected continuation to retain reset from prefix, got %q", c)
	}
	i := strings.Index(c, rs)
	j := strings.Index(c, cl)
	if i == -1 || j == -1 || i > j {
		t.Fatalf("expected reset before color in continuation, got %q", c)
	}
}

func TestWrapStructuredContinuationNoSyntheticResetWithoutPrefixReset(t *testing.T) {
	cl := "\x1b[32m"
	ln := "  " + cl + "\"" + strings.Repeat("x", 32) + "\""

	sg, ok := Line(context.Background(), ln, 20, Structured)
	if !ok {
		t.Fatalf("wrap failed")
	}
	if len(sg) < 2 {
		t.Fatalf("expected wrapped continuation, got %d segment(s)", len(sg))
	}

	c := sg[1]
	if strings.Contains(c, "\x1b[0m") {
		t.Fatalf("expected no synthetic reset on continuation, got %q", c)
	}
}

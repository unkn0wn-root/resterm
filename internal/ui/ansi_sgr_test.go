package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestParseSGRParamsRejectsPrivateCSI(t *testing.T) {
	if isSGR("\x1b[?m") {
		t.Fatal("expected private CSI parameters not to be treated as SGR")
	}
	if _, ok := parseSGRParams("\x1b[?25m"); ok {
		t.Fatal("expected private CSI with digits not to parse as SGR")
	}
}

func TestParseSGRParamsHandlesEmptyAndTrailingDefaults(t *testing.T) {
	tests := []struct {
		seq  string
		want []int
	}{
		{seq: "\x1b[m", want: []int{0}},
		{seq: "\x1b[31;m", want: []int{31, 0}},
		{seq: "\x1b[31;;1m", want: []int{31, 0, 1}},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.seq), func(t *testing.T) {
			got, ok := parseSGRParams(tt.seq)
			if !ok {
				t.Fatalf("expected %q to parse", tt.seq)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("parseSGRParams(%q)=%v, want %v", tt.seq, got, tt.want)
			}
		})
	}
}

func TestResponseSearchBoundariesIgnorePrivateCSIInRestore(t *testing.T) {
	bounds := responseSearchBoundaries("\x1b[31mred\x1b[?mX")
	if len(bounds) != 5 {
		t.Fatalf("expected 4 visible runes plus initial boundary, got %d", len(bounds))
	}
	restore := bounds[len(bounds)-1].sgr
	if strings.Contains(restore, "?") {
		t.Fatalf("expected private CSI to be ignored in restore state, got %q", restore)
	}
	if restore != "\x1b[31m" {
		t.Fatalf("expected red foreground restore, got %q", restore)
	}
}

func TestResponseSearchBoundariesCompactSGRRestore(t *testing.T) {
	bounds := responseSearchBoundaries("\x1b[31m\x1b[32m\x1b[33mX")
	if len(bounds) != 2 {
		t.Fatalf("expected one visible rune plus initial boundary, got %d", len(bounds))
	}
	if got := bounds[1].sgr; got != "\x1b[33m" {
		t.Fatalf("expected compact latest foreground restore, got %q", got)
	}
}

func TestResponseSearchBoundariesPreserveCompositeSGRRestore(t *testing.T) {
	content := "\x1b[1m\x1b[3m\x1b[38;2;0;0;0m\x1b[48;5;244mX"
	bounds := responseSearchBoundaries(content)
	if len(bounds) != 2 {
		t.Fatalf("expected one visible rune plus initial boundary, got %d", len(bounds))
	}
	want := "\x1b[1;3;38;2;0;0;0;48;5;244m"
	if got := bounds[1].sgr; got != want {
		t.Fatalf("expected composite restore %q, got %q", want, got)
	}
}

func TestResponseSearchBoundariesClearAttributesWithoutClearingColors(t *testing.T) {
	bounds := responseSearchBoundaries("\x1b[1;31m\x1b[22mX")
	if len(bounds) != 2 {
		t.Fatalf("expected one visible rune plus initial boundary, got %d", len(bounds))
	}
	if got := bounds[1].sgr; got != "\x1b[31m" {
		t.Fatalf("expected intensity reset to keep foreground only, got %q", got)
	}
}

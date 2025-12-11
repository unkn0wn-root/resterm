package scroll

import "testing"

func TestAlignKeepsBufferAndCentersWhenNeeded(t *testing.T) {
	off := Align(0, 0, 4, 10)
	if off != 0 {
		t.Fatalf("expected offset 0 at top, got %d", off)
	}

	off = Align(2, off, 4, 10)
	if off != 0 {
		t.Fatalf("expected offset to stay put inside buffer, got %d", off)
	}

	off = Align(3, off, 4, 10)
	if off != 1 {
		t.Fatalf("expected offset to move minimally, got %d", off)
	}

	off = Align(9, off, 4, 10)
	if off != 6 {
		t.Fatalf("expected offset to clamp near end, got %d", off)
	}
}

func TestAlignPinsLastPage(t *testing.T) {
	h := 4
	total := 6
	maxOff := total - h
	off := Align(4, 0, h, total)
	if off != maxOff {
		t.Fatalf("expected offset to stay pinned at end, got %d", off)
	}
	off = Align(5, off, h, total)
	if off != maxOff {
		t.Fatalf("expected offset to stay pinned at end, got %d", off)
	}
}

func TestRevealKeepsVisibleSpan(t *testing.T) {
	total := 50
	h := 10
	off := 20
	start := 23
	end := 24
	got := Reveal(start, end, off, h, total)
	if got != off {
		t.Fatalf("expected offset to stay %d when span visible, got %d", off, got)
	}
}

func TestRevealMovesUpWhenAboveBuffer(t *testing.T) {
	total := 50
	h := 10
	off := 20
	start := 17
	end := 18
	got := Reveal(start, end, off, h, total)
	want := 15 // start - buf (2)
	if got != want {
		t.Fatalf("expected offset %d, got %d", want, got)
	}
}

func TestRevealClampsNearEnd(t *testing.T) {
	total := 30
	h := 10
	off := 0
	start := 28
	end := 29
	got := Reveal(start, end, off, h, total)
	want := total - h
	if got != want {
		t.Fatalf("expected offset to clamp to %d, got %d", want, got)
	}
}

func TestRevealHandlesWideSpan(t *testing.T) {
	total := 15
	h := 5
	off := 0
	start := 4
	end := 12
	got := Reveal(start, end, off, h, total)
	want := 9 // keep end inside buffer at bottom
	if got != want {
		t.Fatalf("expected offset %d, got %d", want, got)
	}
}

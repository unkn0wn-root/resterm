package scroll

import "testing"

func TestAlignKeepsCursorCentered(t *testing.T) {
	// h=4 means center is at 4/2 = 2
	// When sel=0, offset should be 0-2 = -2, clamped to 0
	off := Align(0, 0, 4, 10)
	if off != 0 {
		t.Fatalf("expected offset 0 at top, got %d", off)
	}

	// When sel=2, offset should be 2-2 = 0 (cursor at center)
	off = Align(2, off, 4, 10)
	if off != 0 {
		t.Fatalf("expected offset 0 to center sel=2, got %d", off)
	}

	// When sel=3, offset should be 3-2 = 1 (cursor at center)
	off = Align(3, off, 4, 10)
	if off != 1 {
		t.Fatalf("expected offset 1 to center sel=3, got %d", off)
	}

	// When sel=9, offset should be 9-2 = 7, but maxOff is 10-4=6, so clamped to 6
	off = Align(9, off, 4, 10)
	if off != 6 {
		t.Fatalf("expected offset to clamp to maxOff=6, got %d", off)
	}
}

func TestAlignPinsAtBounds(t *testing.T) {
	h := 4
	total := 6
	maxOff := total - h // maxOff = 2
	// sel=4, center=2, offset should be 4-2=2
	off := Align(4, 0, h, total)
	if off != maxOff {
		t.Fatalf("expected offset %d, got %d", maxOff, off)
	}
	// sel=5 (last item), offset should be 5-2=3, but maxOff=2, so clamped to 2
	off = Align(5, off, h, total)
	if off != maxOff {
		t.Fatalf("expected offset %d, got %d", maxOff, off)
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

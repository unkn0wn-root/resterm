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

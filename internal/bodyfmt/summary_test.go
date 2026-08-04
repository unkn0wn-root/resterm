package bodyfmt

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
)

func TestBinarySummaryLines(t *testing.T) {
	pl := Payload{
		Meta: binaryview.Meta{
			Kind:       binaryview.KindBinary,
			MIME:       "image/png",
			DecodeErr:  "bad charset",
			PreviewHex: "89 50 4e 47",
			PreviewB64: "iVBOR",
		},
		Size: 2048,
	}

	lines := pl.BinarySummary()
	wantKinds := []SummaryKind{
		SummaryTitle,
		SummaryField,
		SummaryWarn,
		SummaryPreview,
		SummaryPreview,
		SummaryModes,
	}
	if len(lines) != len(wantKinds) {
		t.Fatalf("BinarySummary() returned %d lines, want %d", len(lines), len(wantKinds))
	}
	for i, kind := range wantKinds {
		if lines[i].Kind != kind {
			t.Fatalf("line %d kind=%v, want %v", i, lines[i].Kind, kind)
		}
	}
	if lines[0].Value != "Binary body (2 KiB)" {
		t.Fatalf("title=%q, want %q", lines[0].Value, "Binary body (2 KiB)")
	}
}

func TestBinarySummaryTextMatchesLines(t *testing.T) {
	pl := Payload{
		Meta: binaryview.Meta{Kind: binaryview.KindBinary, MIME: "image/png"},
		Size: 1024,
	}
	want := "Binary body (1 KiB)\nMIME: image/png\nRaw view: hex / base64"
	if got := pl.BinarySummaryText(); got != want {
		t.Fatalf("BinarySummaryText()=%q, want %q", got, want)
	}
}

func TestBinarySummarySkipsBlankFields(t *testing.T) {
	pl := Payload{Meta: binaryview.Meta{Kind: binaryview.KindBinary, MIME: "  "}, Size: 8}
	for _, line := range pl.BinarySummary() {
		if line.Kind == SummaryField {
			t.Fatalf("expected blank MIME to be skipped, got %q", line)
		}
	}
}

func TestRawSummaryTextIncludesMIME(t *testing.T) {
	pl := Payload{
		Meta: binaryview.Meta{Kind: binaryview.KindBinary, MIME: "application/pdf"},
		Size: 3 * 1024 * 1024,
	}
	got := pl.RawSummaryText()
	if !strings.HasPrefix(got, "Binary body (3 MiB, application/pdf)\n") {
		t.Fatalf("RawSummaryText()=%q, want size and MIME in the title", got)
	}
	if !strings.Contains(got, "<raw dump deferred>") {
		t.Fatalf("RawSummaryText()=%q, want deferred marker", got)
	}
}

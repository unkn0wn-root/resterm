package binaryview

import "testing"

func TestFilenameHintDisposition(t *testing.T) {
	name := FilenameHint(`attachment; filename="report.pdf"`, "", "application/pdf")
	if name != "report.pdf" {
		t.Fatalf("expected disposition filename, got %q", name)
	}
}

func TestFilenameHintURLFallback(t *testing.T) {
	name := FilenameHint("", "https://example.com/files/image.png", "application/octet-stream")
	if name != "image.png" {
		t.Fatalf("expected URL filename, got %q", name)
	}
}

func TestFilenameHintMimeExtension(t *testing.T) {
	name := FilenameHint("", "", "application/json")
	if name != "response.json" {
		t.Fatalf("expected mime-based filename, got %q", name)
	}
}

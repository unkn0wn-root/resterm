package binaryview

import "testing"

func TestFilenameHintDisposition(t *testing.T) {
	name := FilenameHint(`attachment; filename="report.pdf"`, "", "application/pdf")
	if name != "report.pdf" {
		t.Fatalf("expected disposition filename, got %q", name)
	}
}

func TestFilenameHintDispositionRFC5987(t *testing.T) {
	name := FilenameHint(
		`attachment; filename*=UTF-8''d%20%5Ba%5D.txt`,
		"",
		"application/octet-stream",
	)
	if name != "d [a].txt" {
		t.Fatalf("expected decoded RFC 5987 filename, got %q", name)
	}
}

func TestFilenameHintDispositionPrefersDecoded(t *testing.T) {
	h := `attachment; filename*=UTF-8''cool%20name.txt; filename="fallback.txt"`
	name := FilenameHint(h, "", "application/octet-stream")
	if name != "cool name.txt" {
		t.Fatalf("expected filename* to win over filename, got %q", name)
	}
}

func TestFilenameHintURLFallback(t *testing.T) {
	name := FilenameHint("", "https://example.com/files/image.png", "application/octet-stream")
	if name != "image.png" {
		t.Fatalf("expected URL filename, got %q", name)
	}
}

func TestFilenameHintURLRootIgnored(t *testing.T) {
	name := FilenameHint("", "https://example.com/", "application/json")
	if name != "response.json" {
		t.Fatalf("expected fallback filename when URL path is root, got %q", name)
	}
}

func TestFilenameHintMimeExtension(t *testing.T) {
	name := FilenameHint("", "", "application/json")
	if name != "response.json" {
		t.Fatalf("expected mime-based filename, got %q", name)
	}
}

func TestFilenameHintMimeExtensionWithParams(t *testing.T) {
	name := FilenameHint("", "", "text/html; charset=utf-8")
	if name != "response.htm" && name != "response.html" {
		t.Fatalf("expected mime-based filename from parameterized type, got %q", name)
	}
}

func TestFilenameHintSanitize(t *testing.T) {
	name := FilenameHint("", "https://example.com/../../etc/passwd", "application/octet-stream")
	if name != "passwd.bin" {
		t.Fatalf("expected sanitized basename with fallback extension, got %q", name)
	}
}

package binaryview

import (
	"strings"
	"testing"
)

func TestBodyNameDisposition(t *testing.T) {
	name := Body{Disposition: `attachment; filename="report.pdf"`, ContentType: "application/pdf"}.Name()
	if name != "report.pdf" {
		t.Fatalf("expected disposition filename, got %q", name)
	}
}

func TestBodyNameDispositionRFC5987(t *testing.T) {
	name := Body{
		Disposition: `attachment; filename*=UTF-8''d%20%5Ba%5D.txt`,
		ContentType: "application/octet-stream",
	}.Name()
	if name != "d [a].txt" {
		t.Fatalf("expected decoded RFC 5987 filename, got %q", name)
	}
}

func TestBodyNameDispositionPrefersDecoded(t *testing.T) {
	h := `attachment; filename*=UTF-8''cool%20name.txt; filename="fallback.txt"`
	name := Body{Disposition: h, ContentType: "application/octet-stream"}.Name()
	if name != "cool name.txt" {
		t.Fatalf("expected filename* to win over filename, got %q", name)
	}
}

func TestBodyNameURLFallback(t *testing.T) {
	name := Body{URL: "https://example.com/files/image.png", ContentType: "application/octet-stream"}.Name()
	if name != "image.png" {
		t.Fatalf("expected URL filename, got %q", name)
	}
}

func TestBodyNameURLRootIgnored(t *testing.T) {
	name := Body{URL: "https://example.com/", ContentType: "application/json"}.Name()
	if name != "response.json" {
		t.Fatalf("expected fallback filename when URL path is root, got %q", name)
	}
}

func TestBodyNameMimeExtension(t *testing.T) {
	name := Body{ContentType: "application/json"}.Name()
	if name != "response.json" {
		t.Fatalf("expected mime-based filename, got %q", name)
	}
}

func TestBodyNameMimeExtensionWithParams(t *testing.T) {
	name := Body{ContentType: "text/html; charset=utf-8"}.Name()
	if name != "response.html" {
		t.Fatalf("expected mime-based filename from parameterized type, got %q", name)
	}
}

func TestBodyNameSanitize(t *testing.T) {
	name := Body{URL: "https://example.com/../../etc/passwd", ContentType: "application/octet-stream"}.Name()
	if name != "passwd.bin" {
		t.Fatalf("expected sanitized basename with fallback extension, got %q", name)
	}
}

func TestBodyNameCuratedExtensions(t *testing.T) {
	tests := map[string]string{
		"application/problem+json":      "response.json",
		"application/vnd.api+json":      "response.json",
		"application/soap+xml":          "response.xml",
		"audio/mpeg":                    "response.mp3",
		"application/vnd.ms-excel":      "response.xls",
		"application/vnd.ms-powerpoint": "response.ppt",
	}
	for ct, want := range tests {
		if got := (Body{ContentType: ct}).Name(); got != want {
			t.Errorf("Name(%q) = %q, want %q", ct, got, want)
		}
	}
}

func TestBodyViewerName(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	exe := []byte("MZ\x90\x00\x03\x00\x00\x00\x04\x00\x00\x00\xff\xff\x00\x00")
	tests := []struct {
		name string
		body Body
		want string
		ok   bool
	}{
		{
			name: "declared type replaces server extension",
			body: Body{
				Data:        []byte("%PDF-1.7"),
				ContentType: "application/pdf",
				Disposition: `attachment; filename="invoice.exe"`,
			},
			want: "invoice.pdf", ok: true,
		},
		{
			name: "installer url with json body",
			body: Body{
				Data:        []byte(`{}`),
				ContentType: "application/json",
				URL:         "https://api.example.com/download/setup.msi",
			},
			want: "setup.json", ok: true,
		},
		{
			name: "domain-like url segment",
			body: Body{
				Data:        []byte(`{}`),
				ContentType: "application/json",
				URL:         "https://api.example.com/users/me@example.com",
			},
			want: "me_example.json", ok: true,
		},
		{
			name: "unknown url extension",
			body: Body{Data: png, ContentType: "image/png", URL: "https://api.example.com/reports/q3.v2"},
			want: "q3.png", ok: true,
		},
		{
			name: "server name gives the type",
			body: Body{
				Data:        []byte("PK\x03\x04"),
				ContentType: "application/octet-stream",
				Disposition: `attachment; filename="report.docx"`,
			},
			want: "report.docx", ok: true,
		},
		{
			name: "sniffed type",
			body: Body{Data: png, ContentType: "application/octet-stream", URL: "https://api.example.com/blob/42"},
			want: "42.png", ok: true,
		},
		{
			name: "structured suffix",
			body: Body{Data: []byte(`{"title":"x"}`), ContentType: "application/problem+json"},
			want: "response.json", ok: true,
		},
		{
			name: "text without a known type",
			body: Body{Data: []byte("a=1\n"), ContentType: "application/x-www-form-urlencoded"},
			want: "response.txt", ok: true,
		},
		{
			name: "html opens as text",
			body: Body{
				Data:        []byte("<html><script>alert(1)</script></html>"),
				ContentType: "text/html; charset=utf-8",
				URL:         "https://api.example.com/status",
			},
			want: "status.txt", ok: true,
		},
		{
			name: "svg opens as text",
			body: Body{Data: []byte("<svg><script>alert(1)</script></svg>"), ContentType: "image/svg+xml"},
			want: "response.txt", ok: true,
		},
		{
			name: "xml opens as text",
			body: Body{Data: []byte("<?xml version=\"1.0\"?><a/>"), ContentType: "application/soap+xml"},
			want: "response.txt", ok: true,
		},
		{
			name: "markdown opens as text",
			body: Body{Data: []byte("# Title"), ContentType: "text/markdown"},
			want: "response.txt", ok: true,
		},
		{
			name: "sniffed html opens as text",
			body: Body{
				Data:        []byte("<!DOCTYPE html><script>alert(1)</script>"),
				ContentType: "application/octet-stream",
			},
			want: "response.txt", ok: true,
		},
		{
			name: "html file name opens as text",
			body: Body{
				Data:        []byte{0x00, 0x01, 0x02},
				ContentType: "application/octet-stream",
				Disposition: `attachment; filename="page.html"`,
			},
			want: "page.txt", ok: true,
		},
		{
			name: "executable named as one",
			body: Body{
				Data:        exe,
				ContentType: "application/octet-stream",
				Disposition: `attachment; filename="run.exe"`,
			},
		},
		{
			name: "macro-capable office type",
			body: Body{Data: []byte("\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"), ContentType: "application/vnd.ms-excel"},
		},
		{
			name: "grpc wire bytes",
			body: Body{Data: []byte{0x00, 0x00, 0x00, 0x00, 0x02, 0x08, 0x01}, ContentType: "application/grpc"},
		},
		{
			name: "long unicode name",
			body: Body{
				Data:        []byte(`{}`),
				ContentType: "application/json",
				Disposition: `attachment; filename*=UTF-8''%C3%BCber%20` + strings.Repeat("a", 70) + ".json",
			},
			want: "über " + strings.Repeat("a", 59) + ".json", ok: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.body.ViewerName()
			if got != tt.want || ok != tt.ok {
				t.Fatalf("ViewerName() = %q, %v, want %q, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestBodyNamesAvoidWindowsDevices(t *testing.T) {
	tests := []struct {
		body   Body
		name   string
		viewer string
	}{
		{Body{Disposition: `attachment; filename="CON.pdf"`, ContentType: "application/pdf"}, "_CON.pdf", "_CON.pdf"},
		{Body{URL: "https://api.example.com/v1/aux", ContentType: "application/json"}, "_aux.json", "_aux.json"},
		{Body{Disposition: `attachment; filename="nul .txt"`, ContentType: "text/plain"}, "_nul .txt", "_nul.txt"},
		{
			Body{Disposition: `attachment; filename="lpt9.tar.gz"`, ContentType: "application/gzip"},
			"_lpt9.tar.gz",
			"_lpt9.tar.gz",
		},
		{
			Body{Disposition: `attachment; filename*=UTF-8''com%C2%B9.txt`, ContentType: "text/plain"},
			"_com¹.txt",
			"_com_.txt",
		},
		{
			Body{Disposition: `attachment; filename="console.log"`, ContentType: "text/plain"},
			"console.log",
			"console.txt",
		},
		{
			Body{
				Disposition: `attachment; filename="CON` + strings.Repeat(" ", 61) + `x.json"`,
				ContentType: "application/json",
			},
			"CON" + strings.Repeat(" ", 61) + "x.json",
			"_CON.json",
		},
	}
	for _, tt := range tests {
		if got := tt.body.Name(); got != tt.name {
			t.Errorf("Name(%q) = %q, want %q", tt.body.Disposition+tt.body.URL, got, tt.name)
		}
		if got, _ := tt.body.ViewerName(); got != tt.viewer {
			t.Errorf("ViewerName(%q) = %q, want %q", tt.body.Disposition+tt.body.URL, got, tt.viewer)
		}
	}
}

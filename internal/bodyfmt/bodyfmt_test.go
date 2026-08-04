package bodyfmt

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
)

func build(in BuildInput) BodyViews {
	return Build(context.Background(), in)
}

func TestBuildJSONPlainText(t *testing.T) {
	views := build(BuildInput{
		Body:        []byte(`{"b":1,"a":"x"}`),
		ContentType: "application/json",
	})
	if strings.Contains(views.Pretty, "\x1b[") {
		t.Fatalf("expected plain pretty output, got %q", views.Pretty)
	}
	if !strings.Contains(views.Pretty, `a: "x"`) {
		t.Fatalf("expected formatted json body, got %q", views.Pretty)
	}
}

func TestBuildBinaryDefaultsToHex(t *testing.T) {
	views := build(BuildInput{
		Body:        []byte{0x00, 0x01, 0x02, 0x03},
		ContentType: "application/octet-stream",
	})
	if views.Mode != RawHex {
		t.Fatalf("Build(...).Mode=%v, want %v", views.Mode, RawHex)
	}
	if !strings.Contains(views.Pretty, "Binary body") {
		t.Fatalf("expected binary summary, got %q", views.Pretty)
	}
	if views.RawHex == "" || views.Raw != views.RawHex {
		t.Fatalf("expected default raw hex view, got raw=%q hex=%q", views.Raw, views.RawHex)
	}
}

func TestBuildHeavyBinaryDefersRawDump(t *testing.T) {
	body := make([]byte, RawHeavyLimit+1)
	for i := range body {
		body[i] = byte(i % 7)
	}

	views := build(BuildInput{Body: body, ContentType: "application/octet-stream"})
	if views.Mode != RawSummary {
		t.Fatalf("Build(...).Mode=%v, want %v", views.Mode, RawSummary)
	}
	if views.RawHex != "" || views.RawBase64 != "" {
		t.Fatalf("expected heavy body to skip dumps, got hex=%d b64=%d bytes",
			len(views.RawHex), len(views.RawBase64))
	}
	if !strings.Contains(views.Raw, "<raw dump deferred>") {
		t.Fatalf("expected deferred raw text, got %q", views.Raw)
	}
}

func TestBuildEmptyBodyUsesPlaceholder(t *testing.T) {
	views := build(BuildInput{ContentType: "text/plain"})
	if views.Pretty != placeholder || views.Raw != placeholder {
		t.Fatalf("Build(...) pretty=%q raw=%q, want %q for both",
			views.Pretty, views.Raw, placeholder)
	}
}

func TestBuildCancelledContextSkipsFormatting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	body := `{"b":1,"a":"x"}`
	views := Build(ctx, BuildInput{Body: []byte(body), ContentType: "application/json"})
	if views.Pretty != body {
		t.Fatalf("expected cancelled build to return source body, got %q", views.Pretty)
	}
}

func TestBuildViewBodyOverridesBinaryVerdict(t *testing.T) {
	views := build(BuildInput{
		Body:            []byte{0x00, 0x01, 0x02, 0x03},
		ContentType:     "application/grpc",
		ViewBody:        []byte(`{"a":1}`),
		ViewContentType: "application/json",
	})
	if views.Meta.Kind != binaryview.KindText {
		t.Fatalf("expected text verdict from view body, got %v", views.Meta.Kind)
	}
	if views.Mode != RawText {
		t.Fatalf("Build(...).Mode=%v, want %v", views.Mode, RawText)
	}
	if !strings.Contains(views.Pretty, "a: 1") {
		t.Fatalf("expected view body to be prettified, got %q", views.Pretty)
	}
	if views.ContentType != "application/json" {
		t.Fatalf("Build(...).ContentType=%q, want view content type", views.ContentType)
	}
}

func TestBuildJSONColorUsesResolvedFormatter(t *testing.T) {
	views := build(BuildInput{
		Body:        []byte(`{"b":1,"a":"x"}`),
		ContentType: "application/json",
		Color:       termcolor.Enabled(termenv.ANSI256),
	})
	if !strings.Contains(views.Pretty, "\x1b[") {
		t.Fatalf("expected colored pretty output, got %q", views.Pretty)
	}
	if got := ansi.Strip(views.Pretty); !strings.Contains(got, `a: "x"`) {
		t.Fatalf("expected colored output to preserve text, got %q", got)
	}
}

func TestBuildJSONColorRespectsConfiguredStyle(t *testing.T) {
	github := build(BuildInput{
		Body:        []byte(`{"b":1,"a":"x"}`),
		ContentType: "application/json",
		Color:       termcolor.Enabled(termenv.ANSI256),
		Style:       "github",
	})
	monokai := build(BuildInput{
		Body:        []byte(`{"b":1,"a":"x"}`),
		ContentType: "application/json",
		Color:       termcolor.Enabled(termenv.ANSI256),
		Style:       "monokai",
	})

	if github.Pretty == monokai.Pretty {
		t.Fatalf("expected different ANSI output for different styles")
	}
	if ansi.Strip(github.Pretty) != ansi.Strip(monokai.Pretty) {
		t.Fatalf("expected different styles to preserve the same body text")
	}
}

func TestBuildXMLPrettyFormats(t *testing.T) {
	views := build(BuildInput{
		Body:        []byte(`<root><child>one</child></root>`),
		ContentType: "text/xml",
	})
	if !strings.Contains(views.Pretty, "\n  <child>one</child>\n") {
		t.Fatalf("expected formatted XML pretty output, got %q", views.Pretty)
	}
	if !strings.Contains(views.RawText, "\n  <child>one</child>\n") {
		t.Fatalf("expected formatted XML raw output, got %q", views.RawText)
	}
}

func TestBuildMalformedXMLFallsBackToOriginalText(t *testing.T) {
	body := `<root><child></root>`
	views := build(BuildInput{
		Body:        []byte(body),
		ContentType: "application/xml",
	})
	if views.Pretty != body {
		t.Fatalf("expected malformed XML pretty fallback, got %q", views.Pretty)
	}
	if views.RawText != body {
		t.Fatalf("expected malformed XML raw fallback, got %q", views.RawText)
	}
}

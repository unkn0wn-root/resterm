package ui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/theme"
)

func TestMetadataRuneStylerNameDirective(t *testing.T) {
	palette := theme.DefaultTheme().EditorMetadata
	styler := newMetadataRuneStyler(palette)

	line := []rune("# @name getUser")
	styles := styler.StylesForLine(line, 0)
	if styles == nil {
		t.Fatalf("expected styles for metadata line")
	}
	if got, want := styles[0].Render("#"), lipgloss.NewStyle().Foreground(palette.CommentMarker).Render("#"); got != want {
		t.Fatalf("comment marker style mismatch:\nwant %q\n got %q", want, got)
	}
	if got, want := styles[2].Render("@"), lipgloss.NewStyle().Foreground(palette.DirectiveColors["name"]).Bold(true).Render("@"); got != want {
		t.Fatalf("directive style mismatch:\nwant %q\n got %q", want, got)
	}
	if got, want := styles[8].Render("g"), lipgloss.NewStyle().Foreground(palette.Value).Render("g"); got != want {
		t.Fatalf("value style mismatch:\nwant %q\n got %q", want, got)
	}
}

func TestMetadataRuneStylerSettingDirective(t *testing.T) {
	palette := theme.DefaultTheme().EditorMetadata
	styler := newMetadataRuneStyler(palette)

	line := []rune("# @setting timeout 5s")
	styles := styler.StylesForLine(line, 0)
	if styles == nil {
		t.Fatalf("expected styles for metadata line")
	}
	if got, want := styles[2].Render("@"), lipgloss.NewStyle().Foreground(palette.DirectiveColors["setting"]).Bold(true).Render("@"); got != want {
		t.Fatalf("directive style mismatch:\nwant %q\n got %q", want, got)
	}
	if got, want := styles[11].Render("t"), lipgloss.NewStyle().Foreground(palette.SettingKey).Bold(true).Render("t"); got != want {
		t.Fatalf("setting key style mismatch:\nwant %q\n got %q", want, got)
	}
	if got, want := styles[19].Render("5"), lipgloss.NewStyle().Foreground(palette.SettingValue).Render("5"); got != want {
		t.Fatalf("setting value style mismatch:\nwant %q\n got %q", want, got)
	}
}

func TestMetadataRuneStylerRequestLines(t *testing.T) {
	palette := theme.DefaultTheme().EditorMetadata
	styler := newMetadataRuneStyler(palette)
	color := palette.RequestLine
	if color == "" {
		t.Fatal("expected auth color in palette")
	}
	expected := lipgloss.NewStyle().Foreground(color).Bold(true).Render("P")

	httpLine := []rune("POST https://api.example.com")
	styles := styler.StylesForLine(httpLine, 0)
	if styles == nil {
		t.Fatalf("expected styles for HTTP request line")
	}
	if got := styles[0].Render("P"); got != expected {
		t.Fatalf("HTTP request style mismatch:\nwant %q\n got %q", expected, got)
	}

	grpcLine := []rune("GRPC localhost:50051")
	styles = styler.StylesForLine(grpcLine, 0)
	if styles == nil {
		t.Fatalf("expected styles for gRPC request line")
	}
	if got := styles[0].Render("G"); got != lipgloss.NewStyle().Foreground(color).Bold(true).Render("G") {
		t.Fatalf("gRPC request style mismatch:\nwant %q\n got %q", expected, got)
	}
}

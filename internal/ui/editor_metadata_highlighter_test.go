package ui

import (
	"slices"
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
	if got, want := styles[0].Render(
		"#",
	), lipgloss.NewStyle().
		Foreground(palette.CommentMarker).
		Render("#"); got != want {
		t.Fatalf("comment marker style mismatch:\nwant %q\n got %q", want, got)
	}
	if got, want := styles[2].Render(
		"@",
	), lipgloss.NewStyle().
		Foreground(palette.DirectiveColors["name"]).
		Bold(true).
		Render("@"); got != want {
		t.Fatalf("directive style mismatch:\nwant %q\n got %q", want, got)
	}
	if got, want := styles[8].Render(
		"g",
	), lipgloss.NewStyle().
		Foreground(palette.Value).
		Render("g"); got != want {
		t.Fatalf("value style mismatch:\nwant %q\n got %q", want, got)
	}
}

// The colon in "@name: value" is a separator, so it must not be styled as part
// of the value. Parse accepts both spellings and the editor has to agree.
func TestMetadataRuneStylerColonSeparator(t *testing.T) {
	palette := theme.DefaultTheme().EditorMetadata
	if palette.Value == palette.DirectiveDefault {
		t.Fatal("value and directive colors must differ for this test to mean anything")
	}

	tests := []struct {
		name string
		line string
		want []int
	}{
		{name: "space", line: "# @name getUser", want: []int{8, 9, 10, 11, 12, 13, 14}},
		{name: "colon", line: "# @name:getUser", want: []int{8, 9, 10, 11, 12, 13, 14}},
		{name: "colon and space", line: "# @name: getUser", want: []int{9, 10, 11, 12, 13, 14, 15}},
		{name: "rest mode colon", line: "# @desc:some text", want: []int{8, 9, 10, 11, 12, 13, 14, 15, 16}},
		// A non-breaking space is a separator too, so the value starts past it.
		{name: "unicode space", line: "# @name\u00a0getUser", want: []int{8, 9, 10, 11, 12, 13, 14}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			styler := newMetadataRuneStyler(palette)
			styles := styler.StylesForLine([]rune(tt.line), 0)
			if styles == nil {
				t.Fatalf("expected styles for %q", tt.line)
			}
			var got []int
			for i := range styles {
				if styles[i].GetForeground() == palette.Value {
					got = append(got, i)
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("StylesForLine(%q) value indices = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestDirectiveValueModesComeFromCatalog(t *testing.T) {
	tests := map[string]metadataValueMode{
		"name":      metadataValueModeToken,
		"desc":      metadataValueModeRest,
		"nolog":     metadataValueModeNone,
		"settings":  metadataValueModeRest,
		"websocket": metadataValueModeRest,
	}
	for name, want := range tests {
		got, ok := directiveValueMode(name)
		if !ok || got != want {
			t.Fatalf("directiveValueMode(%q) = (%d, %t), want (%d, true)",
				name, got, ok, want)
		}
	}
	if _, ok := directiveValueMode("grpc-method"); ok {
		t.Fatal("unknown directive unexpectedly has a catalog mode")
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
	if got, want := styles[2].Render(
		"@",
	), lipgloss.NewStyle().
		Foreground(palette.DirectiveColors["setting"]).
		Bold(true).
		Render("@"); got != want {
		t.Fatalf("directive style mismatch:\nwant %q\n got %q", want, got)
	}
	if got, want := styles[11].Render(
		"t",
	), lipgloss.NewStyle().
		Foreground(palette.SettingKey).
		Bold(true).
		Render("t"); got != want {
		t.Fatalf("setting key style mismatch:\nwant %q\n got %q", want, got)
	}
	if got, want := styles[19].Render(
		"5",
	), lipgloss.NewStyle().
		Foreground(palette.SettingValue).
		Render("5"); got != want {
		t.Fatalf("setting value style mismatch:\nwant %q\n got %q", want, got)
	}
}

func TestMetadataRuneStylerRequestLines(t *testing.T) {
	palette := theme.DefaultTheme().EditorMetadata
	styler := newMetadataRuneStyler(palette)
	color := palette.RequestLine
	if color == "" {
		t.Fatal("expected request line color in palette")
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
	if got := styles[0].Render(
		"G",
	); got != lipgloss.NewStyle().
		Foreground(color).
		Bold(true).
		Render("G") {
		t.Fatalf("gRPC request style mismatch:\nwant %q\n got %q", expected, got)
	}

	wsLine := []rune("WS wss://stream.example.com")
	styles = styler.StylesForLine(wsLine, 0)
	if styles == nil {
		t.Fatalf("expected styles for WebSocket request line")
	}
	wsWant := lipgloss.NewStyle().Foreground(color).Bold(true).Render("W")
	if got := styles[0].Render("W"); got != wsWant {
		t.Fatalf("WebSocket request style mismatch:\nwant %q\n got %q", wsWant, got)
	}
}

func TestMetadataRuneStylerRequestSeparator(t *testing.T) {
	palette := theme.DefaultTheme().EditorMetadata
	styler := newMetadataRuneStyler(palette)
	color := palette.RequestSeparator
	if color == "" {
		t.Fatal("expected request separator color in palette")
	}

	line := []rune("### graphql list items")
	styles := styler.StylesForLine(line, 0)
	if styles == nil {
		t.Fatalf("expected styles for request separator")
	}
	if got, want := styles[0].Render(
		"#",
	), lipgloss.NewStyle().
		Foreground(color).
		Bold(true).
		Render("#"); got != want {
		t.Fatalf("request separator style mismatch:\nwant %q\n got %q", want, got)
	}
	if got, want := styles[5].Render(
		"g",
	), lipgloss.NewStyle().
		Foreground(color).
		Bold(true).
		Render("g"); got != want {
		t.Fatalf("request separator text not styled uniformly:\nwant %q\n got %q", want, got)
	}

	lineNoSpace := []rune("###")
	styles = styler.StylesForLine(lineNoSpace, 0)
	if styles == nil {
		t.Fatalf("expected styles for compact request separator")
	}
	if got, want := styles[0].Render(
		"#",
	), lipgloss.NewStyle().
		Foreground(color).
		Bold(true).
		Render("#"); got != want {
		t.Fatalf(
			"request separator without space not styled correctly:\nwant %q\n got %q",
			want,
			got,
		)
	}
}

package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/directive"
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

func TestDirectiveArgumentKindsComeFromCatalog(t *testing.T) {
	tests := map[string]directive.ArgKind{
		"name":      directive.ArgToken,
		"desc":      directive.ArgText,
		"nolog":     directive.ArgNone,
		"setting":   directive.ArgSetting,
		"settings":  directive.ArgOptions,
		"timeout":   directive.ArgToken,
		"websocket": directive.ArgOptions,
	}
	for name, want := range tests {
		if got := directiveArgKind(directive.Name(name)); got != want {
			t.Fatalf("directiveArgKind(%q) = %d, want %d", name, got, want)
		}
	}
	if got := directiveArgKind("grpc-method"); got != directive.ArgToken {
		t.Fatalf("unknown directive kind = %d, want the ArgToken fallback", got)
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

func TestMetadataRuneStylerSettingEqualsDirective(t *testing.T) {
	palette := theme.DefaultTheme().EditorMetadata
	styler := newMetadataRuneStyler(palette)

	tests := []struct {
		name  string
		line  string
		value string
	}{
		{name: "value", line: "# @setting timeout=5s", value: "5s"},
		{name: "empty value", line: "# @setting timeout="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			styles := styler.StylesForLine([]rune(tt.line), 0)
			assertMetadataTokenColor(t, styles, tt.line, "timeout", palette.SettingKey)
			assertMetadataTokenNotColor(t, styles, tt.line, "=", palette.SettingKey)
			if tt.value != "" {
				assertMetadataTokenColor(t, styles, tt.line, tt.value, palette.SettingValue)
			}
		})
	}
}

func TestMetadataRuneStylerOptionDirectives(t *testing.T) {
	palette := theme.DefaultTheme().EditorMetadata
	styler := newMetadataRuneStyler(palette)

	tests := []struct {
		name        string
		line        string
		keys        []string
		values      []string
		positionals []string
	}{
		{
			name:   "settings",
			line:   "# @settings timeout=5s insecure=false",
			keys:   []string{"timeout", "insecure"},
			values: []string{"5s", "false"},
		},
		{
			name:        "ssh",
			line:        `# @ssh "file edge" host=jump timeout="5 s" persist`,
			keys:        []string{"host", "timeout"},
			values:      []string{"jump", `"5 s"`},
			positionals: []string{`"file edge"`, "persist"},
		},
		{
			name:   "json value",
			line:   `# @mock json={"name":"hello resterm"} status=200`,
			keys:   []string{"json", "status"},
			values: []string{`{"name":"hello resterm"}`, "200"},
		},
		{
			name:   "escaped space",
			line:   `# @ssh host=hello\ resterm timeout=5s`,
			keys:   []string{"host", "timeout"},
			values: []string{`hello\ resterm`, "5s"},
		},
		{
			name:        "comparison is positional",
			line:        "# @else last.statusCode==200 run=next",
			keys:        []string{"run"},
			values:      []string{"next"},
			positionals: []string{"last.statusCode==200"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			styles := styler.StylesForLine([]rune(tt.line), 0)
			for _, key := range tt.keys {
				assertMetadataTokenColor(t, styles, tt.line, key, palette.SettingKey)
			}
			for _, value := range tt.values {
				assertMetadataTokenColor(t, styles, tt.line, value, palette.SettingValue)
			}
			for _, positional := range tt.positionals {
				assertMetadataTokenColor(t, styles, tt.line, positional, palette.Value)
			}
		})
	}
}

func TestMetadataRuneStylerTimeoutUsesGenericValue(t *testing.T) {
	palette := theme.DefaultTheme().EditorMetadata
	if palette.Value == palette.SettingValue {
		t.Fatal("generic and setting value colors must differ for this test to mean anything")
	}

	line := "# @timeout 5s"
	styles := newMetadataRuneStyler(palette).StylesForLine([]rune(line), 0)
	assertMetadataTokenColor(t, styles, line, "5s", palette.Value)
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

// The returned range is in runes, matching how StylesForLine indexes the line.
func metadataTokenRange(t *testing.T, styles []lipgloss.Style, line, token string) (int, int) {
	t.Helper()
	before, _, ok := strings.Cut(line, token)
	if !ok {
		t.Fatalf("token %q is not in %q", token, line)
	}
	start := len([]rune(before))
	end := start + len([]rune(token))
	if len(styles) < end {
		t.Fatalf("StylesForLine(%q) returned %d styles, need %d", line, len(styles), end)
	}
	return start, end
}

func assertMetadataTokenColor(t *testing.T, styles []lipgloss.Style, line, token string, want lipgloss.Color) {
	t.Helper()
	start, end := metadataTokenRange(t, styles, line, token)
	for i := start; i < end; i++ {
		if got := styles[i].GetForeground(); got != want {
			t.Fatalf("StylesForLine(%q) token %q rune %d color = %q, want %q", line, token, i-start, got, want)
		}
	}
}

func assertMetadataTokenNotColor(t *testing.T, styles []lipgloss.Style, line, token string, unwanted lipgloss.Color) {
	t.Helper()
	start, end := metadataTokenRange(t, styles, line, token)
	for i := start; i < end; i++ {
		if got := styles[i].GetForeground(); got == unwanted {
			t.Fatalf(
				"StylesForLine(%q) token %q rune %d color = %q, do not want %q",
				line,
				token,
				i-start,
				got,
				unwanted,
			)
		}
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

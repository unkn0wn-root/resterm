package ui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/theme"
)

func TestTruncateStatusMaintainsUTF8(t *testing.T) {
	input := "Env: 東京dev"
	truncated := truncateStatus(input, 9)
	if !utf8.ValidString(truncated) {
		t.Fatalf("truncateStatus returned invalid UTF-8: %q", truncated)
	}
}

func TestTruncateStatusTooSmallForContent(t *testing.T) {
	width := 2
	got := truncateStatus("abcd", width)
	if got != "…" {
		t.Fatalf("expected ellipsis when width %d is too small, got %q", width, got)
	}
}

func TestTruncateStatusStaysWithinWidth(t *testing.T) {
	width := 6
	got := truncateStatus("Env: prod", width)
	maxWidth := maxInt(width-2, 1)
	if lipgloss.Width(got) > maxWidth {
		t.Fatalf("truncateStatus exceeded max width %d: %q (width %d)", maxWidth, got, lipgloss.Width(got))
	}
}

func TestRenderStatusBarLongMessagePreservesStaticFields(t *testing.T) {
	m := Model{
		theme: theme.DefaultTheme(),
		cfg: Config{
			EnvironmentName: "prod",
		},
		currentFile:      "/tmp/request.http",
		statusMessage:    statusMsg{text: strings.Repeat("status message ", 8)},
		focus:            focusEditor,
		editorInsertMode: false,
		width:            70,
	}

	view := m.renderStatusBar()
	if lipgloss.Width(view) > m.width {
		t.Fatalf("status bar width %d exceeds frame width %d", lipgloss.Width(view), m.width)
	}

	trimmed := strings.Trim(view, " ")
	for _, segment := range []string{"Env: prod", "request.http", "Focus: Editor", "Mode: VIEW"} {
		if !strings.Contains(trimmed, segment) {
			t.Fatalf("expected status bar to include %q in %q", segment, trimmed)
		}
	}

	envIndex := strings.Index(trimmed, "Env: prod")
	if envIndex == -1 {
		t.Fatalf("expected Env segment in %q", trimmed)
	}
	prefix := trimmed[:envIndex]
	const sep = "    "
	if !strings.HasSuffix(prefix, "…"+sep) {
		t.Fatalf("expected message to end with ellipsis before static fields, got %q", prefix)
	}
	messagePart := strings.TrimSuffix(prefix, sep)
	if messagePart == "" {
		t.Fatalf("expected truncated message before static fields, got %q", prefix)
	}
}

func TestRenderPaneTabsClampsHeight(t *testing.T) {
	model := Model{
		theme:             theme.DefaultTheme(),
		focus:             focusResponse,
		responsePaneFocus: responsePanePrimary,
		wsConsole:         &websocketConsole{},
		responsePanes: [2]responsePaneState{
			{activeTab: responseTabPretty, followLatest: false},
		},
	}

	view := model.renderPaneTabs(responsePanePrimary, true, 12)
	if got := lipgloss.Height(view); got != 2 {
		t.Fatalf("expected tab bar height to remain 2 lines, got %d", got)
	}
}

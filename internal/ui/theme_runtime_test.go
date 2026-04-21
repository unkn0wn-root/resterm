package ui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/theme"
)

func TestThemeRuntimeInactiveStyleUsesFaintOnlyForNonLightThemes(t *testing.T) {
	base := lipgloss.NewStyle().Foreground(lipgloss.Color("#123456"))

	dark := newThemeRuntime(theme.DefaultDefinition())
	if !dark.inactiveStyle(base).GetFaint() {
		t.Fatalf("expected default theme inactive style to stay faint")
	}

	light := newThemeRuntime(theme.Definition{
		Key: "daybreak",
		Metadata: theme.Metadata{
			Name: "Daybreak",
			Tags: []string{"light"},
		},
		Theme: theme.DefaultTheme(),
	})
	if light.inactiveStyle(base).GetFaint() {
		t.Fatalf("expected light theme inactive style to avoid faint")
	}
}

func TestApplyThemeDefinitionStylesGenericInputsOnlyForLightThemes(t *testing.T) {
	model := New(Config{})

	model.applyThemeDefinition(theme.DefaultDefinition())
	if colorDefined(model.searchInput.TextStyle.GetForeground()) {
		t.Fatalf("expected dark generic input text style to stay unset")
	}

	lightTheme := theme.DefaultTheme()
	lightTheme.HeaderTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("#1e40af"))
	lightTheme.ExplainMuted = lipgloss.NewStyle().Foreground(lipgloss.Color("#64748b"))
	lightTheme.PaneActiveForeground = lipgloss.Color("#0f172a")
	lightTheme.NavigatorTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("#0f172a"))
	lightTheme.NavigatorSubtitle = lipgloss.NewStyle().Foreground(lipgloss.Color("#475569"))
	lightTheme.HeaderValue = lipgloss.NewStyle().Foreground(lipgloss.Color("#0f172a"))

	model.applyThemeDefinition(theme.Definition{
		Key: "daybreak",
		Metadata: theme.Metadata{
			Name: "Daybreak",
			Tags: []string{"light"},
		},
		Theme: lightTheme,
	})

	if got := model.searchInput.PromptStyle.GetForeground(); got != lipgloss.Color("#1e40af") {
		t.Fatalf("expected light prompt foreground, got %v", got)
	}
	if got := model.searchInput.TextStyle.GetForeground(); got != lipgloss.Color("#0f172a") {
		t.Fatalf("expected light input text foreground, got %v", got)
	}
	if got := model.historyFilterInput.PlaceholderStyle.GetForeground(); got != lipgloss.Color("#64748b") {
		t.Fatalf("expected history placeholder to use subtle light color, got %v", got)
	}
}

func TestResolveThemeDefinitionFallsBackToDefaultDefinition(t *testing.T) {
	def := resolveThemeDefinition(theme.Catalog{}, "", theme.DefaultTheme())
	if def.Key != "default" {
		t.Fatalf("expected default fallback key, got %q", def.Key)
	}
	if def.Appearance() != theme.AppearanceDark {
		t.Fatalf("expected default fallback appearance to be dark, got %v", def.Appearance())
	}
}

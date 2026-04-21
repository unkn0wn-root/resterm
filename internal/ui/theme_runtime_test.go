package ui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
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
	if got := model.historyFilterInput.PlaceholderStyle.GetForeground(); got != model.theme.HeaderValue.GetForeground() {
		t.Fatalf("expected dark history placeholder to keep header value foreground, got %v", got)
	}
	if got := model.themeRuntime.helpHintStyle(model.theme).GetForeground(); got != model.theme.HeaderValue.GetForeground() {
		t.Fatalf("expected dark help hint to keep header value foreground, got %v", got)
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

func TestThemeRuntimeDarkModalsPreserveLegacyColorsForCustomDarkThemes(t *testing.T) {
	customDark := theme.DefaultTheme()
	customDark.CommandBar = customDark.CommandBar.Background(lipgloss.Color("#102938"))
	customDark.ResponseSelection = customDark.ResponseSelection.Background(lipgloss.Color("#223344"))

	rt := newThemeRuntime(theme.Definition{
		Key: "aurora",
		Metadata: theme.Metadata{
			Name: "Aurora",
			Tags: []string{"dark"},
		},
		Theme: customDark,
	})

	if got := rt.modalBackdropColor(customDark); got != lipgloss.Color("#1A1823") {
		t.Fatalf("expected legacy dark modal backdrop, got %v", got)
	}
	if got := rt.modalInputBackground(customDark); got != lipgloss.Color("#1c1a23") {
		t.Fatalf("expected legacy dark modal input background, got %v", got)
	}
}

func TestApplyThemeDefinitionRerendersHTTPSnapshotsAndClearsPaneCaches(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prevProfile)

	model := New(Config{})
	resp := &httpclient.Response{
		Status:     "200 OK",
		StatusCode: 200,
		ReqMethod:  "GET",
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
		},
		Body:         []byte(`{"id":1,"name":"demo"}`),
		EffectiveURL: "https://api.example.com/items",
	}
	initial := buildHTTPResponseViews(resp, nil, nil)
	requestHeaders := buildHTTPRequestHeadersView(resp)
	snapshot := &responseSnapshot{
		id:             "snap",
		pretty:         initial.pretty,
		raw:            initial.raw,
		headers:        initial.headers,
		requestHeaders: requestHeaders,
		ready:          true,
		source:         newHTTPResponseRenderSource(resp, nil, nil),
	}
	model.responseLatest = snapshot
	model.responsePanes[responsePanePrimary].snapshot = snapshot
	model.responsePanes[responsePanePrimary].setCacheForTab(
		responseTabPretty,
		rawViewText,
		headersViewResponse,
		cachedWrap{width: 40, content: "stale", valid: true},
	)

	lightTheme := theme.DefaultTheme()
	lightTheme.ExplainMuted = lipgloss.NewStyle().Foreground(lipgloss.Color("#475569"))
	lightTheme.ExplainLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("#0369a1"))
	lightTheme.HeaderTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("#1d4ed8"))
	lightTheme.HeaderValue = lipgloss.NewStyle().Foreground(lipgloss.Color("#334155"))
	lightTheme.StatusBarKey = lipgloss.NewStyle().Foreground(lipgloss.Color("#b45309"))
	lightTheme.Success = lipgloss.NewStyle().Foreground(lipgloss.Color("#15803d"))
	lightTheme.Error = lipgloss.NewStyle().Foreground(lipgloss.Color("#b91c1c"))
	lightTheme.ResponseSelection = lipgloss.NewStyle().Background(lipgloss.Color("#e2e8f0"))
	lightTheme.PaneActiveForeground = lipgloss.Color("#0f172a")

	model.applyThemeDefinition(theme.Definition{
		Key: "daybreak",
		Metadata: theme.Metadata{
			Name: "Daybreak",
			Tags: []string{"light"},
		},
		Theme: lightTheme,
	})

	expectedRenderer := model.themeRuntime.responseRenderer(model.theme)
	expected := expectedRenderer.buildHTTPResponseViewsCtx(nil, resp, nil, nil)
	expectedRequestHeaders := expectedRenderer.buildHTTPRequestHeadersView(resp)

	if snapshot.pretty != expected.pretty {
		t.Fatalf("expected themed pretty response to rerender")
	}
	if snapshot.headers != expected.headers {
		t.Fatalf("expected themed headers response to rerender")
	}
	if snapshot.requestHeaders != expectedRequestHeaders {
		t.Fatalf("expected themed request headers to rerender")
	}
	if cache := model.responsePanes[responsePanePrimary].cacheForTab(
		responseTabPretty,
		rawViewText,
		headersViewResponse,
	); cache.valid {
		t.Fatalf("expected pane cache to be invalidated after theme change")
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

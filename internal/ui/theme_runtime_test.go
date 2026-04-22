package ui

import (
	"net/http"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/theme"
	"google.golang.org/grpc/codes"
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

func TestApplyThemeDefinitionStylesFiltersUseDistinctPromptAndTextColors(t *testing.T) {
	model := New(Config{})

	model.applyThemeDefinition(theme.DefaultDefinition())
	if theme.ColorDefined(model.searchInput.TextStyle.GetForeground()) {
		t.Fatalf("expected dark generic input text style to stay unset")
	}
	if got := model.helpFilter.TextStyle.GetForeground(); got != lipgloss.Color("#F5F2FF") {
		t.Fatalf("expected dark help filter text foreground, got %v", got)
	}
	if got := model.historyFilterInput.TextStyle.GetForeground(); got != lipgloss.Color("#F5F2FF") {
		t.Fatalf("expected dark history filter text foreground, got %v", got)
	}
	if got := model.historyFilterInput.PromptStyle.GetForeground(); got != lipgloss.Color("#A6A1BB") {
		t.Fatalf("expected dark history filter prompt foreground, got %v", got)
	}
	if theme.ColorDefined(model.historyFilterInput.PlaceholderStyle.GetForeground()) {
		t.Fatalf("expected dark history placeholder foreground to stay unset")
	}
	if !model.historyFilterInput.PlaceholderStyle.GetFaint() {
		t.Fatalf("expected dark history placeholder to stay faint")
	}
	if theme.ColorDefined(model.themeRuntime.helpHintStyle(model.theme).GetForeground()) {
		t.Fatalf("expected dark help hint foreground to stay unset")
	}
	if !model.themeRuntime.helpHintStyle(model.theme).GetFaint() {
		t.Fatalf("expected dark help hint to stay faint")
	}

	lightTheme := theme.DefaultTheme()
	lightTheme.HeaderTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("#1e40af"))
	lightTheme.ExplainMuted = lipgloss.NewStyle().Foreground(lipgloss.Color("#64748b"))
	lightTheme.PaneActiveForeground = lipgloss.Color("#0f172a")
	lightTheme.NavigatorTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("#0f172a"))
	lightTheme.NavigatorSubtitle = lipgloss.NewStyle().Foreground(lipgloss.Color("#475569"))
	lightTheme.HeaderValue = lipgloss.NewStyle().Foreground(lipgloss.Color("#475569"))

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
	if got := model.helpFilter.TextStyle.GetForeground(); got != lipgloss.Color("#0f172a") {
		t.Fatalf("expected light help filter text foreground, got %v", got)
	}
	if got := model.historyFilterInput.TextStyle.GetForeground(); got != lipgloss.Color("#0f172a") {
		t.Fatalf("expected light history filter text foreground, got %v", got)
	}
	if got := model.historyFilterInput.PromptStyle.GetForeground(); got != lipgloss.Color("#64748b") {
		t.Fatalf("expected light history filter prompt foreground, got %v", got)
	}
	if got := model.historyFilterInput.PlaceholderStyle.GetForeground(); got != lipgloss.Color("#64748b") {
		t.Fatalf("expected history placeholder to use subtle light color, got %v", got)
	}
}

func TestDarkModalFallback(t *testing.T) {
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
		t.Fatalf("expected default dark modal backdrop, got %v", got)
	}
	if got := rt.modalInputBackground(customDark); got != lipgloss.Color("#1c1a23") {
		t.Fatalf("expected default dark modal input background, got %v", got)
	}
}

func TestModalOverrides(t *testing.T) {
	customDark := theme.DefaultTheme()
	customDark.CommandBar = customDark.CommandBar.Background(lipgloss.Color("#102938"))
	customDark.ResponseSelection = customDark.ResponseSelection.Background(lipgloss.Color("#223344"))
	customDark.ModalBackdrop = lipgloss.Color("#0f1720")
	customDark.ModalInputBackground = lipgloss.Color("#162033")
	customDark.ModalOption = lipgloss.Color("#94a3b8")

	rt := newThemeRuntime(theme.Definition{
		Key: "aurora",
		Metadata: theme.Metadata{
			Name: "Aurora",
			Tags: []string{"dark"},
		},
		Theme: customDark,
	})

	if got := rt.modalBackdropColor(customDark); got != lipgloss.Color("#0f1720") {
		t.Fatalf("expected explicit modal backdrop override, got %v", got)
	}
	if got := rt.modalInputBackground(customDark); got != lipgloss.Color("#162033") {
		t.Fatalf("expected explicit modal input background override, got %v", got)
	}
	option := rt.modalOptionStyle(customDark)
	if got := option.GetForeground(); got != lipgloss.Color("#94a3b8") {
		t.Fatalf("expected explicit modal option foreground, got %v", got)
	}
}

func TestEditorSelectionIgnoresModalInput(t *testing.T) {
	model := New(Config{})

	lightTheme := theme.DefaultTheme()
	lightTheme.CommandBar = lightTheme.CommandBar.Background(lipgloss.Color("#dbe4ee"))
	lightTheme.ResponseSelection = lipgloss.NewStyle().Background(lipgloss.Color("#e2e8f0"))
	lightTheme.ModalInputBackground = lipgloss.Color("#cbd5f5")
	lightTheme.PaneActiveForeground = lipgloss.Color("#0f172a")

	model.applyThemeDefinition(theme.Definition{
		Key: "daybreak",
		Metadata: theme.Metadata{
			Name: "Daybreak",
			Tags: []string{"light"},
		},
		Theme: lightTheme,
	})

	if got := model.themeRuntime.modalInputBackground(model.theme); got != lipgloss.Color("#cbd5f5") {
		t.Fatalf("expected modal input background override, got %v", got)
	}
	if got := model.editor.SelectionStyle().GetBackground(); got != lipgloss.Color("#e2e8f0") {
		t.Fatalf("expected editor selection to keep response selection background, got %v", got)
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
	expected := expectedRenderer.buildHTTPResponseViews(resp, nil, nil)
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

func TestSyncThemedResponseStateRestartsPendingRenderWithFreshToken(t *testing.T) {
	model := New(Config{})
	resp := &httpclient.Response{
		Status:       "200 OK",
		StatusCode:   200,
		Headers:      http.Header{"Content-Type": {"application/json"}},
		Body:         []byte(`{"id":1}`),
		EffectiveURL: "https://api.example.com/items",
	}

	initialCmd := model.consumeHTTPResponse(resp, nil, nil, "", nil)
	if initialCmd == nil {
		t.Fatalf("expected pending response render command")
	}
	if model.responsePending == nil {
		t.Fatalf("expected pending snapshot")
	}

	oldRenderToken := model.responseRenderToken
	oldSnapshotID := model.responsePending.id
	if oldRenderToken == "" || oldSnapshotID == "" {
		t.Fatalf("expected initial render token and snapshot id")
	}

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

	restartCmd := model.syncThemedResponseState()
	if restartCmd == nil {
		t.Fatalf("expected themed response sync command")
	}
	if model.responseRenderToken == "" {
		t.Fatalf("expected restarted render token")
	}
	if model.responseRenderToken == oldRenderToken {
		t.Fatalf("expected pending restart to use a fresh render token")
	}
	if model.responsePending == nil || model.responsePending.id != oldSnapshotID {
		t.Fatalf("expected pending snapshot identity to stay stable")
	}

	stale := responseRenderedMsg{token: oldRenderToken, pretty: "stale"}
	if cmd := model.handleResponseRendered(stale); cmd != nil {
		t.Fatalf("expected stale render to be ignored")
	}
	if model.responseRenderToken == "" || model.responseRenderToken == oldRenderToken {
		t.Fatalf("expected stale render to leave restarted token active")
	}

	drainResponseCommands(t, &model, restartCmd)

	if model.responseLatest == nil || !model.responseLatest.ready {
		t.Fatalf("expected restarted render to complete")
	}
	if model.responseLatest.id != oldSnapshotID {
		t.Fatalf("expected restart to preserve snapshot identity")
	}
}

func TestApplyThemeDefinitionKeepsGRPCRawContentType(t *testing.T) {
	model := New(Config{})
	resp := &grpcclient.Response{
		Message:         `{"id":1}`,
		Body:            []byte(`{"id":1}`),
		ContentType:     "application/json",
		Wire:            []byte{0x00, 0x01, 0x02},
		WireContentType: "application/protobuf",
		Headers: map[string][]string{
			"content-type": {"application/grpc"},
		},
		StatusCode: codes.OK,
	}
	req := &restfile.Request{
		GRPC: &restfile.GRPCRequest{FullMethod: "/demo.Service/Call"},
	}

	if cmd := model.consumeGRPCResponse(resp, nil, nil, req, "", nil); cmd != nil {
		_ = collectMsgs(cmd)
	}
	if model.responseLatest == nil {
		t.Fatalf("expected gRPC snapshot")
	}
	if got := model.responseLatest.contentType; got != resp.WireContentType {
		t.Fatalf("expected gRPC snapshot content type to track raw wire body, got %q", got)
	}

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

	if got := model.responseLatest.contentType; got != resp.WireContentType {
		t.Fatalf("expected themed rerender to preserve raw gRPC content type, got %q", got)
	}
}

func TestResolveThemeDefinitionFallsBackToDefaultDefinition(t *testing.T) {
	def := theme.ResolveDefinition(theme.Catalog{}, "", theme.DefaultTheme())
	if def.Key != "default" {
		t.Fatalf("expected default fallback key, got %q", def.Key)
	}
	if def.Appearance() != theme.AppearanceDark {
		t.Fatalf("expected default fallback appearance to be dark, got %v", def.Appearance())
	}
}

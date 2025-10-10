package ui

import (
	"net/http"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
)

func TestRedactHistoryTextMasksSecrets(t *testing.T) {
	mask := maskSecret("", true)
	secrets := []string{"oauth-token", "oauth-refresh"}
	text := "access=oauth-token&refresh=oauth-refresh"
	redacted := redactHistoryText(text, secrets, false)
	expected := "access=" + mask + "&refresh=" + mask
	if redacted != expected {
		t.Fatalf("expected %q, got %q", expected, redacted)
	}
}

func TestRedactHistoryTextSkipsWhenNoSecrets(t *testing.T) {
	text := "plain text"
	if got := redactHistoryText(text, nil, true); got != text {
		t.Fatalf("expected unchanged text when no secrets, got %q", got)
	}

	if got := redactHistoryText("", []string{"secret"}, true); got != "" {
		t.Fatalf("expected empty text to remain empty, got %q", got)
	}
}

func TestRedactHistoryTextMasksSensitiveHeaders(t *testing.T) {
	mask := maskSecret("", true)
	input := "Authorization: Bearer 123\nX-API-Key: abc"
	got := redactHistoryText(input, nil, true)
	if !strings.Contains(got, "Authorization: "+mask) {
		t.Fatalf("expected authorization header to be masked, got %q", got)
	}
	if !strings.Contains(got, "X-API-Key: "+mask) {
		t.Fatalf("expected api key header to be masked, got %q", got)
	}
}

func TestRedactHistoryTextHonorsSensitiveHeaderOverride(t *testing.T) {
	input := "Authorization: Bearer 123"
	got := redactHistoryText(input, nil, false)
	if got != input {
		t.Fatalf("expected header to remain when masking disabled, got %q", got)
	}
}

func TestFormatHistorySnippetStripsHTMLAndLimitsLines(t *testing.T) {
	snippet := "<html><head><style>body{color:red}</style></head><body><h1>Hello</h1><p>World</p><p>More content here.</p><p>Line4</p><p>Line5</p><p>Line6</p><p>Line7</p><p>Line8</p><p>Line9</p><p>Line10</p><p>Line11</p><p>Line12</p><p>Line13</p><p>Line14</p><p>Line15</p><p>Line16</p><p>Line17</p><p>Line18</p><p>Line19</p><p>Line20</p><p>Line21</p><p>Line22</p><p>Line23</p><p>Line24</p><p>Line25</p></body></html>"

	formatted := formatHistorySnippet(snippet, 40)

	if strings.Contains(formatted, "body{") {
		t.Fatalf("expected style content to be removed, got %q", formatted)
	}
	if strings.Contains(formatted, "<") || strings.Contains(formatted, ">") {
		t.Fatalf("expected HTML tags to be stripped, got %q", formatted)
	}

	lines := strings.Split(formatted, "\n")
	if len(lines) != historySnippetMaxLines+1 {
		t.Fatalf("expected %d lines plus truncation, got %d", historySnippetMaxLines+1, len(lines))
	}
	if !strings.HasSuffix(lines[len(lines)-1], "(truncated)") {
		t.Fatalf("expected truncation marker, got %q", lines[len(lines)-1])
	}
}

func TestFormatHistorySnippetHandlesStyleOnly(t *testing.T) {
	snippet := "<style>body{color:red}</style>"
	formatted := formatHistorySnippet(snippet, 40)
	if formatted != historySnippetPlaceholder {
		t.Fatalf("expected placeholder for empty html snippet, got %q", formatted)
	}
}

func TestConsumeHTTPResponseSchedulesAsyncRender(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.width = 120
	model.height = 40
	if cmd := model.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}

	resp := &httpclient.Response{
		Status:       "200 OK",
		StatusCode:   200,
		Headers:      http.Header{"Content-Type": []string{"text/html"}},
		Body:         []byte("<html><body><p>Hello</p></body></html>"),
		Duration:     150 * time.Millisecond,
		EffectiveURL: "https://example.com",
	}

	cmd := model.consumeHTTPResponse(resp, nil, nil)
	if cmd == nil {
		t.Fatalf("expected consumeHTTPResponse to return render command")
	}
	if !model.responseLoading {
		t.Fatalf("expected responseLoading to be true after scheduling render")
	}
	if model.responseRenderToken == "" {
		t.Fatalf("expected responseRenderToken to be assigned")
	}
	if content := model.pane(responsePanePrimary).viewport.View(); !strings.HasPrefix(content, responseFormattingBase) {
		t.Fatalf("expected viewport to show formatting message prefix, got %q", content)
	}

	drainResponseCommands(t, &model, cmd)

	if model.responseLoading {
		t.Fatalf("expected responseLoading to be false after render completes")
	}
	if model.responseLatest == nil || model.responseLatest.pretty == "" {
		t.Fatalf("expected latest snapshot to be populated")
	}
	viewportContent := model.pane(responsePanePrimary).viewport.View()
	if !strings.Contains(viewportContent, "Status:") {
		t.Fatalf("expected viewport content to include response summary, got %q", viewportContent)
	}
}

func collectMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		msgs := make([]tea.Msg, len(batch))
		for i, item := range batch {
			msgs[i] = item
		}
		return msgs
	}
	return []tea.Msg{msg}
}

func drainResponseCommands(t *testing.T, model *Model, initial tea.Cmd) {
	queue := collectMsgs(initial)
	for len(queue) > 0 {
		msg := queue[0]
		queue = queue[1:]
		switch typed := msg.(type) {
		case responseRenderedMsg:
			if typed.token != model.responseRenderToken {
				t.Fatalf("render token mismatch: %s vs %s", typed.token, model.responseRenderToken)
			}
			if follow := model.handleResponseRendered(typed); follow != nil {
				queue = append(queue, collectMsgs(follow)...)
			}
		case tea.Cmd:
			queue = append(queue, collectMsgs(typed)...)
		case statusMsg:
			// ignore status updates
		case responseLoadingTickMsg:
			if follow := model.handleResponseLoadingTick(); follow != nil {
				queue = append(queue, collectMsgs(follow)...)
			}
		case nil:
			// ignore
		default:
			t.Fatalf("unexpected message type %T", typed)
		}
	}
}

func TestToggleResponseSplitConfiguresSecondaryPane(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.width = 120
	model.height = 40
	if cmd := model.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}

	resp := &httpclient.Response{
		Status:       "200 OK",
		StatusCode:   200,
		Headers:      http.Header{"Content-Type": []string{"text/plain"}},
		Body:         []byte("alpha"),
		EffectiveURL: "https://example.com",
	}

	drainResponseCommands(t, &model, model.consumeHTTPResponse(resp, nil, nil))
	if model.responseLatest == nil || !model.responseLatest.ready {
		t.Fatalf("expected latest snapshot to be ready")
	}

	if model.responseSplit {
		t.Fatalf("expected split to be disabled initially")
	}

	if cmd := model.toggleResponseSplitVertical(); cmd != nil {
		collectMsgs(cmd)
	}
	if !model.responseSplit {
		t.Fatalf("expected split to be enabled")
	}
	secondary := model.pane(responsePaneSecondary)
	if secondary == nil {
		t.Fatalf("expected secondary pane to exist")
	}
	if secondary.followLatest {
		t.Fatalf("expected secondary pane to be pinned by default")
	}
	if secondary.activeTab != responseTabPretty {
		t.Fatalf("expected secondary pane default tab to be Pretty, got %v", secondary.activeTab)
	}
	if secondary.snapshot != model.responseLatest {
		t.Fatalf("expected secondary pane to reference latest snapshot")
	}
}

func TestDiffTabAvailableAfterDualResponses(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.width = 120
	model.height = 40
	if cmd := model.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}

	first := &httpclient.Response{
		Status:       "200 OK",
		StatusCode:   200,
		Headers:      http.Header{"Content-Type": []string{"text/plain"}},
		Body:         []byte("first"),
		EffectiveURL: "https://example.com/one",
	}
	drainResponseCommands(t, &model, model.consumeHTTPResponse(first, nil, nil))
	if model.diffAvailable() {
		t.Fatalf("diff should be unavailable before split")
	}

	if cmd := model.toggleResponseSplitVertical(); cmd != nil {
		collectMsgs(cmd)
	}
	if !model.responseSplit {
		t.Fatalf("expected split enabled")
	}

	second := &httpclient.Response{
		Status:       "200 OK",
		StatusCode:   200,
		Headers:      http.Header{"Content-Type": []string{"text/plain"}},
		Body:         []byte("second"),
		EffectiveURL: "https://example.com/two",
	}
	drainResponseCommands(t, &model, model.consumeHTTPResponse(second, nil, nil))
	if !model.diffAvailable() {
		t.Fatalf("expected diff to be available after second response")
	}

	primary := model.pane(responsePanePrimary)
	primary.setActiveTab(responseTabDiff)
	primary.lastContentTab = responseTabRaw
	if cmd := model.syncResponsePane(responsePanePrimary); cmd != nil {
		collectMsgs(cmd)
	}
	diffView := primary.viewport.View()
	if !strings.Contains(diffView, "+") && !strings.Contains(diffView, "Responses are identical") {
		t.Fatalf("expected diff view to contain diff markers, got %q", diffView)
	}

	tabs := model.availableResponseTabs()
	if indexOfResponseTab(tabs, responseTabDiff) == -1 {
		t.Fatalf("expected diff tab to be present")
	}
}

func TestResponsesFollowLastFocusedPane(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.width = 120
	model.height = 40
	if cmd := model.applyLayout(); cmd != nil {
		drainResponseCommands(t, &model, cmd)
	}

	resp1 := &httpclient.Response{
		Status:       "200 OK",
		StatusCode:   200,
		Headers:      http.Header{"Content-Type": []string{"text/plain"}},
		Body:         []byte("first"),
		EffectiveURL: "https://example.com/one",
	}
	drainResponseCommands(t, &model, model.consumeHTTPResponse(resp1, nil, nil))

	primary := model.pane(responsePanePrimary)
	if primary == nil || primary.snapshot == nil || !strings.Contains(primary.snapshot.pretty, "first") {
		t.Fatalf("expected primary pane to hold first response")
	}

	if cmd := model.toggleResponseSplitVertical(); cmd != nil {
		drainResponseCommands(t, &model, cmd)
	}
	model.setFocus(focusResponse)
	model.focusResponsePane(responsePaneSecondary)
	model.setFocus(focusRequests)

	resp2 := &httpclient.Response{
		Status:       "201 Created",
		StatusCode:   201,
		Headers:      http.Header{"Content-Type": []string{"text/plain"}},
		Body:         []byte("second"),
		EffectiveURL: "https://example.com/two",
	}
	drainResponseCommands(t, &model, model.consumeHTTPResponse(resp2, nil, nil))

	secondary := model.pane(responsePaneSecondary)
	if secondary == nil || secondary.snapshot == nil || !strings.Contains(secondary.snapshot.pretty, "second") {
		t.Fatalf("expected secondary pane to receive latest response")
	}
	if primary.snapshot == nil || !strings.Contains(primary.snapshot.pretty, "first") {
		t.Fatalf("expected primary pane to retain previous response")
	}
	if !secondary.followLatest || primary.followLatest {
		t.Fatalf("expected secondary to be live and primary pinned")
	}
}

func TestTogglePaneFollowLatestPinsSnapshot(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.width = 120
	model.height = 40
	if cmd := model.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}

	resp1 := &httpclient.Response{
		Status:       "200 OK",
		StatusCode:   200,
		Headers:      http.Header{"Content-Type": []string{"text/plain"}},
		Body:         []byte("first"),
		EffectiveURL: "https://example.com/a",
	}
	drainResponseCommands(t, &model, model.consumeHTTPResponse(resp1, nil, nil))
	primary := model.pane(responsePanePrimary)
	firstSnapshot := primary.snapshot
	if firstSnapshot == nil {
		t.Fatalf("expected primary snapshot to be set")
	}
	if cmd := model.toggleResponseSplitVertical(); cmd != nil {
		collectMsgs(cmd)
	}
	model.setFocus(focusResponse)

	if cmd := model.togglePaneFollowLatest(responsePanePrimary); cmd != nil {
		collectMsgs(cmd)
	}
	if primary.followLatest {
		t.Fatalf("expected primary pane to be pinned after toggle")
	}
	secondary := model.pane(responsePaneSecondary)
	if secondary == nil || !secondary.followLatest {
		t.Fatalf("expected secondary pane to become live after pinning primary")
	}

	resp2 := &httpclient.Response{
		Status:       "200 OK",
		StatusCode:   200,
		Headers:      http.Header{"Content-Type": []string{"text/plain"}},
		Body:         []byte("second"),
		EffectiveURL: "https://example.com/b",
	}
	drainResponseCommands(t, &model, model.consumeHTTPResponse(resp2, nil, nil))
	if primary.snapshot != firstSnapshot {
		t.Fatalf("expected pinned pane to retain original snapshot")
	}
	if secondary.snapshot == nil || !strings.Contains(secondary.snapshot.pretty, "second") {
		t.Fatalf("expected live pane to receive new response")
	}
}

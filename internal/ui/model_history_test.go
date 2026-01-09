package ui

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/restfile"
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

func TestRecordCompareHistoryAppendsEntry(t *testing.T) {
	tmp := t.TempDir()
	store := history.NewStore(filepath.Join(tmp, "history.json"), 10)
	model := New(Config{History: store})

	req := &restfile.Request{
		Method:   "GET",
		URL:      "https://example.com/data",
		Metadata: restfile.RequestMetadata{Name: "Sample"},
	}

	state := &compareState{
		base:  cloneRequest(req),
		spec:  &restfile.CompareSpec{Baseline: "dev"},
		envs:  []string{"dev"},
		index: 1,
		label: "Compare sample",
		results: []compareResult{
			{
				Environment: "dev",
				Response: &httpclient.Response{
					Status:     "200 OK",
					StatusCode: 200,
					Body:       []byte(`{"ok":true}`),
					Duration:   15 * time.Millisecond,
				},
				Request:     cloneRequest(req),
				RequestText: "GET https://example.com/data\n",
			},
		},
	}

	model.recordCompareHistory(state)

	entries := store.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	entry := entries[0]
	if entry.Method != restfile.HistoryMethodCompare {
		t.Fatalf("expected compare method, got %s", entry.Method)
	}
	if entry.Compare == nil || len(entry.Compare.Results) != 1 {
		t.Fatalf("expected compare metadata, got %#v", entry.Compare)
	}
	if entry.Compare.Results[0].Environment != "dev" {
		t.Fatalf("expected result env dev, got %s", entry.Compare.Results[0].Environment)
	}
}

func TestLoadHistorySelectionComparePrefersFailure(t *testing.T) {
	model := New(Config{})
	entry := history.Entry{
		ID:          "cmp-1",
		Method:      restfile.HistoryMethodCompare,
		URL:         "https://api.example.com/users",
		RequestName: "CompareUsers",
		ExecutedAt:  time.Now(),
		RequestText: "GET https://api.example.com/users\n",
		Compare: &history.CompareEntry{
			Baseline: "dev",
			Results: []history.CompareResult{
				{
					Environment: "dev",
					Status:      "200 OK",
					StatusCode:  200,
					RequestText: "GET https://api.example.com/users\n",
				},
				{
					Environment: "stage",
					Status:      "500 Internal Server Error",
					StatusCode:  500,
					Error:       "internal error",
					RequestText: "GET https://api.example.com/users\nX-Debug: fail\n",
				},
			},
		},
	}

	model.historyEntries = []history.Entry{entry}
	model.historyList.SetItems(makeHistoryItems(model.historyEntries, model.historyScope))
	model.historyList.Select(0)

	cmd := model.loadHistorySelection(false)
	collectMsgs(cmd)

	if env := model.cfg.EnvironmentName; env != "stage" {
		t.Fatalf("expected environment to switch to failing env stage, got %s", env)
	}
	if !strings.Contains(model.editor.Value(), "X-Debug") {
		t.Fatalf("expected stage request text to load, got %q", model.editor.Value())
	}
	if model.compareBundle == nil {
		t.Fatalf("expected compare bundle to populate when loading compare history")
	}
}

func TestLoadHistorySelectionCompareHydratesSnapshots(t *testing.T) {
	model := New(Config{})
	entry := history.Entry{
		ID:          "cmp-2",
		Method:      restfile.HistoryMethodCompare,
		URL:         "https://api.example.com/items",
		RequestName: "CompareItems",
		ExecutedAt:  time.Now(),
		RequestText: "GET https://api.example.com/items\n",
		Compare: &history.CompareEntry{
			Baseline: "dev",
			Results: []history.CompareResult{
				{
					Environment: "dev",
					Status:      "200 OK",
					StatusCode:  200,
					BodySnippet: "{\n  \"value\": \"dev\"\n}",
				},
				{Environment: "stage", Status: "500", StatusCode: 500, Error: "boom"},
			},
		},
	}

	model.historyEntries = []history.Entry{entry}
	model.historyList.SetItems(makeHistoryItems(model.historyEntries, model.historyScope))
	model.historyList.Select(0)

	cmd := model.loadHistorySelection(false)
	collectMsgs(cmd)

	if model.compareBundle == nil {
		t.Fatalf("expected compare bundle to be present")
	}
	if model.compareSelectedEnv != "stage" {
		t.Fatalf("expected selected env to track failure, got %q", model.compareSelectedEnv)
	}
	if model.compareFocusedEnv != "stage" {
		t.Fatalf("expected focused env to be stage, got %q", model.compareFocusedEnv)
	}
	if model.compareRowIndex != 1 {
		t.Fatalf("expected compareRowIndex to point to stage row, got %d", model.compareRowIndex)
	}
	snap := model.compareSnapshot("stage")
	if snap == nil {
		t.Fatalf("expected snapshot stored for stage env")
	}
	if snap.compareBundle == nil {
		t.Fatalf("expected snapshot to reference compare bundle")
	}
	if !strings.Contains(snap.pretty, "Error:") {
		t.Fatalf("expected snapshot summary to include error text, got %q", snap.pretty)
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

	cmd := model.consumeHTTPResponse(resp, nil, nil, "")
	if cmd == nil {
		t.Fatalf("expected consumeHTTPResponse to return render command")
	}
	if !model.responseLoading {
		t.Fatalf("expected responseLoading to be true after scheduling render")
	}
	if model.responseRenderToken == "" {
		t.Fatalf("expected responseRenderToken to be assigned")
	}
	if content := model.pane(
		responsePanePrimary,
	).viewport.View(); !strings.HasPrefix(
		content,
		responseFormattingBase,
	) {
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

	drainResponseCommands(t, &model, model.consumeHTTPResponse(resp, nil, nil, ""))
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

func TestPresentHistoryEntryPopulatesTimeline(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.width = 100
	model.height = 40
	if cmd := model.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}

	tl := &nettrace.Timeline{
		Duration: 110 * time.Millisecond,
		Phases: []nettrace.Phase{
			{Kind: nettrace.PhaseDNS, Duration: 30 * time.Millisecond},
			{Kind: nettrace.PhaseConnect, Duration: 40 * time.Millisecond},
			{Kind: nettrace.PhaseTransfer, Duration: 40 * time.Millisecond},
		},
	}
	budget := nettrace.Budget{
		Total: 120 * time.Millisecond,
		Phases: map[nettrace.PhaseKind]time.Duration{
			nettrace.PhaseDNS: 50 * time.Millisecond,
		},
	}
	report := nettrace.NewReport(tl, budget)
	summary := history.NewTraceSummary(tl, report)
	entry := history.Entry{
		Trace:      summary,
		Status:     "200 OK",
		StatusCode: 200,
		Duration:   tl.Duration,
		Method:     "GET",
		URL:        "https://example.com",
	}

	cmd := model.presentHistoryEntry(entry, nil)
	if cmd != nil {
		collectMsgs(cmd)
	}

	if model.responseLatest == nil || model.responseLatest.timeline == nil {
		t.Fatalf("expected history timeline to populate snapshot")
	}
	if !model.snapshotHasTimeline() {
		t.Fatalf("expected timeline tab to become available")
	}
	pane := model.pane(responsePanePrimary)
	if pane == nil || pane.snapshot != model.responseLatest {
		t.Fatalf("expected primary pane to reference latest snapshot")
	}
	content, _ := model.paneContentForTab(responsePanePrimary, responseTabTimeline)
	if !strings.Contains(content, "Timeline") {
		t.Fatalf("expected timeline content to render, got %q", content)
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
	drainResponseCommands(t, &model, model.consumeHTTPResponse(first, nil, nil, ""))
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
	drainResponseCommands(t, &model, model.consumeHTTPResponse(second, nil, nil, ""))
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
	drainResponseCommands(t, &model, model.consumeHTTPResponse(resp1, nil, nil, ""))

	primary := model.pane(responsePanePrimary)
	if primary == nil || primary.snapshot == nil ||
		!strings.Contains(primary.snapshot.pretty, "first") {
		t.Fatalf("expected primary pane to hold first response")
	}

	if cmd := model.toggleResponseSplitVertical(); cmd != nil {
		drainResponseCommands(t, &model, cmd)
	}
	_ = model.setFocus(focusResponse)
	model.focusResponsePane(responsePaneSecondary)
	_ = model.setFocus(focusRequests)

	resp2 := &httpclient.Response{
		Status:       "201 Created",
		StatusCode:   201,
		Headers:      http.Header{"Content-Type": []string{"text/plain"}},
		Body:         []byte("second"),
		EffectiveURL: "https://example.com/two",
	}
	drainResponseCommands(t, &model, model.consumeHTTPResponse(resp2, nil, nil, ""))

	secondary := model.pane(responsePaneSecondary)
	if secondary == nil || secondary.snapshot == nil ||
		!strings.Contains(secondary.snapshot.pretty, "second") {
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
	drainResponseCommands(t, &model, model.consumeHTTPResponse(resp1, nil, nil, ""))
	primary := model.pane(responsePanePrimary)
	firstSnapshot := primary.snapshot
	if firstSnapshot == nil {
		t.Fatalf("expected primary snapshot to be set")
	}
	if cmd := model.toggleResponseSplitVertical(); cmd != nil {
		collectMsgs(cmd)
	}
	_ = model.setFocus(focusResponse)

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
	drainResponseCommands(t, &model, model.consumeHTTPResponse(resp2, nil, nil, ""))
	if primary.snapshot != firstSnapshot {
		t.Fatalf("expected pinned pane to retain original snapshot")
	}
	if secondary.snapshot == nil || !strings.Contains(secondary.snapshot.pretty, "second") {
		t.Fatalf("expected live pane to receive new response")
	}
}

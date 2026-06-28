package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	xplain "github.com/unkn0wn-root/resterm/internal/explain"
)

func TestRetreatResponseSearchWrap(t *testing.T) {
	model := New(Config{})
	model.responsePaneFocus = responsePanePrimary
	model.searchResponsePane = responsePanePrimary
	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatal("expected response pane to be available")
	}
	pane.viewport.Width = 80
	pane.snapshot = &responseSnapshot{
		id:      "snap-1",
		pretty:  withTrailingNewline("foo bar\nfoo baz\nfoo"),
		raw:     withTrailingNewline("foo bar\nfoo baz\nfoo"),
		headers: withTrailingNewline("Status: 200 OK"),
		ready:   true,
	}

	if status := statusFromCmd(t, model.applyResponseSearch("foo", false)); status == nil {
		t.Fatal("expected initial response search status")
	}

	cmd := model.retreatResponseSearch()
	status := statusFromCmd(t, cmd)
	if status == nil {
		t.Fatal("expected status after retreating search")
	}
	if status.level != statusInfo {
		t.Fatalf("expected info level, got %v", status.level)
	}
	if !strings.Contains(status.text, "Match 3/3") {
		t.Fatalf("expected wrap to last match, got %q", status.text)
	}
	if !strings.Contains(status.text, "(wrapped)") {
		t.Fatalf("expected wrapped indicator, got %q", status.text)
	}
	if pane.search.index != 2 {
		t.Fatalf("expected search index 2, got %d", pane.search.index)
	}

	cmd = model.retreatResponseSearch()
	status = statusFromCmd(t, cmd)
	if status == nil {
		t.Fatal("expected status after moving to previous match")
	}
	if !strings.Contains(status.text, "Match 2/3") {
		t.Fatalf("expected to move to second match, got %q", status.text)
	}
	if strings.Contains(status.text, "(wrapped)") {
		t.Fatalf("did not expect wrapped indicator on second match, got %q", status.text)
	}
	if pane.search.index != 1 {
		t.Fatalf("expected search index 1, got %d", pane.search.index)
	}
}

func TestClearResponseSearchOnEsc(t *testing.T) {
	model := New(Config{})
	model.focus = focusResponse
	model.responsePaneFocus = responsePanePrimary
	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatal("expected response pane to be available")
	}
	pane.viewport.Width = 80
	pane.snapshot = &responseSnapshot{
		id:      "snap-1",
		pretty:  withTrailingNewline("foo bar\nfoo baz\nfoo"),
		raw:     withTrailingNewline("foo bar\nfoo baz\nfoo"),
		headers: withTrailingNewline("Status: 200 OK"),
		ready:   true,
	}

	if status := statusFromCmd(t, model.applyResponseSearch("foo", false)); status == nil {
		t.Fatal("expected initial response search status")
	}
	if !pane.search.active || len(pane.search.matches) == 0 {
		t.Fatalf("expected search to be active with matches")
	}

	cmd := model.handleKeyWithChord(tea.KeyMsg{Type: tea.KeyEscape}, false)
	status := statusFromCmd(t, cmd)
	if status == nil {
		t.Fatal("expected status after clearing search")
	}
	if status.text != "Search cleared" {
		t.Fatalf("expected \"Search cleared\" status, got %q", status.text)
	}
	if pane.search.hasQuery() || pane.search.active || len(pane.search.matches) != 0 {
		t.Fatalf(
			"expected search state cleared, got query=%q active=%v matches=%d",
			pane.search.query,
			pane.search.active,
			len(pane.search.matches),
		)
	}
}

func TestSlashOpensEditorSearchPromptWithoutTypingSlash(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.focus = focusEditor
	model.editorInsertMode = false

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = updated.(Model)

	if !model.showSearchPrompt {
		t.Fatal("expected search prompt to open")
	}
	if model.searchTarget != searchTargetEditor {
		t.Fatalf("expected editor search target, got %v", model.searchTarget)
	}
	if got := model.searchInput.Value(); got != "" {
		t.Fatalf("expected trigger slash to be consumed, got search input %q", got)
	}
}

func TestSlashOpensResponseSearchPromptWithoutTypingSlash(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.focus = focusResponse
	model.responsePaneFocus = responsePanePrimary
	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatal("expected response pane to be available")
	}
	pane.activeTab = responseTabPretty

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = updated.(Model)

	if !model.showSearchPrompt {
		t.Fatal("expected search prompt to open")
	}
	if model.searchTarget != searchTargetResponse {
		t.Fatalf("expected response search target, got %v", model.searchTarget)
	}
	if got := model.searchInput.Value(); got != "" {
		t.Fatalf("expected trigger slash to be consumed, got search input %q", got)
	}
}

func TestSlashStillOpensHistoryFilter(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.focus = focusResponse
	model.responsePaneFocus = responsePanePrimary
	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatal("expected response pane to be available")
	}
	pane.activeTab = responseTabHistory

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = updated.(Model)

	if model.showSearchPrompt {
		t.Fatal("did not expect response search prompt on history slash")
	}
	if !model.historyFilterActive {
		t.Fatal("expected history filter to open")
	}
}

func TestShiftFStillOpensResponseSearchOnHistoryTab(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.focus = focusResponse
	model.responsePaneFocus = responsePanePrimary
	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatal("expected response pane to be available")
	}
	pane.activeTab = responseTabHistory

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	model = updated.(Model)

	if !model.showSearchPrompt {
		t.Fatal("expected response search prompt to open")
	}
	if model.searchTarget != searchTargetResponse {
		t.Fatalf("expected response search target, got %v", model.searchTarget)
	}
	if model.historyFilterActive {
		t.Fatal("did not expect history filter to open for Shift+F")
	}
	if got := model.searchInput.Value(); got != "" {
		t.Fatalf("expected trigger key to be consumed, got search input %q", got)
	}
}

func TestQuestionMarkStillOpensHelp(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.focus = focusResponse
	model.responsePaneFocus = responsePanePrimary

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model = updated.(Model)

	if !model.showHelp {
		t.Fatal("expected question mark to open help")
	}
	if model.showSearchPrompt {
		t.Fatal("did not expect question mark to open search while help binding is active")
	}
}

func TestEditorSearchAppliesWhileTyping(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.focus = focusEditor
	model.editorInsertMode = false
	model.editor.SetValue("one\ntwo\nthree two")
	editorPtr := &model.editor
	editorPtr.moveCursorTo(0, 0)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = updated.(Model)
	for _, r := range "two" {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = updated.(Model)
	}

	if !model.showSearchPrompt {
		t.Fatal("expected search prompt to stay open while typing")
	}
	if got := model.searchInput.Value(); got != "two" {
		t.Fatalf("expected prompt value %q, got %q", "two", got)
	}
	if !model.editor.SearchActive() {
		t.Fatal("expected editor search to apply before enter")
	}
	if model.editor.search.query != "two" {
		t.Fatalf("expected editor search query to be stored, got %q", model.editor.search.query)
	}
	if model.editor.search.index != 0 {
		t.Fatalf("expected first match selected, got %d", model.editor.search.index)
	}
	if ranges := model.editor.HighlightRanges(); len(ranges) != 2 || !ranges[0].Active {
		t.Fatalf("expected both editor matches marked with first active, got %+v", ranges)
	}
	pos := model.editor.caretPosition()
	if pos.Line != 1 || pos.Column != 0 {
		t.Fatalf("expected caret at first live match, got line %d column %d", pos.Line, pos.Column)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	if model.showSearchPrompt {
		t.Fatal("expected esc to close prompt")
	}
	if !model.editor.SearchActive() {
		t.Fatal("expected esc to keep live search active")
	}
}

func TestResponseSearchAppliesWhileTyping(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.focus = focusResponse
	model.responsePaneFocus = responsePanePrimary
	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatal("expected response pane to be available")
	}
	pane.viewport.Width = 80
	pane.snapshot = &responseSnapshot{
		id:      "snap-1",
		pretty:  withTrailingNewline("foo bar\nfoo baz\nfoo"),
		raw:     withTrailingNewline("foo bar\nfoo baz\nfoo"),
		headers: withTrailingNewline("Status: 200 OK"),
		ready:   true,
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = updated.(Model)
	for _, r := range "foo" {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = updated.(Model)
	}

	pane = model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatal("expected response pane to be available after update")
	}
	if !model.showSearchPrompt {
		t.Fatal("expected search prompt to stay open while typing")
	}
	if !pane.search.active {
		t.Fatal("expected response search to apply before enter")
	}
	if pane.search.query != "foo" {
		t.Fatalf("expected response search query to be stored, got %q", pane.search.query)
	}
	if len(pane.search.matches) != 3 {
		t.Fatalf("expected 3 response matches, got %d", len(pane.search.matches))
	}
	if pane.search.index != 0 {
		t.Fatalf("expected first response match selected, got %d", pane.search.index)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	pane = model.pane(responsePanePrimary)
	if model.showSearchPrompt {
		t.Fatal("expected esc to close prompt")
	}
	if pane == nil || !pane.search.active || pane.search.query != "foo" {
		t.Fatalf("expected esc to keep live response search active, got %+v", pane)
	}
}

func TestResponseSearchIgnoresANSIParametersInPretty(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.focus = focusResponse
	model.responsePaneFocus = responsePanePrimary

	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatal("expected response pane to be available")
	}
	pane.activeTab = responseTabPretty
	pane.viewport.Width = 80
	pane.viewport.Height = 8
	colored := "\x1b[38;5;244m\"X-Amzn-Trace-Id\"\x1b[0m: " +
		"\x1b[48;2;248;242;240m\"Root=abc\"\x1b[0m"
	pane.snapshot = &responseSnapshot{
		id:      "snap-colored-no-visible-4",
		pretty:  withTrailingNewline(colored),
		raw:     withTrailingNewline(stripANSIEscape(colored)),
		headers: withTrailingNewline("Status: 200 OK"),
		ready:   true,
	}

	status := statusFromCmd(t, model.applyResponseSearch("4", false))
	if status == nil {
		t.Fatal("expected search status")
	}
	if status.level != statusWarn || !strings.Contains(status.text, "No matches") {
		t.Fatalf("expected no visible matches, got %+v", status)
	}
	if len(pane.search.matches) != 0 || pane.search.active {
		t.Fatalf(
			"expected ANSI-only digits to be ignored, active=%v matches=%d",
			pane.search.active,
			len(pane.search.matches),
		)
	}
}

func TestResponseSearchHighlightPreservesANSIColoredPretty(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.focus = focusResponse
	model.responsePaneFocus = responsePanePrimary

	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatal("expected response pane to be available")
	}
	pane.activeTab = responseTabPretty
	pane.viewport.Width = 80
	pane.viewport.Height = 8
	colored := "\x1b[38;5;244m\"count\"\x1b[0m: \x1b[38;5;114m4\x1b[0m"
	pane.snapshot = &responseSnapshot{
		id:      "snap-colored-visible-4",
		pretty:  withTrailingNewline(colored),
		raw:     withTrailingNewline(stripANSIEscape(colored)),
		headers: withTrailingNewline("Status: 200 OK"),
		ready:   true,
	}

	status := statusFromCmd(t, model.applyResponseSearch("4", false))
	if status == nil {
		t.Fatal("expected search status")
	}
	if status.level != statusInfo {
		t.Fatalf("expected visible match, got %+v", status)
	}
	if len(pane.search.matches) != 1 {
		t.Fatalf("expected one visible match, got %d", len(pane.search.matches))
	}

	rendered := pane.viewport.View()
	plain := stripANSIEscape(rendered)
	if strings.Contains(plain, "38;5") || strings.Contains(plain, "48;2") {
		t.Fatalf("expected ANSI sequences to stay hidden after highlight, got %q", plain)
	}
	if !strings.Contains(plain, `"count": 4`) {
		t.Fatalf("expected highlighted view to preserve visible content, got %q", plain)
	}
}

func TestResponseSearchKeepsExplainStyledRenderer(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.focus = focusResponse
	model.responsePaneFocus = responsePanePrimary

	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatal("expected response pane to be available")
	}
	pane.activeTab = responseTabExplain
	pane.viewport.Width = 80
	pane.viewport.Height = 12
	pane.snapshot = &responseSnapshot{
		id:    "snap-explain-styled-search",
		ready: true,
		explain: explainState{
			report: &xplain.Report{
				Status: xplain.StatusReady,
				Stages: []xplain.Stage{
					{Name: "route", Status: xplain.StageOK, Summary: "direct connection"},
				},
			},
		},
	}

	status := statusFromCmd(t, model.applyResponseSearch("direct", false))
	if status == nil {
		t.Fatal("expected search status")
	}
	if !pane.search.active || len(pane.search.matches) == 0 {
		t.Fatalf(
			"expected explain search matches, active=%v matches=%d",
			pane.search.active,
			len(pane.search.matches),
		)
	}

	plain := stripANSIEscape(pane.viewport.View())
	if strings.Contains(plain, "Summary\n======") {
		t.Fatalf("expected styled explain view, got plain report %q", plain)
	}
	if !strings.Contains(plain, "SUMMARY") || !strings.Contains(plain, "direct connection") {
		t.Fatalf("expected styled explain content to remain visible, got %q", plain)
	}
}

func TestSearchModeStatusClearsWhenPromptCanceled(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.focus = focusEditor
	model.editorInsertMode = false

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	model = updated.(Model)
	if model.statusMessage.text != regexSearchStatus {
		t.Fatalf("expected search mode status, got %q", model.statusMessage.text)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	if model.showSearchPrompt {
		t.Fatalf("expected search prompt to close")
	}
	if model.statusMessage.text != "" {
		t.Fatalf("expected canceled search mode status to clear, got %q", model.statusMessage.text)
	}
}

func TestEnsureResponseMatchVisibleTargetsActualMatchLine(t *testing.T) {
	content := "\x1b[38;5;244mzero\x1b[0m\none\ntwo"
	visible := stripANSIEscape(content)
	start := len([]rune("zero\n"))
	end := start + len([]rune("one"))

	vp := viewport.New(20, 1)
	vp.SetContent(content)
	index := buildResponseSearchContentIndex(content)
	ensureResponseMatchVisible(&vp, &index, searchMatch{start: start, end: end})

	if vp.YOffset != 1 {
		t.Fatalf(
			"expected match line %q in one-line viewport at offset 1, got offset %d for %q",
			"one",
			vp.YOffset,
			visible,
		)
	}
}

func TestLiveSearchEmptyQueryClearsCurrentTarget(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.focus = focusResponse
	model.responsePaneFocus = responsePanePrimary
	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatal("expected response pane to be available")
	}
	pane.viewport.Width = 80
	pane.snapshot = &responseSnapshot{
		id:      "snap-1",
		pretty:  withTrailingNewline("foo bar\nfoo baz\nfoo"),
		raw:     withTrailingNewline("foo bar\nfoo baz\nfoo"),
		headers: withTrailingNewline("Status: 200 OK"),
		ready:   true,
	}
	if status := statusFromCmd(t, model.applyResponseSearch("foo", false)); status == nil {
		t.Fatal("expected initial response search status")
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = updated.(Model)
	for range "foo" {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		model = updated.(Model)
	}

	pane = model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatal("expected response pane to be available after clearing")
	}
	if pane.search.hasQuery() || pane.search.active || len(pane.search.matches) != 0 {
		t.Fatalf(
			"expected live empty query to clear response search, got query=%q active=%v matches=%d",
			pane.search.query,
			pane.search.active,
			len(pane.search.matches),
		)
	}
}

func TestLiveSearchRegexToggleReappliesCurrentInput(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.focus = focusEditor
	model.editorInsertMode = false
	model.editor.SetValue("foo\nfao\nbar")
	editorPtr := &model.editor
	editorPtr.moveCursorTo(0, 0)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = updated.(Model)
	for _, r := range "f.o" {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = updated.(Model)
	}
	if model.editor.SearchActive() {
		t.Fatal("did not expect literal f.o to match before regex toggle")
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	model = updated.(Model)
	if !model.searchIsRegex {
		t.Fatal("expected regex mode after ctrl+r")
	}
	if !model.editor.SearchActive() {
		t.Fatal("expected ctrl+r to reapply current input as regex")
	}
	if len(model.editor.search.matches) != 2 {
		t.Fatalf("expected 2 regex matches, got %d", len(model.editor.search.matches))
	}
	if model.editor.search.query != "f.o" {
		t.Fatalf("expected search query to stay f.o, got %q", model.editor.search.query)
	}
}

package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/ui/navigator"

	tea "github.com/charmbracelet/bubbletea"
)

func writeSampleFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func navigatorIndex(m *Model, id string) int {
	if m == nil || m.navigator == nil {
		return -1
	}
	for idx, row := range m.navigator.Rows() {
		if row.Node != nil && row.Node.ID == id {
			return idx
		}
	}
	return -1
}

func selectNavigatorID(t *testing.T, m *Model, id string) {
	t.Helper()
	if m == nil || m.navigator == nil {
		t.Fatalf("navigator unavailable")
	}
	rows := m.navigator.Rows()
	target := -1
	curr := -1
	sel := m.navigator.Selected()
	for idx, row := range rows {
		if row.Node == sel {
			curr = idx
		}
		if row.Node != nil && row.Node.ID == id {
			target = idx
		}
	}
	if target < 0 {
		t.Fatalf("id %s not found", id)
	}
	if curr < 0 {
		curr = 0
	}
	m.navigator.Move(target - curr)
}

func TestNavigatorEnterExpandsFile(t *testing.T) {
	tmp := t.TempDir()
	fileA := filepath.Join(tmp, "a.http")
	fileB := filepath.Join(tmp, "b.http")
	content := "### req\n# @name sample\nGET https://example.com\n"
	writeSampleFile(t, fileA, content)
	writeSampleFile(t, fileB, content)

	model := New(Config{WorkspaceRoot: tmp, FilePath: fileA})
	m := &model
	if cmd := m.openFile(fileA); cmd != nil {
		cmd()
	}

	target := navigatorIndex(m, "file:"+fileB)
	if target < 0 {
		t.Fatalf("expected navigator to include %s", fileB)
	}
	selectNavigatorID(t, m, "file:"+fileB)

	if cmd := m.updateNavigator(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		cmd()
	}

	node := m.navigator.Find("file:" + fileB)
	if node == nil {
		t.Fatalf("expected node for %s", fileB)
	}
	if !node.Expanded {
		t.Fatalf("expected %s to expand after enter", fileB)
	}
	if len(node.Children) == 0 {
		t.Fatalf("expected requests to load for %s", fileB)
	}
}

func TestNavigatorEnterDoesNotCollapseFile(t *testing.T) {
	tmp := t.TempDir()
	fileA := filepath.Join(tmp, "a.http")
	fileB := filepath.Join(tmp, "b.http")
	content := "### req\n# @name sample\nGET https://example.com\n"
	writeSampleFile(t, fileA, content)
	writeSampleFile(t, fileB, content)

	model := New(Config{WorkspaceRoot: tmp, FilePath: fileA})
	m := &model
	if cmd := m.openFile(fileA); cmd != nil {
		cmd()
	}

	target := navigatorIndex(m, "file:"+fileB)
	if target < 0 {
		t.Fatalf("expected navigator to include %s", fileB)
	}
	selectNavigatorID(t, m, "file:"+fileB)

	if cmd := m.updateNavigator(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		cmd()
	}

	node := m.navigator.Find("file:" + fileB)
	if node == nil || !node.Expanded {
		t.Fatalf("expected %s to expand after first enter", fileB)
	}

	if cmd := m.updateNavigator(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		cmd()
	}

	node = m.navigator.Find("file:" + fileB)
	if node == nil {
		t.Fatalf("expected node for %s after second enter", fileB)
	}
	if !node.Expanded {
		t.Fatalf("expected %s to stay expanded after second enter", fileB)
	}
}

func TestNavigatorRightDoesNotCollapseFile(t *testing.T) {
	tmp := t.TempDir()
	fileA := filepath.Join(tmp, "a.http")
	fileB := filepath.Join(tmp, "b.http")
	content := "### req\n# @name sample\nGET https://example.com\n"
	writeSampleFile(t, fileA, content)
	writeSampleFile(t, fileB, content)

	model := New(Config{WorkspaceRoot: tmp, FilePath: fileA})
	m := &model
	if cmd := m.openFile(fileA); cmd != nil {
		cmd()
	}

	target := navigatorIndex(m, "file:"+fileB)
	if target < 0 {
		t.Fatalf("expected navigator to include %s", fileB)
	}
	selectNavigatorID(t, m, "file:"+fileB)

	if cmd := m.updateNavigator(tea.KeyMsg{Type: tea.KeyRight}); cmd != nil {
		cmd()
	}

	node := m.navigator.Find("file:" + fileB)
	if node == nil {
		t.Fatalf("expected node for %s", fileB)
	}
	if !node.Expanded {
		t.Fatalf("expected %s to expand after first right", fileB)
	}
	if len(node.Children) == 0 {
		t.Fatalf("expected requests to load for %s", fileB)
	}

	if cmd := m.updateNavigator(tea.KeyMsg{Type: tea.KeyRight}); cmd != nil {
		cmd()
	}

	node = m.navigator.Find("file:" + fileB)
	if node == nil {
		t.Fatalf("expected node for %s after second right", fileB)
	}
	if !node.Expanded {
		t.Fatalf("expected %s to stay expanded after second right", fileB)
	}
}

func TestNavigatorMethodFilterExcludesMismatchedRequests(t *testing.T) {
	tmp := t.TempDir()
	fileA := filepath.Join(tmp, "a.http")
	content := "### first\n# @name first\nGET https://example.com/one\n\n### second\n# @name second\nPOST https://example.com/two\n"
	writeSampleFile(t, fileA, content)

	model := New(Config{WorkspaceRoot: tmp, FilePath: fileA})
	m := &model
	if cmd := m.openFile(fileA); cmd != nil {
		cmd()
	}

	// With no filters both requests should be present.
	all := m.navigator.Rows()
	if len(all) < 3 { // file + two requests
		t.Fatalf("expected file and two requests, got %d rows", len(all))
	}

	m.navigator.ToggleMethodFilter("GET")
	m.navigator.Refresh()
	filtered := m.navigator.VisibleRows()
	foundPost := false
	foundGet := false
	for _, row := range filtered {
		if row.Node == nil || row.Node.Kind != navigator.KindRequest {
			continue
		}
		switch row.Node.Method {
		case "GET":
			foundGet = true
		case "POST":
			foundPost = true
		}
	}
	if !foundGet {
		t.Fatalf("expected GET request to remain visible after filter")
	}
	if foundPost {
		t.Fatalf("expected POST request to be filtered out")
	}

	// Switching to POST should hide GET.
	m.navigator.ToggleMethodFilter("POST")
	m.navigator.Refresh()
	filtered = m.navigator.VisibleRows()
	foundPost = false
	foundGet = false
	for _, row := range filtered {
		if row.Node == nil || row.Node.Kind != navigator.KindRequest {
			continue
		}
		switch row.Node.Method {
		case "GET":
			foundGet = true
		case "POST":
			foundPost = true
		}
	}
	if !foundPost {
		t.Fatalf("expected POST request to remain visible after switching filter")
	}
	if foundGet {
		t.Fatalf("expected GET request to be filtered out after switching filter")
	}
}

func TestNavigatorTextFilterRespectsWordBoundaries(t *testing.T) {
	tmp := t.TempDir()
	fileA := filepath.Join(tmp, "a.http")
	content := "### first\n# @name first\nGET https://example.com/one\n\n### second\n# @name second\nPOST https://example.com/two\n# @description Demonstrates @global working together\n"
	writeSampleFile(t, fileA, content)

	model := New(Config{WorkspaceRoot: tmp, FilePath: fileA})
	m := &model
	if cmd := m.openFile(fileA); cmd != nil {
		cmd()
	}

	m.navigatorFilter.SetValue("get")
	m.navigator.SetFilter(m.navigatorFilter.Value())
	m.ensureNavigatorDataForFilter()
	rows := m.navigator.VisibleRows()
	foundPost := false
	for _, row := range rows {
		if row.Node == nil || row.Node.Kind != navigator.KindRequest {
			continue
		}
		if row.Node.Method == "POST" {
			foundPost = true
		}
	}
	if foundPost {
		t.Fatalf("expected POST request with 'together' in description to be excluded when filtering GET")
	}
}

func TestNavigatorSelectionClearsRequestForOtherFile(t *testing.T) {
	tmp := t.TempDir()
	fileA := filepath.Join(tmp, "one.http")
	fileB := filepath.Join(tmp, "two.http")
	writeSampleFile(t, fileA, "### one\n# @name one\nGET https://one.test\n")
	writeSampleFile(t, fileB, "### two\n# @name two\nGET https://two.test\n")

	model := New(Config{WorkspaceRoot: tmp, FilePath: fileA})
	m := &model
	if cmd := m.openFile(fileA); cmd != nil {
		cmd()
	}
	if m.requestList.Index() < 0 {
		t.Fatalf("expected request selection for %s", fileA)
	}

	idx := navigatorIndex(m, "file:"+fileB)
	if idx < 0 {
		t.Fatalf("expected navigator to include %s", fileB)
	}
	selectNavigatorID(t, m, "file:"+fileB)
	m.syncNavigatorSelection()

	if m.requestList.Index() != -1 {
		t.Fatalf("expected request selection to clear when switching files")
	}
	if m.currentRequest != nil {
		t.Fatalf("expected active request to clear")
	}

	if cmd := m.updateNavigator(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		cmd()
	}
	if node := m.navigator.Find("file:" + fileB); node == nil || !node.Expanded {
		t.Fatalf("expected %s to expand on enter", fileB)
	}
	if m.requestList.Index() != -1 {
		t.Fatalf("expected request selection to stay cleared after expansion")
	}
}

func TestNavigatorFilterLoadsOtherFiles(t *testing.T) {
	tmp := t.TempDir()
	fileA := filepath.Join(tmp, "alpha.http")
	fileB := filepath.Join(tmp, "bravo.http")
	writeSampleFile(t, fileA, "### alpha\n# @name first\nGET https://one.test\n")
	writeSampleFile(t, fileB, "### bravo\n# @name second\nPOST https://two.test\n")

	model := New(Config{WorkspaceRoot: tmp, FilePath: fileA})
	m := &model
	if cmd := m.openFile(fileA); cmd != nil {
		cmd()
	}

	// Before filtering, the second file should not have loaded children.
	if node := m.navigator.Find("file:" + fileB); node == nil || len(node.Children) != 0 {
		t.Fatalf("expected %s to start without children", fileB)
	}

	// Apply a filter that matches the second file's request name.
	m.navigatorFilter.SetValue("second")
	m.navigator.SetFilter(m.navigatorFilter.Value())
	m.ensureNavigatorDataForFilter()

	node := m.navigator.Find("file:" + fileB)
	if node == nil {
		t.Fatalf("expected navigator node for %s", fileB)
	}
	if len(node.Children) == 0 {
		t.Fatalf("expected %s children to load after filter", fileB)
	}
	found := false
	for _, child := range node.Children {
		if child.Kind == navigator.KindRequest && strings.Contains(strings.ToLower(child.Title), "second") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected filtered request to be present for %s", fileB)
	}
}

func TestNavigatorEscClearsFilters(t *testing.T) {
	model := New(Config{})
	m := &model
	m.navigator = navigator.New[any]([]*navigator.Node[any]{
		{ID: "file:/tmp/a", Kind: navigator.KindFile, Payload: navigator.Payload[any]{FilePath: "/tmp/a"}},
	})
	m.ensureNavigatorFilter()
	m.navigatorFilter.SetValue("abc")
	m.navigator.ToggleMethodFilter("GET")
	m.navigator.ToggleTagFilter("foo")
	m.navigatorFilter.Focus()

	_ = m.updateNavigator(tea.KeyMsg{Type: tea.KeyEsc})

	if m.navigatorFilter.Value() != "" {
		t.Fatalf("expected filter to clear on esc, got %q", m.navigatorFilter.Value())
	}
	if m.navigatorFilter.Focused() {
		t.Fatalf("expected filter to blur on esc")
	}
	if len(m.navigator.MethodFilters()) != 0 {
		t.Fatalf("expected method filters to clear on esc")
	}
	if len(m.navigator.TagFilters()) != 0 {
		t.Fatalf("expected tag filters to clear on esc")
	}
}

func TestNavigatorFilterTypingIgnoresNavShortcuts(t *testing.T) {
	model := New(Config{})
	m := &model
	m.navigator = navigator.New[any]([]*navigator.Node[any]{
		{
			ID:       "file:/tmp/a",
			Title:    "ghost",
			Kind:     navigator.KindFile,
			Payload:  navigator.Payload[any]{FilePath: "/tmp/a"},
			Expanded: true,
			Children: []*navigator.Node[any]{
				{ID: "req:/tmp/a:0", Kind: navigator.KindRequest, Title: "get", Method: "GET", Payload: navigator.Payload[any]{FilePath: "/tmp/a"}},
			},
		},
	})
	m.ensureNavigatorFilter()
	m.navigatorFilter.Focus()

	sendKey := func(msg tea.KeyMsg) {
		if cmd := m.handleKey(msg); cmd != nil {
			cmd()
		}
		if cmd := m.updateNavigator(msg); cmd != nil {
			cmd()
		}
	}

	sendKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if !m.navigatorFilter.Focused() {
		t.Fatalf("expected filter to stay focused while typing")
	}
	if got := m.navigatorFilter.Value(); got != "g" {
		t.Fatalf("expected filter to capture typed runes, got %q", got)
	}
	if m.hasPendingChord || m.pendingChord != "" {
		t.Fatalf("expected global chord to stay inactive while typing filter")
	}
	if sel := m.navigator.Selected(); sel == nil || !sel.Expanded {
		t.Fatalf("expected navigator selection to stay expanded while typing")
	}

	sendKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if got := m.navigatorFilter.Value(); got != "gh" {
		t.Fatalf("expected filter to continue capturing runes, got %q", got)
	}
	if m.hasPendingChord || m.pendingChord != "" {
		t.Fatalf("expected chord prefix to remain inactive after additional typing")
	}
	if sel := m.navigator.Selected(); sel == nil || !sel.Expanded {
		t.Fatalf("expected navigator to ignore collapse shortcuts while filter is focused")
	}

	sendKey(tea.KeyMsg{Type: tea.KeyLeft})
	if got := m.navigatorFilter.Value(); got != "gh" {
		t.Fatalf("expected navigation arrow to avoid collapsing and preserve filter, got %q", got)
	}
	if sel := m.navigator.Selected(); sel == nil || !sel.Expanded {
		t.Fatalf("expected left arrow to move cursor only when filter is focused")
	}
}

func TestNavigatorRequestEnterSendsFromSidebar(t *testing.T) {
	model := newTestModelWithDoc(sampleRequestDoc)
	m := model
	m.ready = true
	m.currentFile = "/tmp/sample.http"
	m.cfg.FilePath = m.currentFile
	m.syncRequestList(m.doc)

	if len(m.doc.Requests) == 0 {
		t.Fatalf("expected parsed requests in doc")
	}
	if len(m.requestItems) == 0 {
		t.Fatalf("expected request items after sync")
	}
	if idx := m.requestList.Index(); idx != 0 {
		t.Fatalf("expected request list to select first item after sync, got %d", idx)
	}
	if m.activeRequestKey == "" {
		t.Fatalf("expected active request key after sync")
	}
	t.Logf("active key before navigator sync: %s", m.activeRequestKey)
	t.Logf("request index before navigator sync: %d", m.requestList.Index())

	req := m.doc.Requests[0]
	m.navigator = navigator.New[any]([]*navigator.Node[any]{
		{
			ID:      "req:/tmp/sample:0",
			Kind:    navigator.KindRequest,
			Payload: navigator.Payload[any]{FilePath: m.currentFile, Data: req},
		},
	})

	if m.focus != focusFile {
		t.Fatalf("expected initial focus on file pane, got %v", m.focus)
	}

	m.syncNavigatorSelection()
	t.Logf("request index after navigator sync: %d", m.requestList.Index())

	if m.focus != focusRequests {
		t.Fatalf("expected navigator request selection to move focus to requests, got %v", m.focus)
	}
	if sel := m.navigator.Selected(); sel == nil || sel.Kind != navigator.KindRequest {
		t.Fatalf("expected navigator selection to be a request, got %v", sel)
	}
	if _, ok := m.navigator.Selected().Payload.Data.(*restfile.Request); !ok {
		t.Fatalf("expected navigator selection payload to be request, got %T", m.navigator.Selected().Payload.Data)
	}
	if sel := m.navigator.Selected(); sel == nil || !samePath(sel.Payload.FilePath, m.currentFile) {
		t.Fatalf("expected navigator selection to target current file, got %v vs %q", sel, m.currentFile)
	}
	if m.activeRequestKey == "" {
		t.Fatalf("expected active request to remain selected after navigator sync")
	}

	if !m.navGate(navigator.KindRequest, "") {
		t.Fatalf("expected navGate to allow request actions for current file selection")
	}
	if idx := m.requestList.Index(); idx < 0 {
		sel := m.navigator.Selected()
		path := ""
		if sel != nil {
			path = sel.Payload.FilePath
		}
		items := m.requestList.Items()
		t.Fatalf("expected request list selection, got %d (items=%d path=%q current=%q)", idx, len(items), path, m.currentFile)
	}
	if _, ok := m.requestList.SelectedItem().(requestListItem); !ok {
		t.Fatalf("expected request list item to be selected")
	}

	cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected enter to issue send command from navigator request selection")
	}
}

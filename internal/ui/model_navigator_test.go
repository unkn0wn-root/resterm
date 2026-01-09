package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/parser"
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

func TestNavigatorFollowsEditorCursor(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "sample.http")
	content := "### first\nGET https://example.com/one\n\n### second\nGET https://example.com/two\n"
	writeSampleFile(t, file, content)

	model := New(Config{WorkspaceRoot: tmp, FilePath: file, InitialContent: content})
	m := &model

	_ = m.setFocus(focusEditor)

	firstStart := m.doc.Requests[0].LineRange.Start
	m.moveCursorToLine(firstStart)
	if sel := m.navigator.Selected(); sel == nil || sel.ID != navigatorRequestID(file, 0) {
		t.Fatalf("expected navigator to select first request, got %#v", sel)
	}

	secondStart := m.doc.Requests[1].LineRange.Start
	m.moveCursorToLine(secondStart)

	if sel := m.navigator.Selected(); sel == nil || sel.ID != navigatorRequestID(file, 1) {
		t.Fatalf("expected navigator to select second request after cursor move, got %#v", sel)
	}
	if key := requestKey(m.doc.Requests[1]); m.activeRequestKey != key {
		t.Fatalf("expected active request key %s, got %s", key, m.activeRequestKey)
	}
}

func TestNavigatorIgnoresLinesOutsideRequests(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "preface.http")
	content := "# preface\n\n### first\nGET https://example.com/one\n\n### second\nGET https://example.com/two\n"
	writeSampleFile(t, file, content)

	model := New(Config{WorkspaceRoot: tmp, FilePath: file, InitialContent: content})
	m := &model

	_ = m.setFocus(focusEditor)

	firstStart := m.doc.Requests[0].LineRange.Start
	m.moveCursorToLine(firstStart)
	firstID := navigatorRequestID(file, 0)
	if sel := m.navigator.Selected(); sel == nil || sel.ID != firstID {
		t.Fatalf("expected navigator to select first request, got %#v", sel)
	}

	m.moveCursorToLine(1)

	if sel := m.navigator.Selected(); sel == nil || sel.ID != firstID {
		t.Fatalf(
			"expected navigator to keep first request selected on non-request line, got %#v",
			sel,
		)
	}
}

func TestNavigatorFollowsCursorAtEOF(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "eof.http")
	content := "### first\nGET https://example.com/one\n\n### second\nGET https://example.com/two\n\n"
	writeSampleFile(t, file, content)

	model := New(Config{WorkspaceRoot: tmp, FilePath: file, InitialContent: content})
	m := &model

	_ = m.setFocus(focusEditor)

	endLine := strings.Count(content, "\n") + 1
	m.moveCursorToLine(endLine)

	lastID := navigatorRequestID(file, 1)
	if sel := m.navigator.Selected(); sel == nil || sel.ID != lastID {
		t.Fatalf("expected navigator to select last request at EOF, got %#v", sel)
	}
	if key := requestKey(m.doc.Requests[1]); m.activeRequestKey != key {
		t.Fatalf(
			"expected active request to follow last request at EOF, got %s",
			m.activeRequestKey,
		)
	}
}

func TestNavigatorCursorSyncPreservesFiltersWithinRequest(t *testing.T) {
	content := "### one\nGET https://example.com/one\n\n### two\nGET https://example.com/two\n"
	file := "/tmp/navsync.http"
	model := newTestModelWithDoc(content)
	m := model
	m.currentFile = file
	m.cfg.FilePath = file
	m.doc = parser.Parse(file, []byte(content))
	m.syncRequestList(m.doc)

	nodes := []*navigator.Node[any]{
		{
			ID:       "file:" + file,
			Kind:     navigator.KindFile,
			Payload:  navigator.Payload[any]{FilePath: file},
			Expanded: true,
			Children: []*navigator.Node[any]{
				{
					ID:      navigatorRequestID(file, 0),
					Kind:    navigator.KindRequest,
					Payload: navigator.Payload[any]{FilePath: file, Data: m.doc.Requests[0]},
				},
				{
					ID:      navigatorRequestID(file, 1),
					Kind:    navigator.KindRequest,
					Payload: navigator.Payload[any]{FilePath: file, Data: m.doc.Requests[1]},
				},
			},
		},
	}
	m.navigator = navigator.New(nodes)
	_ = m.setFocus(focusEditor)

	firstStart := m.doc.Requests[0].LineRange.Start
	m.moveCursorToLine(firstStart)
	m.streamFilterActive = true
	m.streamFilterInput.SetValue("trace")

	m.moveCursorToLine(firstStart + 1)
	if !m.streamFilterActive {
		t.Fatalf("expected stream filter to remain active within request")
	}
	if got := m.streamFilterInput.Value(); got != "trace" {
		t.Fatalf("expected stream filter value to remain, got %q", got)
	}
	if key := requestKey(m.doc.Requests[0]); m.activeRequestKey != key {
		t.Fatalf("expected active request to stay on first request, got %s", m.activeRequestKey)
	}

	secondStart := m.doc.Requests[1].LineRange.Start
	m.moveCursorToLine(secondStart)

	if m.streamFilterActive {
		t.Fatalf("expected stream filter to reset after switching requests")
	}
	if val := m.streamFilterInput.Value(); val != "" {
		t.Fatalf("expected stream filter input to clear, got %q", val)
	}
	if key := requestKey(m.doc.Requests[1]); m.activeRequestKey != key {
		t.Fatalf("expected active request to switch to second request, got %s", m.activeRequestKey)
	}
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

func TestNavigatorEmptyFileStaysCollapsed(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "empty.http")
	writeSampleFile(t, file, "")

	model := New(Config{WorkspaceRoot: tmp, FilePath: file})
	m := &model

	selectNavigatorID(t, m, "file:"+file)
	if cmd := m.updateNavigator(tea.KeyMsg{Type: tea.KeyRight}); cmd != nil {
		cmd()
	}

	node := m.navigator.Find("file:" + file)
	if node == nil {
		t.Fatalf("expected node for %s", file)
	}
	if node.Expanded {
		t.Fatalf("expected %s to stay collapsed", file)
	}
	if len(node.Children) != 0 {
		t.Fatalf("expected no children for %s", file)
	}
}

func TestNavigatorLFocusesEditorForRTS(t *testing.T) {
	tmp := t.TempDir()
	fileA := filepath.Join(tmp, "a.http")
	fileB := filepath.Join(tmp, "helpers.rts")
	content := "### req\nGET https://example.com\n"
	writeSampleFile(t, fileA, content)
	writeSampleFile(t, fileB, "fn add(a, b) { return a + b }\n")

	model := New(Config{WorkspaceRoot: tmp, FilePath: fileA})
	m := &model

	selectNavigatorID(t, m, "file:"+fileB)

	if cmd := m.updateNavigator(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}}); cmd != nil {
		cmd()
	}

	if m.focus != focusEditor {
		t.Fatalf("expected focus to move to editor, got %v", m.focus)
	}
	if filepath.Clean(m.currentFile) != filepath.Clean(fileB) {
		t.Fatalf("expected current file %s, got %s", fileB, m.currentFile)
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
		t.Fatalf(
			"expected POST request with 'together' in description to be excluded when filtering GET",
		)
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
		if child.Kind == navigator.KindRequest &&
			strings.Contains(strings.ToLower(child.Title), "second") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected filtered request to be present for %s", fileB)
	}
}

func TestNavigatorBuildsDirectoryTree(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "root.http")
	dir := filepath.Join(tmp, "rtsfiles")
	nested := filepath.Join(dir, "nested")
	fileA := filepath.Join(dir, "one.http")
	fileB := filepath.Join(dir, "mod.rts")
	fileC := filepath.Join(nested, "two.http")

	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", nested, err)
	}
	writeSampleFile(t, root, "### root\nGET https://example.com\n")
	writeSampleFile(t, fileA, "### one\nGET https://example.com/one\n")
	writeSampleFile(t, fileB, "export const x = 1\n")
	writeSampleFile(t, fileC, "### two\nGET https://example.com/two\n")

	model := New(Config{WorkspaceRoot: tmp, Recursive: true})
	m := &model

	dirID := "dir:" + dir
	dirNode := m.navigator.Find(dirID)
	if dirNode == nil || dirNode.Kind != navigator.KindDir {
		t.Fatalf("expected directory node for %s", dir)
	}

	findChild := func(n *navigator.Node[any], id string) *navigator.Node[any] {
		for _, c := range n.Children {
			if c != nil && c.ID == id {
				return c
			}
		}
		return nil
	}

	childA := findChild(dirNode, "file:"+fileA)
	if childA == nil || childA.Title != "one.http" {
		t.Fatalf("expected child file %s with title one.http", fileA)
	}
	childB := findChild(dirNode, "file:"+fileB)
	if childB == nil || childB.Title != "mod.rts" {
		t.Fatalf("expected child file %s with title mod.rts", fileB)
	}
	childDir := findChild(dirNode, "dir:"+nested)
	if childDir == nil || childDir.Kind != navigator.KindDir || childDir.Title != "nested" {
		t.Fatalf("expected nested directory node %s", nested)
	}
	if nestedChild := findChild(childDir, "file:"+fileC); nestedChild == nil {
		t.Fatalf("expected nested file %s under %s", fileC, nested)
	}
}

func TestNavigatorDirFirstSort(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "alpha")
	nested := filepath.Join(dir, "beta")
	rootFile := filepath.Join(tmp, "zeta.http")
	dirFile := filepath.Join(dir, "a.http")
	nestedFile := filepath.Join(nested, "b.http")

	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", nested, err)
	}
	writeSampleFile(t, rootFile, "### root\nGET https://example.com\n")
	writeSampleFile(t, dirFile, "### a\nGET https://example.com/a\n")
	writeSampleFile(t, nestedFile, "### b\nGET https://example.com/b\n")

	model := New(Config{WorkspaceRoot: tmp, Recursive: true})
	m := &model

	rows := m.navigator.Rows()
	if len(rows) < 2 {
		t.Fatalf("expected at least 2 rows, got %d", len(rows))
	}
	if rows[0].Node == nil || rows[0].Node.Kind != navigator.KindDir {
		t.Fatalf("expected first row to be dir, got %+v", rows[0].Node)
	}
	if rows[1].Node == nil || rows[1].Node.Kind != navigator.KindFile {
		t.Fatalf("expected second row to be file, got %+v", rows[1].Node)
	}

	dirNode := m.navigator.Find("dir:" + dir)
	if dirNode == nil {
		t.Fatalf("expected dir node for %s", dir)
	}
	if len(dirNode.Children) < 2 {
		t.Fatalf("expected dir node to have children, got %d", len(dirNode.Children))
	}
	if dirNode.Children[0].Kind != navigator.KindDir || dirNode.Children[0].Title != "beta" {
		t.Fatalf("expected nested dir first under %s", dir)
	}
	if dirNode.Children[1].Kind != navigator.KindFile || dirNode.Children[1].Title != "a.http" {
		t.Fatalf("expected file after dir under %s", dir)
	}
}

func TestNavigatorEscClearsFilters(t *testing.T) {
	model := New(Config{})
	m := &model
	m.navigator = navigator.New[any]([]*navigator.Node[any]{
		{
			ID:      "file:/tmp/a",
			Kind:    navigator.KindFile,
			Payload: navigator.Payload[any]{FilePath: "/tmp/a"},
		},
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
				{
					ID:      "req:/tmp/a:0",
					Kind:    navigator.KindRequest,
					Title:   "get",
					Method:  "GET",
					Payload: navigator.Payload[any]{FilePath: "/tmp/a"},
				},
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
		t.Fatalf(
			"expected navigator selection payload to be request, got %T",
			m.navigator.Selected().Payload.Data,
		)
	}
	if sel := m.navigator.Selected(); sel == nil || !samePath(sel.Payload.FilePath, m.currentFile) {
		t.Fatalf(
			"expected navigator selection to target current file, got %v vs %q",
			sel,
			m.currentFile,
		)
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
		t.Fatalf(
			"expected request list selection, got %d (items=%d path=%q current=%q)",
			idx,
			len(items),
			path,
			m.currentFile,
		)
	}
	if _, ok := m.requestList.SelectedItem().(requestListItem); !ok {
		t.Fatalf("expected request list item to be selected")
	}

	cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected enter to issue send command from navigator request selection")
	}
}

func TestNavigatorRequestLJumpsToDefinition(t *testing.T) {
	content := strings.Repeat(
		"\n",
		5,
	) + "### example\n# @name getExample\nGET https://example.com\n"
	model := newTestModelWithDoc(content)
	m := model
	m.currentFile = "/tmp/sample.http"
	m.cfg.FilePath = m.currentFile
	m.syncRequestList(m.doc)

	if len(m.doc.Requests) == 0 {
		t.Fatalf("expected parsed requests in doc")
	}
	req := m.doc.Requests[0]
	if req.LineRange.Start <= 1 {
		t.Fatalf("expected request to start after line 1, got %d", req.LineRange.Start)
	}
	if got := currentCursorLine(m.editor); got != 1 {
		t.Fatalf("expected cursor to start on line 1, got %d", got)
	}

	m.navigator = navigator.New[any]([]*navigator.Node[any]{
		{
			ID:      "req:/tmp/sample:0",
			Kind:    navigator.KindRequest,
			Payload: navigator.Payload[any]{FilePath: m.currentFile, Data: req},
		},
	})

	if res := m.setCollapseState(paneRegionEditor, true); res.blocked {
		t.Fatalf("expected editor collapse to be allowed")
	}
	if !m.collapseState(paneRegionEditor) {
		t.Fatalf("expected editor to start collapsed")
	}

	if cmd := m.updateNavigator(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}}); cmd != nil {
		cmd()
	}

	if m.collapseState(paneRegionEditor) {
		t.Fatalf("expected editor to be restored")
	}
	if m.focus != focusEditor {
		t.Fatalf("expected focus to move to editor, got %v", m.focus)
	}
	if got := currentCursorLine(m.editor); got != req.LineRange.Start {
		t.Fatalf("expected cursor to jump to line %d, got %d", req.LineRange.Start, got)
	}
}

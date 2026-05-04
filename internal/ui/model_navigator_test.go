package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/ui/navigator"
	"github.com/unkn0wn-root/resterm/internal/util"

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

func applyModelUpdate(t *testing.T, m *Model, msg tea.Msg) *Model {
	t.Helper()
	updated, _ := m.Update(msg)
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected ui.Model update to return Model, got %T", updated)
	}
	return &next
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

func TestNavigatorResyncsSelectionWhenCursorUnchanged(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "sample.http")
	content := "### first\nGET https://example.com/one\n\n### second\nGET https://example.com/two\n"
	writeSampleFile(t, file, content)

	model := New(Config{WorkspaceRoot: tmp, FilePath: file, InitialContent: content})
	m := &model

	_ = m.setFocus(focusEditor)

	secondStart := m.doc.Requests[1].LineRange.Start
	secondID := navigatorRequestID(file, 1)
	m.moveCursorToLine(secondStart)
	if sel := m.navigator.Selected(); sel == nil || sel.ID != secondID {
		t.Fatalf("expected navigator to select second request, got %#v", sel)
	}

	firstID := navigatorRequestID(file, 0)
	if !m.navigator.SelectByID(firstID) {
		t.Fatalf("expected navigator to select first request")
	}

	m.syncNavigatorWithEditorCursor()
	if sel := m.navigator.Selected(); sel == nil || sel.ID != secondID {
		t.Fatalf("expected navigator to resync second request, got %#v", sel)
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

func TestNavigatorIncludesActiveEnvFile(t *testing.T) {
	tmp := t.TempDir()
	reqFile := filepath.Join(tmp, "sample.http")
	envFile := filepath.Join(tmp, "resterm.env.json")
	writeSampleFile(t, reqFile, "GET https://example.com\n")
	writeSampleFile(t, envFile, "{\n  \"dev\": {\"baseUrl\": \"https://example.com\"}\n}\n")

	model := New(Config{
		WorkspaceRoot:   tmp,
		FilePath:        reqFile,
		EnvironmentFile: envFile,
	})
	m := &model

	node := m.navigator.Find("file:" + envFile)
	if node == nil {
		t.Fatalf("expected navigator node for %s", envFile)
	}
	if len(node.Children) != 0 {
		t.Fatalf("expected env file to remain a leaf node")
	}
	if got := strings.Join(node.Badges, ","); got != "ENV,ACTIVE" {
		t.Fatalf("expected ENV and ACTIVE badges, got %q", got)
	}
}

func TestNavigatorIncludesExplicitEnvFileInsideWorkspace(t *testing.T) {
	tmp := t.TempDir()
	reqFile := filepath.Join(tmp, "sample.http")
	envDir := filepath.Join(tmp, "config")
	envFile := filepath.Join(envDir, ".env.local")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	writeSampleFile(t, reqFile, "GET https://example.com\n")
	writeSampleFile(t, envFile, "workspace=dev\nAPI_URL=https://example.com\n")

	model := New(Config{
		WorkspaceRoot:   tmp,
		FilePath:        reqFile,
		EnvironmentFile: envFile,
	})
	m := &model

	node := m.navigator.Find("file:" + envFile)
	if node == nil {
		t.Fatalf("expected navigator node for explicit env file %s", envFile)
	}
	if got := strings.Join(node.Badges, ","); got != "ENV,ACTIVE" {
		t.Fatalf("expected ENV and ACTIVE badges, got %q", got)
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

func TestNavigatorSpaceExpandsUnloadedFile(t *testing.T) {
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

	selectNavigatorID(t, m, "file:"+fileB)
	if cmd := m.updateNavigator(tea.KeyMsg{Type: tea.KeySpace}); cmd != nil {
		cmd()
	}

	node := m.navigator.Find("file:" + fileB)
	if node == nil {
		t.Fatalf("expected node for %s", fileB)
	}
	if !node.Expanded {
		t.Fatalf("expected %s to expand after space", fileB)
	}
	if len(node.Children) == 0 {
		t.Fatalf("expected requests to load for %s", fileB)
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
	content := "# @use ./helpers.rts\n### req\nGET https://example.com\n"
	writeSampleFile(t, fileA, content)
	writeSampleFile(t, fileB, "fn add(a, b) { return a + b }\n")

	model := New(Config{WorkspaceRoot: tmp, FilePath: fileA, InitialContent: content})
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

func TestNavigatorWorkflowSelectionPreservesActiveRequest(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "workflow.http")
	writeSampleFile(t, file, `# @workflow sample-order on-failure=continue
# @step Cleanup using=DeleteResource

### Cleanup resource
# @name DeleteResource
# @auth bearer {{auth.token}}
DELETE https://example.com/resource
`)

	model := New(Config{WorkspaceRoot: tmp, FilePath: file})
	m := &model
	if cmd := m.openFile(file); cmd != nil {
		cmd()
	}

	reqID := navigatorRequestID(file, 0)
	selectNavigatorID(t, m, reqID)
	m.syncNavigatorSelection()
	if m.currentRequest == nil || m.activeRequestKey == "" {
		t.Fatalf("expected request selection to activate request")
	}
	activeReq := m.currentRequest
	activeReqKey := m.activeRequestKey
	reqIndex := m.requestList.Index()

	wfID := "wf:" + file + ":0"
	selectNavigatorID(t, m, wfID)
	m.syncNavigatorSelection()

	if m.currentRequest != activeReq || m.activeRequestKey != activeReqKey {
		t.Fatalf(
			"expected workflow selection to preserve active request %q, got %q",
			activeReqKey,
			m.activeRequestKey,
		)
	}
	if m.requestList.Index() != reqIndex {
		t.Fatalf("expected request list selection %d, got %d", reqIndex, m.requestList.Index())
	}
	if m.workflowSelectionKey == "" {
		t.Fatalf("expected workflow selection to activate workflow key")
	}
	if state := m.navigatorRenderState(); state.ActiveNodeID != "" {
		t.Fatalf("expected workflow selection not to render request marker, got %q", state.ActiveNodeID)
	}
}

func TestNavigatorNilSelectionPreservesWorkflowSelectionKey(t *testing.T) {
	content := `# @workflow sample-order
# @step Fetch using=FetchExample

### Fetch
# @name FetchExample
GET https://example.com
`
	model := newTestModelWithDoc(content)
	m := model
	m.currentFile = "/tmp/workflow.http"
	m.cfg.FilePath = m.currentFile
	m.syncRequestList(m.doc)

	if len(m.doc.Workflows) == 0 {
		t.Fatalf("expected workflow")
	}
	key := workflowKey(&m.doc.Workflows[0])
	m.workflowSelectionKey = key
	if !m.selectWorkflowItemByKey(key) {
		t.Fatalf("expected workflow list selection for key %q", key)
	}

	m.navigator = navigator.New[any](nil)
	m.syncNavigatorSelection()

	if m.workflowSelectionKey != key {
		t.Fatalf("expected workflow selection key %q to be preserved, got %q", key, m.workflowSelectionKey)
	}
	if m.workflowList.Index() != -1 {
		t.Fatalf("expected empty navigator selection to clear visible workflow selection")
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
	writeSampleFile(t, fileA, "# @use ./mod.rts\n### one\nGET https://example.com/one\n")
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

func TestNavigatorIncludesReferencedAuxiliaryWorkspaceFiles(t *testing.T) {
	tmp := t.TempDir()
	apiFile := filepath.Join(tmp, "api.http")
	queryFile := filepath.Join(tmp, "addNote.graphql")
	varsFile := filepath.Join(tmp, "addNote.variables.json")
	scriptFile := filepath.Join(tmp, "pre.js")
	orphanFile := filepath.Join(tmp, "orphan.json")

	writeSampleFile(t, apiFile, `# @script pre-request
> < ./pre.js
# @graphql
# @query < ./addNote.graphql
# @variables < ./addNote.variables.json
POST https://example.com/graphql
Content-Type: application/json
`)
	writeSampleFile(t, queryFile, "mutation AddNote { addNote { id } }\n")
	writeSampleFile(t, varsFile, `{"id":"1"}`)
	writeSampleFile(t, scriptFile, "request.setHeader('X-Test', '1');\n")
	writeSampleFile(t, orphanFile, `{}`)

	model := New(Config{WorkspaceRoot: tmp})
	m := &model

	tests := []struct {
		path string
		kind filesvc.FileKind
	}{
		{path: queryFile, kind: filesvc.FileKindGraphQL},
		{path: varsFile, kind: filesvc.FileKindJSON},
		{path: scriptFile, kind: filesvc.FileKindJavaScript},
	}

	for _, tt := range tests {
		node := m.navigator.Find("file:" + tt.path)
		if node == nil {
			t.Fatalf("expected navigator node for %s", tt.path)
		}
		entry, ok := node.Payload.Data.(filesvc.FileEntry)
		if !ok {
			t.Fatalf(
				"expected filesvc.FileEntry payload for %s, got %T",
				tt.path,
				node.Payload.Data,
			)
		}
		if entry.Kind != tt.kind {
			t.Fatalf("expected kind %v for %s, got %v", tt.kind, tt.path, entry.Kind)
		}
		if len(node.Badges) != 0 {
			t.Fatalf("expected no extension-derived badges for %s, got %+v", tt.path, node.Badges)
		}
		if len(node.Children) != 0 || node.Count != 0 {
			t.Fatalf(
				"expected auxiliary file to be a leaf node, got count=%d children=%d",
				node.Count,
				len(node.Children),
			)
		}
	}
	if node := m.navigator.Find("file:" + orphanFile); node != nil {
		t.Fatalf("did not expect unreferenced auxiliary file in navigator")
	}

	m.navigator.SetFilter("addNote.graphql")
	rows := m.navigator.Rows()
	if len(rows) != 1 || rows[0].Node == nil || rows[0].Node.ID != "file:"+queryFile {
		t.Fatalf("expected filename filter to find graphql file, got %+v", rows)
	}
}

func TestRequestBadgesDoesNotDuplicateGRPCMethod(t *testing.T) {
	req := &restfile.Request{
		GRPC: &restfile.GRPCRequest{},
	}
	if got := requestBadges(req); len(got) != 0 {
		t.Fatalf("expected no automatic gRPC badge, got %+v", got)
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
	if sel := m.navigator.Selected(); sel == nil || !util.SamePath(sel.Payload.FilePath, m.currentFile) {
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

func TestNavigatorRequestLKeepsLongFirstRequestCursorVisible(t *testing.T) {
	content := strings.Repeat("\n", 5) +
		"### Long first request\n# @name LongFirst\n" +
		strings.Repeat("# @assert true == true\n", 32) +
		"GET https://example.com/long\n\n" +
		"### second\n# @name Second\nGET https://example.com/second\n"
	model := newTestModelWithDoc(content)
	m := model
	m.currentFile = "/tmp/long-first.http"
	m.cfg.FilePath = m.currentFile
	m.editor.SetHeight(8)
	m.syncRequestList(m.doc)

	req := m.doc.Requests[0]
	if req.LineRange.End-req.LineRange.Start < m.editor.Height() {
		t.Fatalf("expected first request to exceed editor viewport")
	}
	m.moveCursorToLine(req.LineRange.Start)
	m.editor.SetViewStart(0)
	m.navigator = navigator.New[any]([]*navigator.Node[any]{
		{
			ID:      navigatorRequestID(m.currentFile, 0),
			Kind:    navigator.KindRequest,
			Payload: navigator.Payload[any]{FilePath: m.currentFile, Data: req},
		},
	})
	m.focus = focusRequests

	if cmd := m.updateNavigator(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}}); cmd != nil {
		cmd()
	}

	cursorLine := currentCursorLine(m.editor)
	if cursorLine != req.LineRange.Start {
		t.Fatalf("expected cursor to stay on request line %d, got %d", req.LineRange.Start, cursorLine)
	}
	viewStart := m.editor.ViewStart()
	cursorOffset := cursorLine - 1
	if cursorOffset < viewStart || cursorOffset >= viewStart+m.editor.Height() {
		t.Fatalf(
			"expected cursor line %d to be visible in viewport [%d,%d)",
			cursorLine,
			viewStart+1,
			viewStart+m.editor.Height()+1,
		)
	}
}

func TestNavigatorRightDoesNotJumpToRequestDefinition(t *testing.T) {
	content := strings.Repeat(
		"\n",
		5,
	) + "### example\n# @name getExample\nGET https://example.com\n"
	model := newTestModelWithDoc(content)
	m := model
	m.currentFile = "/tmp/sample.http"
	m.cfg.FilePath = m.currentFile
	m.syncRequestList(m.doc)

	req := m.doc.Requests[0]
	m.navigator = navigator.New[any]([]*navigator.Node[any]{
		{
			ID:      navigatorRequestID(m.currentFile, 0),
			Kind:    navigator.KindRequest,
			Payload: navigator.Payload[any]{FilePath: m.currentFile, Data: req},
		},
	})
	m.focus = focusRequests

	if res := m.setCollapseState(paneRegionEditor, true); res.blocked {
		t.Fatalf("expected editor collapse to be allowed")
	}

	m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyRight})

	if !m.collapseState(paneRegionEditor) {
		t.Fatalf("expected right key not to restore editor for request jump")
	}
	if m.focus == focusEditor {
		t.Fatalf("expected right key not to move focus to editor")
	}
	if got := currentCursorLine(m.editor); got == req.LineRange.Start {
		t.Fatalf("expected right key not to jump cursor to request line %d", got)
	}
}

func TestNavigatorRightFallsThroughWhenRequestJumpCannotResolve(t *testing.T) {
	model := newTestModelWithDoc("### req\nGET https://example.com\n")
	m := model
	m.navigator = navigator.New[any]([]*navigator.Node[any]{
		{
			ID:    "req:/tmp/sample:0",
			Kind:  navigator.KindRequest,
			Title: "Broken request",
			Children: []*navigator.Node[any]{
				{
					ID:    "req:/tmp/sample:0:child",
					Kind:  navigator.KindRequest,
					Title: "Child",
				},
			},
		},
	})

	if cmd := m.updateNavigator(tea.KeyMsg{Type: tea.KeyRight}); cmd != nil {
		cmd()
	}

	n := m.navigator.Find("req:/tmp/sample:0")
	if n == nil || !n.Expanded {
		t.Fatalf("expected right key to expand unresolved request node")
	}
}

func TestNavigatorLDoesNotExpandWhenRequestJumpCannotResolve(t *testing.T) {
	model := newTestModelWithDoc("### req\nGET https://example.com\n")
	m := model
	m.navigator = navigator.New[any]([]*navigator.Node[any]{
		{
			ID:    "req:/tmp/sample:0",
			Kind:  navigator.KindRequest,
			Title: "Broken request",
			Children: []*navigator.Node[any]{
				{
					ID:    "req:/tmp/sample:0:child",
					Kind:  navigator.KindRequest,
					Title: "Child",
				},
			},
		},
	})

	if cmd := m.updateNavigator(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}}); cmd != nil {
		cmd()
	}

	n := m.navigator.Find("req:/tmp/sample:0")
	if n == nil || n.Expanded {
		t.Fatalf("expected l key not to expand unresolved request node")
	}
}

func TestNavigatorWorkflowLJumpsToDefinition(t *testing.T) {
	content := strings.Repeat("\n", 5) + `# @workflow sample-order on-failure=continue
# @step Cleanup using=DeleteResource

### Cleanup resource
# @name DeleteResource
DELETE https://example.com/resource
`
	model := newTestModelWithDoc(content)
	m := model
	m.currentFile = "/tmp/workflow.http"
	m.cfg.FilePath = m.currentFile
	m.syncRequestList(m.doc)

	if len(m.doc.Workflows) == 0 {
		t.Fatalf("expected parsed workflows in doc")
	}
	wf := &m.doc.Workflows[0]
	if wf.LineRange.Start <= 1 {
		t.Fatalf("expected workflow to start after line 1, got %d", wf.LineRange.Start)
	}
	if m.activeRequestKey == "" {
		t.Fatalf("expected request list sync to select the request before workflow jump")
	}
	activeReq := m.currentRequest
	activeReqKey := m.activeRequestKey

	m.navigator = navigator.New[any]([]*navigator.Node[any]{
		{
			ID:      "wf:" + m.currentFile + ":0",
			Kind:    navigator.KindWorkflow,
			Payload: navigator.Payload[any]{FilePath: m.currentFile, Data: wf},
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
	if got := currentCursorLine(m.editor); got != wf.LineRange.Start {
		t.Fatalf("expected cursor to jump to line %d, got %d", wf.LineRange.Start, got)
	}
	if m.currentRequest != activeReq || m.activeRequestKey != activeReqKey {
		t.Fatalf(
			"expected workflow jump to preserve active request %q, got %q",
			activeReqKey,
			m.activeRequestKey,
		)
	}
	if m.workflowSelectionKey != workflowKey(wf) {
		t.Fatalf("expected workflow selection key %q, got %q", workflowKey(wf), m.workflowSelectionKey)
	}
}

func TestNavigatorWorkflowJumpFullUpdateKeepsWorkflowSelected(t *testing.T) {
	content := `### Fetch
# @name FetchExample
GET https://example.com

# @workflow sample-order
# @step Fetch using=FetchExample
`
	model := newTestModelWithDoc(content)
	m := model
	m.currentFile = "/tmp/workflow-after-request.http"
	m.cfg.FilePath = m.currentFile
	m.syncRequestList(m.doc)

	if len(m.doc.Requests) == 0 || len(m.doc.Workflows) == 0 {
		t.Fatalf("expected request and workflow")
	}
	wf := &m.doc.Workflows[0]
	wfID := "wf:" + m.currentFile + ":0"
	m.navigator = navigator.New[any]([]*navigator.Node[any]{
		{
			ID:      navigatorRequestID(m.currentFile, 0),
			Kind:    navigator.KindRequest,
			Payload: navigator.Payload[any]{FilePath: m.currentFile, Data: m.doc.Requests[0]},
		},
		{
			ID:      wfID,
			Kind:    navigator.KindWorkflow,
			Payload: navigator.Payload[any]{FilePath: m.currentFile, Data: wf},
		},
	})
	selectNavigatorID(t, m, wfID)
	m.focus = focusWorkflows

	m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	if m.focus != focusEditor {
		t.Fatalf("expected focus to move to editor, got %v", m.focus)
	}
	if got := currentCursorLine(m.editor); got != wf.LineRange.Start {
		t.Fatalf("expected cursor to jump to workflow line %d, got %d", wf.LineRange.Start, got)
	}
	if sel := m.navigator.Selected(); sel == nil || sel.ID != wfID {
		t.Fatalf("expected workflow selection to survive full update, got %#v", sel)
	}
	if m.suppressEditorKey {
		t.Fatalf("expected editor key suppression to be consumed after full update")
	}
}

func TestNavigatorWorkflowJumpSurvivesNextEditorSync(t *testing.T) {
	content := `### Fetch
# @name FetchExample
GET https://example.com

# @workflow sample-order
# @step Fetch using=FetchExample
`
	model := newTestModelWithDoc(content)
	m := model
	m.currentFile = "/tmp/workflow-stable.http"
	m.cfg.FilePath = m.currentFile
	m.syncRequestList(m.doc)

	wf := &m.doc.Workflows[0]
	wfID := navigatorWorkflowID(m.currentFile, 0)
	activeReq := m.currentRequest
	activeReqKey := m.activeRequestKey
	m.navigator = navigator.New[any]([]*navigator.Node[any]{
		{
			ID:      navigatorRequestID(m.currentFile, 0),
			Kind:    navigator.KindRequest,
			Payload: navigator.Payload[any]{FilePath: m.currentFile, Data: m.doc.Requests[0]},
		},
		{
			ID:      wfID,
			Kind:    navigator.KindWorkflow,
			Payload: navigator.Payload[any]{FilePath: m.currentFile, Data: wf},
		},
	})
	selectNavigatorID(t, m, wfID)
	m.focus = focusWorkflows

	m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if got := currentCursorLine(m.editor); got != wf.LineRange.Start {
		t.Fatalf("expected cursor to jump to workflow line %d, got %d", wf.LineRange.Start, got)
	}
	if sel := m.navigator.Selected(); sel == nil || sel.ID != wfID {
		t.Fatalf("expected workflow selected after jump, got %#v", sel)
	}

	m = applyModelUpdate(t, m, tea.WindowSizeMsg{Width: 100, Height: 32})
	if got := currentCursorLine(m.editor); got != wf.LineRange.Start {
		t.Fatalf("expected cursor to remain on workflow line %d, got %d", wf.LineRange.Start, got)
	}
	if sel := m.navigator.Selected(); sel == nil || sel.ID != wfID {
		t.Fatalf("expected workflow selection to survive next editor sync, got %#v", sel)
	}
	if m.currentRequest != activeReq || m.activeRequestKey != activeReqKey {
		t.Fatalf(
			"expected active request %q to survive workflow cursor sync, got %q",
			activeReqKey,
			m.activeRequestKey,
		)
	}
}

func TestNavigatorCrossFileJumpConfirmationIsNotReusedForWorkflowRun(t *testing.T) {
	tmp := t.TempDir()
	fileA := filepath.Join(tmp, "current.http")
	fileB := filepath.Join(tmp, "workflow.http")
	writeSampleFile(t, fileA, "### Local\n# @name Local\nGET https://local.test\n")
	writeSampleFile(t, fileB, `# @workflow cross-file
# @step Fetch using=FetchRemote

### Fetch remote
# @name FetchRemote
GET https://remote.test
`)

	model := New(Config{WorkspaceRoot: tmp, FilePath: fileA})
	m := &model
	if cmd := m.openFile(fileA); cmd != nil {
		cmd()
	}
	m.expandNavigatorFile(fileB)
	wfID := "wf:" + fileB + ":0"
	selectNavigatorID(t, m, wfID)
	m.focus = focusWorkflows
	m.markDirty()

	m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if !util.SamePath(m.currentFile, fileA) {
		t.Fatalf("expected first jump press to keep current file %q, got %q", fileA, m.currentFile)
	}
	if !strings.Contains(m.statusMessage.text, "Press l again to jump.") {
		t.Fatalf("expected jump-specific warning, got %q", m.statusMessage.text)
	}

	m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !util.SamePath(m.currentFile, fileA) {
		t.Fatalf("expected enter not to reuse jump confirmation, got current file %q", m.currentFile)
	}
	if m.workflowRun != nil {
		t.Fatalf("expected workflow not to start from stale jump confirmation")
	}
	if !strings.Contains(m.statusMessage.text, "Press Enter/Space again to run.") {
		t.Fatalf("expected run-specific warning, got %q", m.statusMessage.text)
	}
}

func TestNavigatorCrossFilePreviewConfirmationIsNotReusedForRequestSend(t *testing.T) {
	tmp := t.TempDir()
	fileA := filepath.Join(tmp, "current.http")
	fileB := filepath.Join(tmp, "remote.http")
	writeSampleFile(t, fileA, "### Local\n# @name Local\nGET https://local.test\n")
	writeSampleFile(t, fileB, "### Remote\n# @name Remote\nGET https://remote.test\n")

	model := New(Config{WorkspaceRoot: tmp, FilePath: fileA})
	m := &model
	if cmd := m.openFile(fileA); cmd != nil {
		cmd()
	}
	m.expandNavigatorFile(fileB)
	reqID := navigatorRequestID(fileB, 0)
	selectNavigatorID(t, m, reqID)
	m.focus = focusRequests
	m.markDirty()

	m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeySpace})
	if !util.SamePath(m.currentFile, fileA) {
		t.Fatalf("expected first preview press to keep current file %q, got %q", fileA, m.currentFile)
	}
	if !strings.Contains(m.statusMessage.text, "Press Space again to preview.") {
		t.Fatalf("expected preview-specific warning, got %q", m.statusMessage.text)
	}

	m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !util.SamePath(m.currentFile, fileA) {
		t.Fatalf("expected enter not to reuse preview confirmation, got current file %q", m.currentFile)
	}
	if !strings.Contains(m.statusMessage.text, "Press Enter again to send.") {
		t.Fatalf("expected send-specific warning, got %q", m.statusMessage.text)
	}
}

func TestNavigatorCrossFileConfirmationIsBoundToSourceBuffer(t *testing.T) {
	tmp := t.TempDir()
	fileA := filepath.Join(tmp, "current.http")
	fileB := filepath.Join(tmp, "remote.http")
	fileC := filepath.Join(tmp, "other.http")
	writeSampleFile(t, fileA, "### Local\n# @name Local\nGET https://local.test\n")
	writeSampleFile(t, fileB, "### Remote\n# @name Remote\nGET https://remote.test\n")
	writeSampleFile(t, fileC, "### Other\n# @name Other\nGET https://other.test\n")

	model := New(Config{WorkspaceRoot: tmp, FilePath: fileA})
	m := &model
	if cmd := m.openFile(fileA); cmd != nil {
		cmd()
	}
	m.expandNavigatorFile(fileB)
	reqID := navigatorRequestID(fileB, 0)
	selectNavigatorID(t, m, reqID)
	m.focus = focusRequests
	m.markDirty()

	m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeySpace})
	if !util.SamePath(m.currentFile, fileA) {
		t.Fatalf("expected first preview press to keep current file %q, got %q", fileA, m.currentFile)
	}
	if !strings.Contains(m.statusMessage.text, "Press Space again to preview.") {
		t.Fatalf("expected preview-specific warning, got %q", m.statusMessage.text)
	}

	if cmd := m.openFile(fileC); cmd != nil {
		cmd()
	}
	m.expandNavigatorFile(fileB)
	selectNavigatorID(t, m, reqID)
	m.focus = focusRequests
	m.markDirty()

	m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeySpace})
	if !util.SamePath(m.currentFile, fileC) {
		t.Fatalf("expected stale confirmation not to open %q, got current file %q", fileB, m.currentFile)
	}
	if !strings.Contains(m.statusMessage.text, "Press Space again to preview.") {
		t.Fatalf("expected a fresh preview warning for new source buffer, got %q", m.statusMessage.text)
	}
}

func TestNavigatorCrossFileConfirmationIsInvalidatedByFurtherEdits(t *testing.T) {
	tmp := t.TempDir()
	fileA := filepath.Join(tmp, "current.http")
	fileB := filepath.Join(tmp, "remote.http")
	writeSampleFile(t, fileA, "### Local\n# @name Local\nGET https://local.test\n")
	writeSampleFile(t, fileB, "### Remote\n# @name Remote\nGET https://remote.test\n")

	model := New(Config{WorkspaceRoot: tmp, FilePath: fileA})
	m := &model
	if cmd := m.openFile(fileA); cmd != nil {
		cmd()
	}
	m.expandNavigatorFile(fileB)
	reqID := navigatorRequestID(fileB, 0)
	selectNavigatorID(t, m, reqID)
	m.focus = focusRequests
	m.markDirty()

	m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeySpace})
	if !util.SamePath(m.currentFile, fileA) {
		t.Fatalf("expected first preview press to keep current file %q, got %q", fileA, m.currentFile)
	}

	original := m.editor.Value()
	m.editor.SetValue(original + "\n# changed after warning")
	m.editor.SetValue(original)
	m.markDirty()
	m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeySpace})
	if !util.SamePath(m.currentFile, fileA) {
		t.Fatalf("expected edited-then-restored buffer to require fresh confirmation, got current file %q", m.currentFile)
	}
	if !strings.Contains(m.statusMessage.text, "Press Space again to preview.") {
		t.Fatalf("expected fresh preview warning after restoring an edit, got %q", m.statusMessage.text)
	}
}

func TestNavigatorFileOpenRequiresDirtyConfirmation(t *testing.T) {
	tmp := t.TempDir()
	fileA := filepath.Join(tmp, "current.http")
	fileB := filepath.Join(tmp, "remote.http")
	writeSampleFile(t, fileA, "### Local\n# @name Local\nGET https://local.test\n")
	writeSampleFile(t, fileB, "### Remote\n# @name Remote\nGET https://remote.test\n")

	model := New(Config{WorkspaceRoot: tmp, FilePath: fileA})
	m := &model
	if cmd := m.openFile(fileA); cmd != nil {
		cmd()
	}
	selectNavigatorID(t, m, "file:"+fileB)
	m.focus = focusFile
	m.markDirty()

	m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !util.SamePath(m.currentFile, fileA) {
		t.Fatalf("expected first enter to keep dirty file %q, got %q", fileA, m.currentFile)
	}
	if !strings.Contains(m.statusMessage.text, "Press Enter again to open.") {
		t.Fatalf("expected file-open warning, got %q", m.statusMessage.text)
	}

	m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !util.SamePath(m.currentFile, fileB) {
		t.Fatalf("expected repeated enter to open %q, got %q", fileB, m.currentFile)
	}
	if m.dirty {
		t.Fatalf("expected opened file to be clean")
	}
}

func TestNavigatorWorkflowJumpUsesNodeIndexBeforeWorkflowName(t *testing.T) {
	content := `# @workflow duplicate
# @step First using=req

# @workflow duplicate
# @step Second using=req

### req
GET https://example.com
`
	model := newTestModelWithDoc(content)
	m := model
	m.currentFile = "/tmp/duplicate-workflow.http"
	m.cfg.FilePath = m.currentFile
	m.syncRequestList(m.doc)

	if len(m.doc.Workflows) != 2 {
		t.Fatalf("expected two workflows, got %d", len(m.doc.Workflows))
	}
	first := &m.doc.Workflows[0]
	second := &m.doc.Workflows[1]
	if workflowKey(first) != workflowKey(second) {
		t.Fatalf("expected duplicate workflow keys")
	}

	secondID := "wf:" + m.currentFile + ":1"
	m.navigator = navigator.New[any]([]*navigator.Node[any]{
		{
			ID:      "wf:" + m.currentFile + ":0",
			Kind:    navigator.KindWorkflow,
			Payload: navigator.Payload[any]{FilePath: m.currentFile, Data: first},
		},
		{
			ID:      secondID,
			Kind:    navigator.KindWorkflow,
			Payload: navigator.Payload[any]{FilePath: m.currentFile, Data: second},
		},
	})
	selectNavigatorID(t, m, secondID)

	if cmd := m.updateNavigator(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}}); cmd != nil {
		cmd()
	}

	if got := currentCursorLine(m.editor); got != second.LineRange.Start {
		t.Fatalf("expected cursor to jump to duplicate workflow line %d, got %d", second.LineRange.Start, got)
	}
	if got := m.workflowList.Index(); got != 1 {
		t.Fatalf("expected second workflow list item to be selected, got %d", got)
	}
}

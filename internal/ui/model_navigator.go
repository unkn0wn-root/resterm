package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/ui/navigator"
)

func (m *Model) rebuildNavigator(entries []filesvc.FileEntry) {
	m.cacheDoc(m.currentFile, m.doc)
	if len(entries) == 0 {
		entries = m.entriesFromList()
	}

	var prevNav *navigator.Model[any]
	if m.navigator != nil {
		prevNav = m.navigator
	}

	nodes := make([]*navigator.Node[any], 0, len(entries))
	for _, entry := range entries {
		nodes = append(nodes, m.buildFileNode(entry))
	}
	if prevNav != nil {
		applyNavigatorExpansion(nodes, prevNav)
	}
	if m.navigator == nil {
		m.navigator = navigator.New[any](nodes)
	} else {
		m.navigator.SetNodes(nodes)
	}
	m.navigator.SetCompact(m.navigatorCompact)
}

func navigatorRequestID(path string, idx int) string {
	return fmt.Sprintf("req:%s:%d", path, idx)
}

func (m *Model) buildFileNode(entry filesvc.FileEntry) *navigator.Node[any] {
	id := "file:" + entry.Path
	node := &navigator.Node[any]{
		ID:      id,
		Title:   entry.Name,
		Kind:    navigator.KindFile,
		Payload: navigator.Payload[any]{FilePath: entry.Path, Data: entry},
	}

	if doc, ok := m.cachedDoc(entry.Path); ok && doc != nil {
		node.Count = len(doc.Requests)
		node.Children = m.buildRequestNodes(doc, entry.Path)
		if filepath.Clean(entry.Path) == filepath.Clean(m.currentFile) {
			node.Expanded = true
		}
	}
	return node
}

func (m *Model) buildRequestNodes(doc *restfile.Document, filePath string) []*navigator.Node[any] {
	if doc == nil || len(doc.Requests) == 0 {
		return nil
	}
	nodes := make([]*navigator.Node[any], 0, len(doc.Requests)+len(doc.Workflows))
	for idx, req := range doc.Requests {
		resolver := m.statusResolver(doc, req, m.cfg.EnvironmentName)
		title := requestNavLabel(req, resolver, fmt.Sprintf("Request %d", idx+1))
		method := strings.ToUpper(strings.TrimSpace(req.Method))
		desc := condense(expandStatusText(resolver, req.Metadata.Description), 80)
		target := expandStatusText(resolver, requestTarget(req))
		hasName := strings.TrimSpace(req.Metadata.Name) != ""
		if !hasName && strings.TrimSpace(title) == strings.TrimSpace(target) {
			target = ""
		}
		badges := requestBadges(req)
		nodes = append(nodes, &navigator.Node[any]{
			ID:      navigatorRequestID(filePath, idx),
			Title:   title,
			Desc:    desc,
			Kind:    navigator.KindRequest,
			Method:  method,
			Tags:    req.Metadata.Tags,
			Target:  target,
			Badges:  badges,
			HasName: hasName,
			Payload: navigator.Payload[any]{FilePath: filePath, Data: req},
		})
	}
	for idx := range doc.Workflows {
		wf := &doc.Workflows[idx]
		title := strings.TrimSpace(wf.Name)
		if title == "" {
			title = fmt.Sprintf("Workflow %d", idx+1)
		}
		desc := condense(strings.TrimSpace(wf.Description), 80)
		badges := []string{fmt.Sprintf("%d steps", len(wf.Steps))}
		nodes = append(nodes, &navigator.Node[any]{
			ID:      fmt.Sprintf("wf:%s:%d", filePath, idx),
			Title:   title,
			Desc:    desc,
			Kind:    navigator.KindWorkflow,
			Tags:    wf.Tags,
			Badges:  badges,
			Payload: navigator.Payload[any]{FilePath: filePath, Data: wf},
		})
	}
	return nodes
}

func (m *Model) cachedDoc(path string) (*restfile.Document, bool) {
	if path == "" {
		return nil, false
	}
	if path == m.currentFile && m.doc != nil {
		return m.doc, true
	}

	entry, ok := m.docCache[path]
	if !ok || entry.doc == nil {
		return nil, false
	}

	info, err := os.Stat(path)
	if err != nil || info.ModTime().After(entry.mod) {
		return nil, false
	}
	return entry.doc, true
}

func (m *Model) cacheDoc(path string, doc *restfile.Document) {
	if path == "" || doc == nil {
		return
	}

	info, err := os.Stat(path)
	mod := time.Time{}
	if err == nil {
		mod = info.ModTime()
	}
	if m.docCache == nil {
		m.docCache = make(map[string]navDocCache)
	}
	m.docCache[path] = navDocCache{doc: doc, mod: mod}
}

func (m *Model) ensureNavigatorFilter() {
	if m.navigatorFilter.Prompt == "" {
		ni := textinput.New()
		ni.Placeholder = "Requests, tags, files..."
		ni.Prompt = "Filter: "
		ni.CharLimit = 0
		ni.SetCursor(0)
		ni.TextStyle = m.theme.NavigatorTitle
		ni.PromptStyle = m.theme.NavigatorTitle
		ni.PlaceholderStyle = m.theme.NavigatorSubtitle
		ni.Cursor.Style = m.theme.NavigatorTitle
		ni.Blur()
		m.navigatorFilter = ni
	}
}

func (m *Model) loadDocFor(path string) *restfile.Document {
	if path == "" {
		return nil
	}
	if path == m.currentFile && m.doc != nil {
		return m.doc
	}
	if doc, ok := m.cachedDoc(path); ok {
		return doc
	}
	data, err := os.ReadFile(path)
	if err != nil {
		m.setStatusMessage(statusMsg{text: fmt.Sprintf("open failed: %v", err), level: statusError})
		return nil
	}
	doc := parser.Parse(path, data)
	m.cacheDoc(path, doc)
	return doc
}

func (m *Model) expandNavigatorFile(path string) {
	if m.navigator == nil {
		return
	}

	doc := m.loadDocFor(path)
	if doc == nil {
		return
	}

	children := m.buildRequestNodes(doc, path)
	m.navigator.ReplaceChildren("file:"+path, children)
	node := m.navigator.Find("file:" + path)
	if node != nil && len(children) > 0 {
		node.Count = len(children)
		node.Expanded = true
	}
}

func (m *Model) ensureNavigatorRequestsForFile(path string) {
	if m.navigator == nil || path == "" {
		return
	}
	node := m.navigator.Find("file:" + path)
	if node == nil {
		return
	}
	if len(node.Children) == 0 {
		m.expandNavigatorFile(path)
		node = m.navigator.Find("file:" + path)
	}
	if node != nil && !node.Expanded {
		node.Expanded = true
		m.navigator.Refresh()
	}
}

func (m *Model) ensureNavigatorDataForFilter() {
	if m.navigator == nil {
		return
	}
	filter := strings.TrimSpace(m.navigatorFilter.Value())
	need := filter != "" || len(m.navigator.MethodFilters()) > 0 || len(m.navigator.TagFilters()) > 0
	if !need {
		return
	}
	for _, entry := range m.entriesFromList() {
		path := entry.Path
		if path == "" {
			continue
		}
		node := m.navigator.Find("file:" + path)
		if node == nil || len(node.Children) > 0 {
			continue
		}
		m.expandNavigatorFile(path)
	}
}

func (m *Model) entriesFromList() []filesvc.FileEntry {
	items := m.fileList.Items()
	out := make([]filesvc.FileEntry, 0, len(items))
	for _, it := range items {
		if fi, ok := it.(fileItem); ok {
			out = append(out, fi.entry)
		}
	}
	return out
}

func requestBadges(req *restfile.Request) []string {
	if req == nil {
		return nil
	}
	var b []string
	switch {
	case req.WebSocket != nil:
		b = append(b, "WS")
	case req.SSE != nil:
		b = append(b, "SSE")
	case req.GRPC != nil:
		b = append(b, "gRPC")
	default:
	}
	if req.Metadata.Compare != nil {
		b = append(b, "CMP")
	}
	if req.Metadata.Auth != nil {
		b = append(b, "AUTH")
	}
	if len(req.Metadata.Scripts) > 0 {
		b = append(b, "SCRIPT")
	}
	return b
}

func (m *Model) syncNavigatorSelection() {
	if m.navigator == nil {
		return
	}
	n := m.navigator.Selected()
	m.syncNavigatorFocus(n)
	if n == nil {
		m.setActiveRequest(nil)
		m.requestList.Select(-1)
		m.workflowList.Select(-1)
		return
	}
	path := n.Payload.FilePath
	switch n.Kind {
	case navigator.KindRequest:
		if req, ok := n.Payload.Data.(*restfile.Request); ok {
			if path != "" {
				_ = m.selectFileByPath(path)
			}
			if samePath(path, m.currentFile) {
				m.setActiveRequest(req)
			} else {
				if m.pendingCrossFileID == n.ID {
					return
				}
				m.setActiveRequest(nil)
				m.requestList.Select(-1)
				m.setStatusMessage(statusMsg{text: "Open file to edit this request", level: statusInfo})
			}
		} else {
			m.setActiveRequest(nil)
			m.requestList.Select(-1)
		}
	case navigator.KindWorkflow:
		if wf, ok := n.Payload.Data.(*restfile.Workflow); ok {
			if path != "" {
				_ = m.selectFileByPath(path)
			}
			if samePath(path, m.currentFile) {
				m.activeWorkflowKey = workflowKey(wf)
				_ = m.selectWorkflowItemByKey(m.activeWorkflowKey)
			} else {
				if m.pendingCrossFileID == n.ID {
					return
				}
				m.activeWorkflowKey = ""
				m.workflowList.Select(-1)
				m.setStatusMessage(statusMsg{text: "Open file to edit this workflow", level: statusInfo})
			}
		} else {
			m.activeWorkflowKey = ""
			m.workflowList.Select(-1)
		}
	case navigator.KindFile:
		if path != "" {
			_ = m.selectFileByPath(path)
		}
		m.setActiveRequest(nil)
		m.requestList.Select(-1)
	default:
		m.setActiveRequest(nil)
		m.requestList.Select(-1)
	}
}

func (m *Model) syncNavigatorFocus(n *navigator.Node[any]) {
	if n == nil {
		return
	}
	switch n.Kind {
	case navigator.KindRequest:
		_ = m.setFocus(focusRequests)
	case navigator.KindWorkflow:
		_ = m.setFocus(focusWorkflows)
	case navigator.KindFile:
		_ = m.setFocus(focusFile)
	}
}

func (m *Model) resetCursorSync() {
	m.lastCursorLine = -1
	m.lastCursorFile = ""
	m.lastCursorDoc = nil
}

func (m *Model) syncNavigatorWithEditorCursor() {
	if m.navigator == nil || m.doc == nil || m.currentFile == "" {
		return
	}
	if m.focus != focusEditor {
		return
	}

	line := currentCursorLine(m.editor)
	req, reqIdx := requestAtLine(m.doc, line)

	if req == nil {
		m.lastCursorLine = line
		m.lastCursorFile = m.currentFile
		m.lastCursorDoc = m.doc
		return
	}

	targetID := navigatorRequestID(m.currentFile, reqIdx)
	currentID := ""
	if sel := m.navigator.Selected(); sel != nil {
		currentID = sel.ID
	}
	// Doc pointer comparison intentionally detects reparses as a change.
	if line == m.lastCursorLine &&
		m.lastCursorFile == m.currentFile &&
		m.lastCursorDoc == m.doc &&
		currentID == targetID {
		return
	}

	m.ensureNavigatorRequestsForFile(m.currentFile)
	if !m.navigator.SelectByID(targetID) {
		m.lastCursorLine = line
		m.lastCursorFile = m.currentFile
		m.lastCursorDoc = m.doc
		return
	}

	m.setActiveRequest(req)
	m.lastCursorLine = line
	m.lastCursorFile = m.currentFile
	m.lastCursorDoc = m.doc
}

// applyNavigatorExpansion copies expanded state from the previous navigator tree.
func applyNavigatorExpansion(nodes []*navigator.Node[any], prev *navigator.Model[any]) {
	if prev == nil || len(nodes) == 0 {
		return
	}
	var walk func(n *navigator.Node[any])
	walk = func(n *navigator.Node[any]) {
		if n == nil {
			return
		}
		if old := prev.Find(n.ID); old != nil && old.Expanded {
			n.Expanded = true
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, n := range nodes {
		walk(n)
	}
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

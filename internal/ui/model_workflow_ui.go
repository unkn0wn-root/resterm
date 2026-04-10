package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (m *Model) syncWorkflowList(doc *restfile.Document) bool {
	items, listItems := buildWorkflowItems(doc)
	m.workflowItems = items
	visible := len(listItems) > 0
	if !visible {
		m.workflowList.SetItems(nil)
		m.workflowList.Select(-1)
		m.activeWorkflowKey = ""
		m.setHistoryWorkflow("")
		changed := m.setWorkflowShown(false)
		if m.focus == focusWorkflows {
			m.resetWorkflowFocus(doc)
		}
		return changed
	}
	m.workflowList.SetItems(listItems)
	if !m.selectWorkflowItemByKey(m.activeWorkflowKey) {
		m.workflowList.Select(0)
		if len(m.workflowItems) > 0 {
			m.activeWorkflowKey = workflowKey(m.workflowItems[0].workflow)
		}
	}
	return m.setWorkflowShown(true)
}

func (m *Model) setWorkflowShown(visible bool) bool {
	if m.showWorkflow == visible {
		return false
	}
	m.showWorkflow = visible
	return true
}

func (m *Model) resetWorkflowFocus(doc *restfile.Document) {
	if doc != nil && len(doc.Requests) > 0 {
		_ = m.setFocus(focusRequests)
		return
	}
	if len(m.fileList.Items()) > 0 {
		_ = m.setFocus(focusFile)
		return
	}
	_ = m.setFocus(focusEditor)
}

func (m *Model) selectWorkflowItemByKey(key string) bool {
	if key == "" {
		return false
	}
	for i, item := range m.workflowItems {
		if workflowKey(item.workflow) == key {
			m.workflowList.Select(i)
			return true
		}
	}
	return false
}

func (m *Model) runSelectedWorkflow() tea.Cmd {
	if m.doc == nil {
		m.setStatusMessage(statusMsg{text: "No document loaded", level: statusWarn})
		return nil
	}
	item, ok := m.workflowList.SelectedItem().(workflowListItem)
	if !ok || item.workflow == nil {
		m.setStatusMessage(statusMsg{text: "No workflow selected", level: statusWarn})
		return nil
	}

	wf := *item.workflow
	m.setHistoryWorkflow(wf.Name)
	if key := workflowKey(item.workflow); key != "" {
		m.activeWorkflowKey = key
	}
	return m.startWorkflowRun(m.doc, wf, m.cfg.HTTPOptions)
}

func (m *Model) setHistoryWorkflow(name string) {
	trimmed := history.NormalizeWorkflowName(name)
	if trimmed == "" {
		if m.historyWorkflowName == "" && m.historyScope != historyScopeWorkflow {
			return
		}
		m.historyWorkflowName = ""
		if m.historyScope == historyScopeWorkflow {
			m.historyScope = historyScopeRequest
		}
		if m.ready {
			m.syncHistory()
		}
		return
	}
	if m.historyWorkflowName == trimmed && m.historyScope == historyScopeWorkflow {
		return
	}
	m.historyWorkflowName = trimmed
	m.historyScope = historyScopeWorkflow
	if m.ready {
		m.syncHistory()
	}
}

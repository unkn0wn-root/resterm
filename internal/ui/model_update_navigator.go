package ui

import (
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/ui/navigator"
)

func navigatorFilterConsumesKey(msg tea.KeyMsg) bool {
	if isPlainRuneKey(msg) || isSpaceKey(msg) {
		return true
	}
	switch msg.Type {
	case tea.KeyLeft, tea.KeyRight, tea.KeyBackspace, tea.KeyDelete:
		return true
	default:
		return false
	}
}

func (m *Model) updateNavigator(msg tea.Msg) tea.Cmd {
	if m.navigator == nil {
		return nil
	}
	m.ensureNavigatorFilter()

	applyFilter := func(cmd tea.Cmd) tea.Cmd {
		var filterCmd tea.Cmd
		if m.navigatorFilter.Focused() {
			m.navigatorFilter, filterCmd = m.navigatorFilter.Update(msg)
		}
		m.navigator.SetFilter(m.navigatorFilter.Value())
		m.ensureNavigatorDataForFilter()
		m.syncNavigatorSelection()
		switch {
		case cmd != nil && filterCmd != nil:
			return tea.Batch(cmd, filterCmd)
		case cmd != nil:
			return cmd
		default:
			return filterCmd
		}
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok && m.navigatorFilter.Focused() && navigatorFilterConsumesKey(keyMsg) {
		return applyFilter(nil)
	}

	var cmd tea.Cmd
	switch ev := msg.(type) {
	case tea.KeyMsg:
		switch ev.String() {
		case "/":
			m.navigatorFilter.Focus()
			m.resetChordState()
			return nil
		case "esc":
			wasFocused := m.navigatorFilter.Focused()
			hasFilter := strings.TrimSpace(m.navigatorFilter.Value()) != ""
			hasMethod := len(m.navigator.MethodFilters()) > 0
			hasTags := len(m.navigator.TagFilters()) > 0
			if wasFocused || hasFilter || hasMethod || hasTags {
				m.navigatorFilter.SetValue("")
				m.navigator.ClearMethodFilters()
				m.navigator.ClearTagFilters()
				m.navigator.SetFilter("")
				m.navigatorFilter.Blur()
				m.syncNavigatorSelection()
				return nil
			}
		case "down", "j":
			m.navigator.Move(1)
			m.syncNavigatorSelection()
		case "up", "k":
			m.navigator.Move(-1)
			m.syncNavigatorSelection()
		case "right", "l":
			if m.navigatorFilter.Focused() {
				m.navigatorFilter.Blur()
				return nil
			}
			n := m.navigator.Selected()
			if n != nil && n.Kind == navigator.KindFile {
				path := n.Payload.FilePath
				if path != "" && filepath.Clean(path) != filepath.Clean(m.currentFile) {
					cmd = m.openFile(path)
				}
				if len(n.Children) == 0 {
					m.expandNavigatorFile(path)
				}
				if refreshed := m.navigator.Find("file:" + path); refreshed != nil {
					n = refreshed
				}
				if n != nil && len(n.Children) > 0 && !n.Expanded {
					n.Expanded = true
					m.navigator.Refresh()
				}
			} else if n != nil && (n.Kind == navigator.KindRequest || n.Kind == navigator.KindWorkflow) {
				// Jump to editor when right/l is pressed on a request or workflow
				m.setFocus(focusEditor)
				return nil
			} else {
				m.navigator.ToggleExpanded()
			}
		case "enter":
			if m.navigatorFilter.Focused() {
				m.navigatorFilter.Blur()
				return nil
			}
			n := m.navigator.Selected()
			if n == nil {
				return nil
			}
			if n.Kind == navigator.KindFile {
				path := n.Payload.FilePath
				if path != "" && filepath.Clean(path) != filepath.Clean(m.currentFile) {
					cmd = m.openFile(path)
				}
				if len(n.Children) == 0 {
					m.expandNavigatorFile(path)
				}
				if refreshed := m.navigator.Find("file:" + path); refreshed != nil {
					n = refreshed
				}
				if n != nil && len(n.Children) > 0 && !n.Expanded {
					n.Expanded = true
					m.navigator.Refresh()
				}
			} else if n.Kind == navigator.KindRequest || n.Kind == navigator.KindWorkflow {
				// Jump to editor when enter is pressed on a request or workflow
				m.setFocus(focusEditor)
				return nil
			}
		case " ":
			n := m.navigator.Selected()
			if n == nil || n.Kind != navigator.KindFile {
				return nil
			}
			path := n.Payload.FilePath
			hasChildren := len(n.Children) > 0
			if !hasChildren {
				m.expandNavigatorFile(path)
				if refreshed := m.navigator.Find("file:" + path); refreshed != nil {
					n = refreshed
				}
			}
			if n != nil && len(n.Children) > 0 {
				if hasChildren {
					n.Expanded = !n.Expanded
				} else {
					n.Expanded = true
				}
				m.navigator.Refresh()
			}
		case "left", "h":
			n := m.navigator.Selected()
			if n != nil && n.Expanded {
				m.navigator.ToggleExpanded()
			}
		case "m":
			if n := m.navigator.Selected(); n != nil && n.Method != "" {
				m.navigator.ToggleMethodFilter(n.Method)
				m.syncNavigatorSelection()
			} else {
				m.navigator.ClearMethodFilters()
			}
		case "t":
			if n := m.navigator.Selected(); n != nil && len(n.Tags) > 0 {
				for _, tag := range n.Tags {
					m.navigator.ToggleTagFilter(tag)
				}
				m.syncNavigatorSelection()
			} else {
				m.navigator.ClearTagFilters()
			}
		case "r":
			req, _, cmds, ok := m.prepareNavigatorRequest()
			if !ok {
				if len(cmds) == 0 {
					return applyFilter(nil)
				}
				return applyFilter(tea.Batch(cmds...))
			}
			m.jumpToNavigatorRequest(req, true)
			m.setFocus(focusEditor)
			if len(cmds) > 0 {
				return applyFilter(tea.Batch(cmds...))
			}
			return applyFilter(nil)
		}
	}

	return applyFilter(cmd)
}

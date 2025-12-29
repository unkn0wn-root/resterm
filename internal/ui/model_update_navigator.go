package ui

import (
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
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

type navJumpResult struct {
	cmd   tea.Cmd
	ok    bool
	focus bool
}

func (m *Model) navReqJumpCmd() navJumpResult {
	if m.navigator == nil {
		return navJumpResult{}
	}
	n := m.navigator.Selected()
	if n == nil || n.Kind != navigator.KindRequest {
		return navJumpResult{}
	}
	req, _, cmds, ok := m.prepareNavigatorRequest()
	if !ok {
		if len(cmds) > 0 {
			return navJumpResult{cmd: tea.Batch(cmds...), ok: true}
		}
		return navJumpResult{ok: true}
	}
	m.jumpToNavigatorRequest(req, true)
	if len(cmds) > 0 {
		return navJumpResult{cmd: tea.Batch(cmds...), ok: true, focus: true}
	}
	return navJumpResult{ok: true, focus: true}
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

	applyJump := func(cmd tea.Cmd, focus bool) tea.Cmd {
		out := applyFilter(cmd)
		if !focus {
			return out
		}
		return batchCommands(out, m.setFocus(focusEditor))
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok && m.navigatorFilter.Focused() &&
		navigatorFilterConsumesKey(keyMsg) {
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
			if ev.String() == "l" {
				if res := m.navReqJumpCmd(); res.ok {
					return applyJump(res.cmd, res.focus)
				}
			}
			if m.navigatorFilter.Focused() {
				m.navigatorFilter.Blur()
				return nil
			}
			n := m.navigator.Selected()
			if n == nil {
				return nil
			}
			switch n.Kind {
			case navigator.KindFile:
				path := n.Payload.FilePath
				if path != "" && filepath.Clean(path) != filepath.Clean(m.currentFile) {
					cmd = m.openFile(path)
				}
				if filesvc.IsRTSFile(path) {
					return applyJump(cmd, true)
				}
				m.navExpandFile(n, false)
			case navigator.KindDir:
				m.navExpandDir(n, false)
			default:
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
			switch n.Kind {
			case navigator.KindFile:
				path := n.Payload.FilePath
				if path != "" && filepath.Clean(path) != filepath.Clean(m.currentFile) {
					cmd = m.openFile(path)
				}
				m.navExpandFile(n, false)
			case navigator.KindDir:
				m.navExpandDir(n, false)
			default:
				// Let main key handling drive request/workflow actions.
				return nil
			}
		case " ":
			n := m.navigator.Selected()
			if n == nil {
				return nil
			}
			switch n.Kind {
			case navigator.KindFile:
				m.navExpandFile(n, true)
			case navigator.KindDir:
				m.navExpandDir(n, true)
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
			res := m.navReqJumpCmd()
			if res.ok {
				return applyJump(res.cmd, res.focus)
			}
		}
	}

	return applyFilter(cmd)
}

func (m *Model) navExpandFile(n *navigator.Node[any], toggle bool) {
	if m.navigator == nil || n == nil {
		return
	}
	has := len(n.Children) > 0
	if !has {
		m.expandNavigatorFile(n.Payload.FilePath)
		if refreshed := m.navigator.Find(n.ID); refreshed != nil {
			n = refreshed
		}
	}
	if n == nil || len(n.Children) == 0 {
		return
	}
	changed := false
	if toggle && has {
		n.Expanded = !n.Expanded
		changed = true
	} else if !n.Expanded {
		n.Expanded = true
		changed = true
	}
	if changed {
		m.navigator.Refresh()
	}
}

func (m *Model) navExpandDir(n *navigator.Node[any], toggle bool) {
	if m.navigator == nil || n == nil || len(n.Children) == 0 {
		return
	}
	changed := false
	if toggle {
		n.Expanded = !n.Expanded
		changed = true
	} else if !n.Expanded {
		n.Expanded = true
		changed = true
	}
	if changed {
		m.navigator.Refresh()
	}
}

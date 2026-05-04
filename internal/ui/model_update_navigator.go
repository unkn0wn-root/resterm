package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/ui/navigator"
	"github.com/unkn0wn-root/resterm/internal/util"
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

func (m *Model) navJumpCmd(key string) navJumpResult {
	if m.navigator == nil {
		return navJumpResult{}
	}
	n := m.navigator.Selected()
	if n == nil {
		return navJumpResult{}
	}

	switch n.Kind {
	case navigator.KindRequest:
		return m.navReqJumpCmd(key)
	case navigator.KindWorkflow:
		return m.navWfJumpCmd(key)
	default:
		return navJumpResult{}
	}
}

func (m *Model) navReqJumpCmd(key string) navJumpResult {
	action, hint := navJumpCrossFileAction(key)
	prep := m.resolveNavReq(action, hint)
	if !prep.ok {
		if len(prep.cmds) > 0 {
			return navJumpResult{cmd: tea.Batch(prep.cmds...)}
		}
		return navJumpResult{}
	}
	cmds := prep.cmds
	if cmd := m.restorePane(paneRegionEditor); cmd != nil {
		cmds = append(cmds, cmd)
	}
	m.jumpToNavigatorRequest(prep.request, true)
	if len(cmds) > 0 {
		return navJumpResult{cmd: tea.Batch(cmds...), ok: true, focus: true}
	}
	return navJumpResult{ok: true, focus: true}
}

func (m *Model) navWfJumpCmd(key string) navJumpResult {
	action, hint := navJumpCrossFileAction(key)
	prep := m.resolveNavWf(action, hint)
	if !prep.ok {
		if len(prep.cmds) > 0 {
			return navJumpResult{cmd: tea.Batch(prep.cmds...)}
		}
		return navJumpResult{}
	}
	cmds := prep.cmds
	if cmd := m.restorePane(paneRegionEditor); cmd != nil {
		cmds = append(cmds, cmd)
	}
	m.jumpToNavigatorWorkflow(prep.workflow, true)
	if len(cmds) > 0 {
		return navJumpResult{cmd: tea.Batch(cmds...), ok: true, focus: true}
	}
	return navJumpResult{ok: true, focus: true}
}

func navJumpCrossFileAction(key string) (string, string) {
	if key == "r" {
		return navActionJumpR, "Press r again to jump."
	}
	return navActionJumpL, "Press l again to jump."
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
		focusCmd := m.setFocus(focusEditor)
		if m.focus == focusEditor {
			m.suppressEditorKey = true
		}
		return batchCommands(out, focusCmd)
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
			selected := m.navigator.Selected()
			if ev.String() == "l" && navJumpable(selected) {
				res := m.navJumpCmd("l")
				if res.ok {
					return applyJump(res.cmd, res.focus)
				}
				if res.cmd != nil {
					return applyFilter(res.cmd)
				}
				return applyFilter(nil)
			}
			if m.navigatorFilter.Focused() {
				m.navigatorFilter.Blur()
				return nil
			}
			n := selected
			if n == nil {
				return nil
			}
			switch n.Kind {
			case navigator.KindFile:
				path := n.Payload.FilePath
				if path != "" && !util.SamePath(path, m.currentFile) {
					if !m.confirmCrossFileNavigation(
						n,
						navActionOpenFile,
						navOpenFileRetryHint(ev.String()),
					) {
						return applyFilter(nil)
					}
					cmd = m.openFile(path)
				}
				if path != "" && !filesvc.IsRequestFile(path) {
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
				if path != "" && !util.SamePath(path, m.currentFile) {
					if !m.confirmCrossFileNavigation(
						n,
						navActionOpenFile,
						navOpenFileRetryHint(ev.String()),
					) {
						return applyFilter(nil)
					}
					cmd = m.openFile(path)
				}
				if path != "" && !filesvc.IsRequestFile(path) {
					return applyJump(cmd, true)
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
			res := m.navJumpCmd("r")
			if res.ok {
				return applyJump(res.cmd, res.focus)
			}
			if res.cmd != nil {
				return applyFilter(res.cmd)
			}
		}
	}

	return applyFilter(cmd)
}

func navJumpable(n *navigator.Node[any]) bool {
	return n != nil && (n.Kind == navigator.KindRequest || n.Kind == navigator.KindWorkflow)
}

func navOpenFileRetryHint(key string) string {
	switch key {
	case "enter":
		return "Press Enter again to open."
	case "right":
		return "Press Right again to open."
	case "l":
		return "Press l again to open."
	default:
		return "Repeat the open action to continue."
	}
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

package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/bindings"
)

type helpSection struct {
	title   string
	entries []helpEntry
}

type helpEntry struct {
	key         string
	description string
}

func (m *Model) focusHelpFilter() tea.Cmd {
	m.helpFilter.CursorEnd()
	return m.helpFilter.Focus()
}

func (m *Model) clearHelpFilter() {
	m.helpFilter.SetValue("")
	m.helpFilter.Blur()
	m.resetHelpViewport()
}

func (m *Model) updateHelpFilter(msg tea.Msg) tea.Cmd {
	prev := m.helpFilter.Value()
	var cmd tea.Cmd
	m.helpFilter, cmd = m.helpFilter.Update(msg)
	if m.helpFilter.Value() != prev {
		m.resetHelpViewport()
	}
	return cmd
}

func (m *Model) resetHelpViewport() {
	if vp := m.helpViewport; vp != nil {
		vp.SetYOffset(0)
		vp.GotoTop()
	}
}

func (m *Model) handleHelpKey(msg tea.KeyMsg) tea.Cmd {
	keyStr := msg.String()
	if m.helpFilter.Focused() {
		switch keyStr {
		case "ctrl+q", "ctrl+d":
			return tea.Quit
		case "esc":
			if strings.TrimSpace(m.helpFilter.Value()) != "" {
				m.clearHelpFilter()
				return nil
			}
			m.clearHelpFilter()
			m.showHelp = false
			m.helpJustOpened = false
			return nil
		case "enter":
			m.helpFilter.Blur()
			return nil
		default:
			return m.updateHelpFilter(msg)
		}
	}

	vp := m.helpViewport
	switch keyStr {
	case "ctrl+q", "ctrl+d":
		return tea.Quit
	case "/", "shift+f", "F":
		return m.focusHelpFilter()
	case "esc":
		if strings.TrimSpace(m.helpFilter.Value()) != "" {
			m.clearHelpFilter()
			return nil
		}
		m.clearHelpFilter()
		m.showHelp = false
		m.helpJustOpened = false
	case "?", "shift+/":
		m.clearHelpFilter()
		m.showHelp = false
		m.helpJustOpened = false
	case "down", "j":
		if vp != nil {
			vp.ScrollDown(1)
		}
	case "up", "k":
		if vp != nil {
			vp.ScrollUp(1)
		}
	case "pgdown", "ctrl+f":
		if vp != nil {
			vp.ScrollDown(maxInt(1, vp.Height))
		}
	case "pgup", "ctrl+b", "ctrl+u":
		if vp != nil {
			vp.ScrollUp(maxInt(1, vp.Height))
		}
	case "home":
		if vp != nil {
			vp.GotoTop()
		}
	case "end":
		if vp != nil {
			vp.GotoBottom()
		}
	}
	return nil
}

func (m Model) helpSections() []helpSection {
	return []helpSection{
		{
			title: "Navigation & Focus",
			entries: sortedHelpEntries([]helpEntry{
				{m.helpActionKey(bindings.ActionCycleFocusNext, "Tab"), "Cycle focus"},
				{m.helpActionKey(bindings.ActionCycleFocusPrev, "Shift+Tab"), "Reverse focus"},
				{
					m.helpCombinedKey(
						[]bindings.ActionID{bindings.ActionToggleZoom, bindings.ActionClearZoom},
						"g z / g Z",
					),
					"Zoom focused pane / reset zoom",
				},
				{
					m.helpCombinedKey(
						[]bindings.ActionID{
							bindings.ActionFocusRequests,
							bindings.ActionFocusEditorNormal,
							bindings.ActionFocusResponse,
						},
						"g r / g i / g p",
					),
					"Focus navigator / editor / response",
				},
			}),
		},
		{
			title: "Requests & Files",
			entries: sortedHelpEntries([]helpEntry{
				{"Enter", "Run selected request"},
				{"Space", "Preview selected request / toggle file expansion"},
				{
					m.helpActionKey(bindings.ActionShowRequestDetails, "g ,"),
					"Show selected request details",
				},
				{m.helpActionKey(bindings.ActionSendRequest, "Ctrl+Enter"), "Send active request"},
				{
					m.helpActionKey(bindings.ActionExplainRequest, "g x"),
					"Prepare Explain preview (no request sent)",
				},
				{
					m.helpActionKey(bindings.ActionCancelRun, "Ctrl+C"),
					"Cancel in-flight run/request",
				},
				{m.helpActionKey(bindings.ActionSaveFile, "Ctrl+S"), "Save current file"},
				{
					m.helpActionKey(bindings.ActionSaveLayout, "g Shift+L"),
					"Save layout to settings",
				},
				{m.helpActionKey(bindings.ActionOpenNewFileModal, "Ctrl+N"), "Create request file"},
				{m.helpActionKey(bindings.ActionOpenPathModal, "Ctrl+O"), "Open file or folder"},
				{
					m.helpActionKey(bindings.ActionReloadWorkspace, "Ctrl+Shift+O"),
					"Refresh workspace",
				},
				{m.helpActionKey(bindings.ActionOpenTempDocument, "Ctrl+T"), "Temporary document"},
				{m.helpActionKey(bindings.ActionReparseDocument, "Ctrl+P"), "Reparse document"},
				{
					m.helpActionKey(bindings.ActionReloadFileFromDisk, "Ctrl+Alt+R"),
					"Reload file from disk",
				},
				{m.helpActionKey(bindings.ActionQuitApp, "Ctrl+Q"), "Quit (Ctrl+D also works)"},
				{m.helpActionKey(bindings.ActionToggleHelp, "?"), "Toggle this help"},
			}),
		},
		{
			title: "Navigator & Filters",
			entries: sortedHelpEntries([]helpEntry{
				{"/ (Esc clears)", "Focus navigator filter / reset filters"},
				{"m", "Navigator: toggle method filter for selected request"},
				{"t", "Navigator: toggle tag filters for selected item"},
				{"l / r", "Navigator: jump to selected request in editor"},
				{
					m.helpCombinedKey(
						[]bindings.ActionID{
							bindings.ActionSidebarHeightDecrease,
							bindings.ActionSidebarHeightIncrease,
						},
						"g j / g k",
					),
					"Collapse / expand current navigator branch",
				},
				{
					m.helpCombinedKey(
						[]bindings.ActionID{
							bindings.ActionWorkflowHeightIncrease,
							bindings.ActionWorkflowHeightDecrease,
						},
						"g Shift+J / g Shift+K",
					),
					"Collapse all / expand all navigator branches",
				},
			}),
		},
		{
			title: "Layout & View",
			entries: sortedHelpEntries([]helpEntry{
				{
					m.helpCombinedKey(
						[]bindings.ActionID{
							bindings.ActionToggleResponseSplitVert,
							bindings.ActionToggleResponseSplitHorz,
						},
						"Ctrl+V / Ctrl+U",
					),
					"Split response vertically / horizontally",
				},
				{
					m.helpActionKey(bindings.ActionTogglePaneFollowLatest, "Ctrl+Shift+V"),
					"Pin or unpin focused response pane",
				},
				{
					m.helpActionKey(bindings.ActionCopyResponseTab, "Ctrl+Shift+C"),
					"Copy Pretty / Raw / Headers response tab",
				},
				{
					m.helpCombinedKey(
						[]bindings.ActionID{
							bindings.ActionScrollResponseTop,
							bindings.ActionScrollResponseBottom,
						},
						"gg / G",
					),
					"Response/History tab: top / bottom",
				},
				{
					m.helpActionKey(bindings.ActionToggleHeaderPreview, "g Shift+H"),
					"Toggle request/response headers view",
				},
				{
					m.helpActionKey(bindings.ActionCycleRawView, "g b"),
					"Cycle raw view: text / hex / base64 (summary for large binary)",
				},
				{
					m.helpActionKey(bindings.ActionShowRawDump, "g Shift+D"),
					"Load full raw dump (hex)",
				},
				{
					m.helpActionKey(bindings.ActionSaveResponseBody, "g Shift+S"),
					"Save response body to file",
				},
				{
					m.helpActionKey(bindings.ActionOpenResponseExternally, "g Shift+E"),
					"Open response in external app",
				},
				{"Ctrl+F or Ctrl+B, ←/→", "Send future responses to selected pane"},
				{
					m.helpCombinedKey(
						[]bindings.ActionID{
							bindings.ActionSidebarWidthDecrease,
							bindings.ActionSidebarWidthIncrease,
						},
						"g h / g l",
					),
					"Adjust editor/response width",
				},
				{
					m.helpCombinedKey(
						[]bindings.ActionID{
							bindings.ActionToggleSidebarCollapse,
							bindings.ActionToggleEditorCollapse,
							bindings.ActionToggleResponseCollapse,
						},
						"g1 / g2 / g3",
					),
					"Toggle sidebar / editor / response minimize",
				},
			}),
		},
		{
			title: "Streaming & WebSocket",
			entries: sortedHelpEntries([]helpEntry{
				{"Ctrl+Space", "Stream tab: pause or resume live follow"},
				{"Ctrl+F", "Stream tab: filter events (Enter apply, Esc cancel)"},
				{"Ctrl+B", "Stream tab: add bookmark"},
				{"Ctrl+Up / Ctrl+Down", "Stream tab: previous / next bookmark"},
				{
					fmt.Sprintf(
						"%s, then i / p / c / l",
						m.helpActionKey(bindings.ActionToggleWebsocketConsole, "g w"),
					),
					"WebSocket commands: console / ping / close / clear",
				},
				{"F2", "WebSocket console: cycle payload mode"},
				{"Ctrl+S / Ctrl+Enter", "WebSocket console: send payload"},
				{"Up / Down", "WebSocket console: previous / next payload"},
				{"Esc", "WebSocket console: exit input focus"},
			}),
		},
		{
			title: "History",
			entries: sortedHelpEntries([]helpEntry{
				{"c", "History: cycle scope"},
				{"s", "History: toggle sort"},
				{"/", "History: filter (Enter apply, Esc clear)"},
				{"Space", "History: toggle selection"},
				{"PgUp / PgDn", "History: prev / next page"},
				{"Enter", "History: load entry"},
				{"p", "History: preview entry"},
				{"d", "History: delete selection"},
				{"r", "History: replay entry"},
			}),
		},
		{
			title: "Environment & Themes",
			entries: sortedHelpEntries([]helpEntry{
				{m.helpActionKey(bindings.ActionShowGlobals, "Ctrl+G"), "Show globals summary"},
				{
					m.helpActionKey(bindings.ActionClearGlobals, "Ctrl+Shift+G"),
					"Clear globals for environment",
				},
				{m.helpActionKey(bindings.ActionOpenEnvSelector, "Ctrl+E"), "Environment selector"},
				{
					m.helpActionKey(bindings.ActionSelectTimelineTab, "Ctrl+Alt+L / g t"),
					"Timeline tab",
				},
				{
					m.helpActionKey(bindings.ActionOpenThemeSelector, "Ctrl+Alt+T / g m"),
					"Theme selector",
				},
			}),
		},
		{
			title: "Editor motions",
			entries: []helpEntry{
				{"h / j / k / l", "Move left / down / up / right"},
				{"w / b / e", "Word forward / back / end (W / B / E for WORD)"},
				{"0 / ^ / $", "Line start / first non-blank / line end"},
				{"gg / G", "Top / bottom of buffer"},
				{"Ctrl+f / Ctrl+b", "Page down / up (Ctrl+d / Ctrl+u half-page)"},
				{"v / V / y", "Visual select (char / line) / yank selection"},
				{"d / c + motion", "Delete / change via Vim motions (dw, db, cw, c$)"},
				{"dd / D / x / cc", "Delete line / to end / char / change line"},
				{"a", "Append after cursor (enter insert mode)"},
				{"p / P", "Paste after / before cursor"},
				{"f / t / T", "Find character (forward / till / backward)"},
				{"u / Ctrl+r", "Undo / redo last edit"},
			},
		},
		{
			title: "Response selection",
			entries: []helpEntry{
				{"v / V", "Response: show cursor / start selection"},
				{"j / k / ↑ / ↓", "Response: move cursor / extend selection"},
				{"y / c", "Response: copy selection"},
				{"Esc", "Response: clear selection (again clears cursor)"},
			},
		},
		{
			title: "Search",
			entries: []helpEntry{
				{"/", "Help: focus help search"},
				{"Shift+F", "Open search prompt (Ctrl+R toggles regex)"},
				{"n / p", "Next / previous match (wraps around)"},
			},
		},
	}
}

func (m Model) filteredHelpSections() []helpSection {
	sections := m.helpSections()
	tokens := filterQueryTokens(m.helpFilter.Value())
	if len(tokens) == 0 {
		return sections
	}

	out := make([]helpSection, 0, len(sections))
	for _, section := range sections {
		if helpTextMatchesAll(section.title, tokens) {
			out = append(out, section)
			continue
		}

		filtered := make([]helpEntry, 0, len(section.entries))
		for _, entry := range section.entries {
			if helpEntryMatchesAll(entry, tokens) {
				filtered = append(filtered, entry)
			}
		}
		if len(filtered) == 0 {
			continue
		}
		out = append(out, helpSection{
			title:   section.title,
			entries: filtered,
		})
	}
	return out
}

func helpEntryMatchesAll(entry helpEntry, tokens []string) bool {
	if len(tokens) == 0 {
		return true
	}
	return helpTextMatchesAll(entry.key+" "+entry.description, tokens)
}

func helpTextMatchesAll(text string, tokens []string) bool {
	if len(tokens) == 0 {
		return true
	}
	haystack := strings.ToLower(strings.TrimSpace(text))
	if haystack == "" {
		return false
	}
	for _, token := range tokens {
		if token == "" {
			continue
		}
		if !strings.Contains(haystack, token) {
			return false
		}
	}
	return true
}

func sortedHelpEntries(entries []helpEntry) []helpEntry {
	cleaned := make([]helpEntry, 0, len(entries))
	for _, entry := range entries {
		key := strings.TrimSpace(entry.key)
		description := strings.TrimSpace(entry.description)
		if key == "" || description == "" {
			continue
		}
		cleaned = append(cleaned, helpEntry{
			key:         key,
			description: description,
		})
	}

	sort.Slice(cleaned, func(i, j int) bool {
		return strings.ToLower(cleaned[i].key) < strings.ToLower(cleaned[j].key)
	})

	return cleaned
}

func helpRow(m Model, key, description string) string {
	keyStyled := m.theme.HeaderTitle.
		Width(helpKeyColumnWidth).
		Align(lipgloss.Left).
		Render(key)
	descStyled := m.theme.HeaderValue.
		PaddingLeft(6).
		Render(description)
	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		keyStyled,
		descStyled,
	)
}

package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/bindings"
	"github.com/unkn0wn-root/resterm/internal/helpdoc"
	"github.com/unkn0wn-root/resterm/internal/mdterm"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
)

type helpSection struct {
	title   string
	entries []helpEntry
}

type helpEntry struct {
	key         string
	description string
}

const helpTopicsSectionTitle = "Documentation Topics"

func (m *Model) toggleHelp() {
	if m.showHelp {
		m.closeHelp()
		return
	}
	m.openHelpIndex("")
}

func (m *Model) openHelpQuery(args []string) tea.Cmd {
	query := strings.Join(args, " ")
	if topic, ok := helpdoc.Lookup(query); ok {
		m.openHelpTopic(topic)
		return nil
	}
	return m.openHelpIndex(query)
}

func (m *Model) openHelpIndex(query string) tea.Cmd {
	m.openHelp()
	if query == "" {
		return nil
	}
	m.helpFilter.SetValue(query)
	return m.focusHelpFilter()
}

func (m *Model) openHelpTopic(topic helpdoc.Topic) {
	m.openHelp()
	m.helpTopic = &topic
}

func (m *Model) openHelp() {
	if m.showEnvSelector {
		m.closeEnvironmentSelector()
	}
	m.showHelp = true
	m.helpJustOpened = true
	m.helpTopic = nil
	m.showThemeSelector = false
	m.clearHelpFilter()
}

func (m *Model) closeHelp() {
	m.showHelp = false
	m.helpJustOpened = false
	m.helpTopic = nil
	m.clearHelpFilter()
}

func (m *Model) focusHelpFilter() tea.Cmd {
	m.helpFilter.CursorEnd()
	return m.helpFilter.Focus()
}

func (m *Model) clearHelpFilter() {
	m.helpFilter.SetValue("")
	m.helpFilter.Blur()
	m.helpViewport.GotoTop()
}

func (m *Model) updateHelpFilter(msg tea.Msg) tea.Cmd {
	prev := m.helpFilter.Value()
	var cmd tea.Cmd
	m.helpFilter, cmd = m.helpFilter.Update(msg)
	if m.helpFilter.Value() != prev {
		m.helpViewport.GotoTop()
	}
	return cmd
}

func (m *Model) handleHelpKey(msg tea.KeyMsg) tea.Cmd {
	keyStr := msg.String()
	if m.helpTopic != nil {
		switch keyStr {
		case "ctrl+q", "ctrl+d":
			return tea.Quit
		case "esc", "?", "shift+/":
			m.closeHelp()
		case "o":
			return m.openTopicDoc(*m.helpTopic)
		default:
			scrollViewportKey(m.helpViewport, keyStr)
		}
		return nil
	}
	if m.helpFilter.Focused() {
		switch keyStr {
		case "ctrl+q", "ctrl+d":
			return tea.Quit
		case "esc":
			if strings.TrimSpace(m.helpFilter.Value()) != "" {
				m.clearHelpFilter()
				return nil
			}
			m.closeHelp()
			return nil
		case "enter":
			m.helpFilter.Blur()
			return nil
		default:
			return m.updateHelpFilter(msg)
		}
	}

	if isSearchTriggerKey(keyStr) {
		return m.focusHelpFilter()
	}
	switch keyStr {
	case "ctrl+q", "ctrl+d":
		return tea.Quit
	case "esc":
		if strings.TrimSpace(m.helpFilter.Value()) != "" {
			m.clearHelpFilter()
			return nil
		}
		m.closeHelp()
	case "?", "shift+/":
		m.closeHelp()
	default:
		scrollViewportKey(m.helpViewport, keyStr)
	}
	return nil
}

func (m Model) helpSections() []helpSection {
	editorEntries := []helpEntry{
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
	}
	if key := m.helpBindingLabel(bindings.ActionShowContextHelp); key != "" {
		editorEntries = append([]helpEntry{{
			key:         key,
			description: "Open help for the directive or keyword under the cursor",
		}}, editorEntries...)
	}

	var statusMessageEntries []helpEntry
	if key := m.helpBindingLabel(bindings.ActionShowStatusMessage); key != "" {
		statusMessageEntries = append(statusMessageEntries, helpEntry{
			key:         key,
			description: "Show document warnings or the current status in full",
		})
	}

	sections := []helpSection{
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
				{
					m.helpActionKey(bindings.ActionOpenFileInEditor, "g e"),
					"Open file in external editor",
				},
				{m.helpActionKey(bindings.ActionOpenNewFileModal, "Ctrl+N"), "Create request file"},
				{m.helpActionKey(bindings.ActionOpenPathModal, "Ctrl+O"), "Browse for a file or workspace"},
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
				{"l / r", "Navigator: jump to selected request/workflow in editor"},
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
			entries: sortedHelpEntries(append(statusMessageEntries, []helpEntry{
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
					"Response/History tab: top / bottom; Workflow list: first / last step",
				},
				{"Enter / Esc", "Workflow tab: focus detail / return to step list"},
				{"j / k / PgUp / PgDn", "Workflow tab: step navigation or focused detail scroll"},
				{"Enter / Space", "Headers tab: switch response / request"},
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
					"Adjust editor/response width in side-by-side layout",
				},
				{
					m.helpCombinedKey(
						[]bindings.ActionID{
							bindings.ActionSidebarHeightDecrease,
							bindings.ActionSidebarHeightIncrease,
						},
						"g j / g k",
					),
					"Adjust editor/response height in stacked layout",
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
			}...)),
		},
		{
			title: "Mock Server",
			entries: sortedHelpEntries([]helpEntry{
				{
					m.helpActionKey(bindings.ActionToggleMockServer, "g Shift+M"),
					"Start or stop the workspace mock server",
				},
				{
					m.helpActionKey(bindings.ActionCaptureMockResponse, "g a"),
					"Capture the focused HTTP response as a mock",
				},
				{":mock logs", "Open mock request log (c clears the log)"},
				{":mock start --source [path]", "Start with request files selected from the path popup"},
				{":mock reset [sequence]", "Reset all or one named response sequence"},
				{":mock verify", "Verify active # @expect call counts"},
				{":mock status", "Show address, routes, scenarios, and calls"},
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
					"Clear globals and cookies for environment",
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
			title:   "Editor motions",
			entries: editorEntries,
		},
		{
			title: "Command line",
			entries: []helpEntry{
				{":", "Open Vim-style command line"},
				{":w / :write", "Save current file"},
				{":q / :qa", "Quit when there are no unsaved changes"},
				{":q! / :qa!", "Quit without saving"},
				{":wq / :x", "Save and quit"},
				{":e [path] / :edit [path]", "Open a path directly, or show the path prompt when omitted"},
				{":noh", "Clear search highlights"},
				{":help [topic] / :man [topic]", "Open embedded help or a documentation topic"},
				{":docs [topic]", "Open version-matched web documentation"},
				{"Up / Down / Tab / Enter", "Select, complete, or run command suggestions"},
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
				{"Shift+F or /", "Editor / response: open search prompt"},
				{"Ctrl+R", "Search prompt: toggle literal / regex"},
				{"/", "History tab: focus history filter"},
				{"n / p", "Next / previous match (wraps around)"},
			},
		},
	}
	return sections
}

func helpTopicSection(topics []helpdoc.Topic) helpSection {
	entries := make([]helpEntry, 0, len(topics))
	for _, topic := range topics {
		entries = append(entries, helpEntry{
			key:         ":help " + topic.ID,
			description: topic.Summary,
		})
	}
	return helpSection{title: helpTopicsSectionTitle, entries: entries}
}

// filteredHelpSections narrows the shortcut sections to the filter tokens and
// surfaces documentation topics whose metadata or body match the query. The
// unfiltered index lists only shortcuts because the :help popup already
// suggests every topic.
func (m *Model) filteredHelpSections() []helpSection {
	sections := m.helpSections()
	tokens := filterQueryTokens(m.helpFilter.Value())
	if len(tokens) == 0 {
		return sections
	}

	out := make([]helpSection, 0, len(sections)+1)
	if topics := helpdoc.Search(m.helpFilter.Value()); len(topics) > 0 {
		out = append(out, helpTopicSection(topics))
	}
	for _, section := range sections {
		if helpTextMatchesAll(section.title, tokens) {
			out = append(out, section)
			continue
		}

		filtered := make([]helpEntry, 0, len(section.entries))
		for _, entry := range section.entries {
			if helpTextMatchesAll(entry.key+" "+entry.description, tokens) {
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

func helpTextMatchesAll(text string, tokens []string) bool {
	haystack := strings.ToLower(text)
	for _, token := range tokens {
		if !strings.Contains(haystack, token) {
			return false
		}
	}
	return true
}

func sortedHelpEntries(entries []helpEntry) []helpEntry {
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].key) < strings.ToLower(entries[j].key)
	})
	return entries
}

func (m *Model) renderHelpOverlay() string {
	box, _ := m.helpOverlayBox()
	return m.renderCenteredModal(box)
}

func (m Model) helpOverlayBox() (string, mouseRect) {
	// topics are prose and read best narrow, the index needs room for its
	// key and description columns
	maxWidth := 120
	if m.helpTopic != nil {
		maxWidth = 100
	}
	width := max(min(m.width-6, maxWidth), 48)
	viewWidth := max(width-4, 22)

	header := func(text string) string {
		return m.theme.HeaderTitle.Width(viewWidth).Align(lipgloss.Center).Render(text)
	}
	subtitle := func(text string) string {
		return m.theme.HeaderValue.Width(viewWidth).Align(lipgloss.Center).Render(text)
	}

	var top []string
	var body string
	if topic := m.helpTopic; topic != nil {
		top = []string{
			header(topic.Title),
			subtitle("o web docs • Esc close • ↑/↓ scroll • PgUp/PgDn page"),
			"",
		}
		body = mdterm.Render(topic.Body, mdterm.Options{Width: viewWidth, Color: helpColor()})
	} else {
		top = []string{
			header("Help"),
			subtitle("/ search docs and shortcuts • Esc clear/close • ↑/↓ scroll • PgUp/PgDn page"),
			"",
			m.renderHelpFilter(viewWidth),
			"",
		}
		body = m.helpIndexBody(viewWidth)
	}

	topView := lipgloss.NewStyle().
		Padding(0, 2).
		Width(width).
		Render(lipgloss.JoinVertical(lipgloss.Left, top...))
	bodyHeight := max(max(m.height-8, 6)-lipgloss.Height(topView), 6)
	if m.helpTopic != nil {
		// a topic body is static, so the box shrinks to fit short topics. The
		// index keeps its height stable while the filter is typed.
		bodyHeight = min(bodyHeight, lipgloss.Height(body))
	}
	bodyView := m.renderHelpViewport(body, viewWidth, width, bodyHeight)

	style := m.theme.BrowserBorder
	if m.helpTopic != nil {
		// the topic body carries its own mdterm colors, so frame text
		// attributes would leak only until the first inner reset
		style = stripTextAttrs(style)
	}
	content := lipgloss.JoinVertical(lipgloss.Left, topView, bodyView)
	box := style.Width(width).Render(content)
	return box, m.helpOverlayBodyRect(box, lipgloss.Height(topView), bodyHeight)
}

// helpColor matches mdterm output to the color depth lipgloss negotiated for
// the rest of the UI. Monochrome terminals keep the plain-text fallbacks.
func helpColor() termcolor.Config {
	p := lipgloss.ColorProfile()
	if p == termenv.Ascii {
		return termcolor.Config{}
	}
	return termcolor.Config{Enabled: true, Profile: p}
}

func (m *Model) helpIndexBody(viewWidth int) string {
	sections := m.filteredHelpSections()
	if len(sections) == 0 {
		return m.theme.HeaderValue.Render("No help entries match the current filter.")
	}

	rows := make([]string, 0, len(sections)*8)
	for idx, section := range sections {
		rows = append(rows, m.theme.HeaderTitle.Width(viewWidth).Render(section.title))
		for _, entry := range section.entries {
			rows = append(rows, m.helpRow(entry.key, entry.description))
		}
		if idx < len(sections)-1 {
			rows = append(rows, "")
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (m *Model) renderHelpViewport(body string, viewWidth, contentWidth, bodyHeight int) string {
	vp := m.helpViewport
	vp.Width = viewWidth
	vp.Height = bodyHeight
	vp.SetContent(body)
	return lipgloss.NewStyle().
		Padding(0, 2).
		Width(contentWidth).
		Height(bodyHeight).
		Render(vp.View())
}

func (m *Model) helpOverlayBodyRect(box string, topHeight, bodyHeight int) mouseRect {
	boxWidth := lipgloss.Width(box)
	boxHeight := lipgloss.Height(box)
	width := max(m.width, boxWidth)
	height := max(m.height, boxHeight)
	if width <= 0 || height <= 0 {
		return mouseRect{}
	}

	x := max((width-boxWidth)/2, 0)
	y := max((height-boxHeight)/2, 0)
	style := m.theme.BrowserBorder
	left := style.GetBorderLeftSize() + style.GetPaddingLeft()
	right := style.GetBorderRightSize() + style.GetPaddingRight()
	top := style.GetBorderTopSize() + style.GetPaddingTop()
	bottom := style.GetBorderBottomSize() + style.GetPaddingBottom()
	availableHeight := max(boxHeight-top-bottom-topHeight, 0)
	return mouseRect{
		x: x + left,
		y: y + top + topHeight,
		w: max(boxWidth-left-right, 0),
		h: min(bodyHeight, availableHeight),
	}
}

func (m Model) renderHelpFilter(width int) string {
	if width < 16 {
		width = 16
	}
	m.helpFilter.Width = width
	input := lipgloss.NewStyle().
		Width(width).
		Render(m.helpFilter.View())

	if !m.helpFilter.Focused() {
		return input
	}
	hintText := "Type to filter the help • Enter done • Esc clear/close"

	return lipgloss.JoinVertical(
		lipgloss.Left,
		input,
		m.themeRuntime.helpHintStyle(m.theme).Width(width).Render(hintText),
	)
}

func (m *Model) helpRow(key, description string) string {
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

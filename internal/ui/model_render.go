package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/theme"
)

func (m Model) View() string {
	if !m.ready {
		return m.renderWithinAppFrame("Initialising...")
	}

	if m.showErrorModal {
		return m.renderWithinAppFrame(m.renderErrorModal())
	}

	if m.showHistoryPreview {
		return m.renderWithinAppFrame(m.renderHistoryPreviewModal())
	}

	if m.showOpenModal {
		return m.renderWithinAppFrame(m.renderOpenModal())
	}

	if m.showNewFileModal {
		return m.renderWithinAppFrame(m.renderNewFileModal())
	}

	filePane := m.renderFilePane()
	editorPane := m.renderEditorPane()
	responsePane := m.renderResponsePane()

	panes := lipgloss.JoinHorizontal(
		lipgloss.Top,
		filePane,
		editorPane,
		responsePane,
	)
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderCommandBar(),
		panes,
		m.renderStatusBar(),
	)
	header := m.renderHeader()
	base := lipgloss.JoinVertical(lipgloss.Left, header, body)
	if m.showHelp {
		return m.renderWithinAppFrame(m.renderHelpOverlay())
	}
	if m.showEnvSelector {
		return m.renderWithinAppFrame(m.renderEnvironmentModal())
	}
	return m.renderWithinAppFrame(base)
}

func (m Model) renderWithinAppFrame(content string) string {
	innerWidth := maxInt(m.width, lipgloss.Width(content))
	innerHeight := maxInt(m.height, lipgloss.Height(content))

	if innerWidth > 0 {
		content = lipgloss.Place(
			innerWidth,
			lipgloss.Height(content),
			lipgloss.Top,
			lipgloss.Left,
			content,
			lipgloss.WithWhitespaceChars(" "),
		)
	}
	if innerWidth > 0 && innerHeight > lipgloss.Height(content) {
		content = lipgloss.Place(
			innerWidth,
			innerHeight,
			lipgloss.Top,
			lipgloss.Left,
			content,
			lipgloss.WithWhitespaceChars(" "),
		)
	}

	framed := m.theme.AppFrame.Render(content)

	frameWidth := maxInt(m.frameWidth, lipgloss.Width(framed))
	frameHeight := maxInt(m.frameHeight, lipgloss.Height(framed))

	if frameWidth > lipgloss.Width(framed) ||
		frameHeight > lipgloss.Height(framed) {
		framed = lipgloss.Place(
			frameWidth,
			frameHeight,
			lipgloss.Top,
			lipgloss.Left,
			framed,
			lipgloss.WithWhitespaceChars(" "),
		)
	}

	return framed
}

func (m Model) renderFilePane() string {
	style := m.theme.BrowserBorder
	paneActive := m.focus == focusFile || m.focus == focusRequests || m.focus == focusWorkflows
	switch m.focus {
	case focusFile:
		style = style.
			BorderForeground(m.theme.PaneBorderFocusFile).
			Bold(true).
			BorderStyle(lipgloss.ThickBorder())
	case focusRequests:
		style = style.
			BorderForeground(m.theme.PaneBorderFocusRequests).
			Bold(true).
			BorderStyle(lipgloss.ThickBorder())
	case focusWorkflows:
		style = style.
			BorderForeground(m.theme.PaneBorderFocusRequests).
			Bold(true).
			BorderStyle(lipgloss.ThickBorder())
	}

	faintStyle := lipgloss.NewStyle().Faint(true)
	if !paneActive {
		style = style.Faint(true)
	}

	width := m.fileList.Width() + 4
	innerWidth := maxInt(1, width-4)
	titleBase := m.theme.PaneTitle.Width(innerWidth).Align(lipgloss.Center)
	filesTitle := titleBase.Render(strings.ToUpper("Files"))
	requestsTitle := titleBase.Render(strings.ToUpper("Requests"))
	workflowsTitle := titleBase.Render(strings.ToUpper("Workflows"))
	if m.focus == focusFile {
		filesTitle = m.theme.PaneTitleFile.
			Width(innerWidth).
			Align(lipgloss.Center).
			Foreground(m.theme.PaneActiveForeground).
			Render(strings.ToUpper("Files"))
	}
	if m.focus == focusRequests {
		requestsTitle = m.theme.PaneTitleRequests.
			Width(innerWidth).
			Align(lipgloss.Center).
			Foreground(m.theme.PaneActiveForeground).
			Render(strings.ToUpper("Requests"))
	}
	if m.focus == focusWorkflows {
		workflowsTitle = m.theme.PaneTitleRequests.
			Width(innerWidth).
			Align(lipgloss.Center).
			Foreground(m.theme.PaneActiveForeground).
			Render(strings.ToUpper("Workflows"))
	}

	listStyle := lipgloss.NewStyle().Width(innerWidth)
	filesView := listStyle.Render(m.fileList.View())
	requestsView := listStyle.Render(m.requestList.View())
	workflowsView := listStyle.Render(m.workflowList.View())
	if m.focus == focusFile {
		filesView = listStyle.
			Foreground(m.theme.PaneBorderFocusFile).
			Render(m.fileList.View())
	}
	if m.focus == focusRequests {
		requestsView = listStyle.
			Foreground(m.theme.PaneBorderFocusRequests).
			Render(m.requestList.View())
	}
	if m.focus == focusWorkflows {
		workflowsView = listStyle.
			Foreground(m.theme.PaneBorderFocusRequests).
			Render(m.workflowList.View())
	}
	if len(m.requestItems) == 0 {
		empty := lipgloss.NewStyle().
			Width(innerWidth).
			Align(lipgloss.Center)
		requestsView = empty.Render(
			m.theme.HeaderValue.Render("No requests parsed"),
		)
	}
	if len(m.workflowItems) == 0 {
		empty := lipgloss.NewStyle().
			Width(innerWidth).
			Align(lipgloss.Center)
		workflowsView = empty.Render(
			m.theme.HeaderValue.Render("No workflows defined"),
		)
	}
	separator := m.theme.PaneDivider.
		Width(innerWidth).
		Render(strings.Repeat("─", innerWidth))

	filesSection := lipgloss.JoinVertical(
		lipgloss.Left,
		filesTitle,
		separator,
		filesView,
	)
	requestsSection := lipgloss.JoinVertical(
		lipgloss.Left,
		requestsTitle,
		separator,
		requestsView,
	)
	workflowsSection := lipgloss.JoinVertical(
		lipgloss.Left,
		workflowsTitle,
		separator,
		workflowsView,
	)

	if m.focus == focusFile {
		highlight := lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(m.theme.PaneBorderFocusFile).
			Padding(0, 1)
		filesSection = highlight.Render(filesSection)
	}
	if m.focus == focusRequests {
		highlight := lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(m.theme.PaneBorderFocusRequests).
			Padding(0, 1)
		requestsSection = highlight.Render(requestsSection)
	}
	if m.focus == focusWorkflows {
		highlight := lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(m.theme.PaneBorderFocusRequests).
			Padding(0, 1)
		workflowsSection = highlight.Render(workflowsSection)
	}
	sections := []string{filesSection, "", requestsSection}
	if len(m.workflowItems) > 0 {
		sections = append(sections, "", workflowsSection)
	}
	content := lipgloss.JoinVertical(lipgloss.Left, sections...)
	if !paneActive {
		content = faintStyle.Render(content)
	}

	gapCount := sidebarSplitPadding
	if len(m.workflowItems) > 0 {
		gapCount++
	}
	contentHeight := m.sidebarFilesHeight + m.sidebarRequestsHeight + gapCount
	if contentHeight < m.paneContentHeight {
		contentHeight = m.paneContentHeight
	}
	frameHeight := style.GetVerticalFrameSize()
	targetHeight := contentHeight + frameHeight
	return style.
		Width(width).
		Height(targetHeight).
		Render(content)
}

func (m Model) renderEditorPane() string {
	style := m.theme.EditorBorder
	content := m.editor.View()
	if m.focus == focusEditor {
		style = style.
			BorderForeground(lipgloss.Color("#B794F6")).
			Bold(true).
			BorderStyle(lipgloss.ThickBorder())
	} else {
		style = style.Faint(true)
		content = lipgloss.NewStyle().Faint(true).Render(content)
	}
	frameHeight := style.GetVerticalFrameSize()
	innerHeight := maxInt(m.editor.Height(), m.paneContentHeight)
	height := innerHeight + frameHeight
	return style.
		Width(m.editor.Width() + 4).
		Height(height).
		Render(content)
}

func (m Model) renderResponsePane() string {
	style := m.theme.ResponseBorder
	active := m.focus == focusResponse
	if active {
		style = style.
			BorderForeground(lipgloss.Color("#6CC4C4")).
			Bold(true).
			BorderStyle(lipgloss.ThickBorder())
	} else {
		style = style.Faint(true)
	}

	var body string
	if m.responseSplit {
		primaryFocused := active && m.responsePaneFocus == responsePanePrimary
		secondaryFocused := active && m.responsePaneFocus == responsePaneSecondary
		if m.responseSplitOrientation == responseSplitHorizontal {
			top := m.renderResponseColumn(responsePanePrimary, primaryFocused)
			bottom := m.renderResponseColumn(responsePaneSecondary, secondaryFocused)
			divider := m.renderResponseDividerHorizontal(top, bottom)
			if divider != "" {
				body = lipgloss.JoinVertical(lipgloss.Left, top, divider, bottom)
			} else {
				body = lipgloss.JoinVertical(lipgloss.Left, top, bottom)
			}
		} else {
			left := m.renderResponseColumn(responsePanePrimary, primaryFocused)
			right := m.renderResponseColumn(responsePaneSecondary, secondaryFocused)
			divider := m.renderResponseDivider(left, right)
			if divider != "" {
				body = lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right)
			} else {
				body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
			}
		}
	} else {
		column := m.renderResponseColumn(responsePanePrimary, active)
		if !active {
			column = lipgloss.NewStyle().Faint(true).Render(column)
		}
		body = column
	}

	width := m.responseContentWidth() + 4
	frameHeight := style.GetVerticalFrameSize()
	height := m.paneContentHeight + frameHeight
	if height < frameHeight {
		height = frameHeight
	}
	return style.
		Width(width).
		Height(height).
		Render(body)
}

func (m Model) renderResponseColumn(id responsePaneID, focused bool) string {
	pane := m.pane(id)
	if pane == nil {
		return ""
	}

	contentWidth := maxInt(pane.viewport.Width, 1)
	contentHeight := maxInt(pane.viewport.Height, 1)

	tabs := m.renderPaneTabs(id, focused)
	tabs = lipgloss.NewStyle().
		MaxWidth(contentWidth).
		Render(tabs)

	searchView := ""
	if m.showSearchPrompt && m.searchTarget == searchTargetResponse && m.searchResponsePane == id {
		searchView = m.renderResponseSearchPrompt(contentWidth)
	}

	var content string
	if pane.activeTab == responseTabHistory {
		content = m.renderHistoryPaneFor(id)
	} else {
		content = pane.viewport.View()
	}
	content = lipgloss.NewStyle().
		MaxWidth(contentWidth).
		MaxHeight(contentHeight).
		Render(content)

	if !focused && m.focus == focusResponse {
		tabs = lipgloss.NewStyle().Faint(true).Render(tabs)
		if searchView != "" {
			searchView = lipgloss.NewStyle().Faint(true).Render(searchView)
		}
		content = lipgloss.NewStyle().Faint(true).Render(content)
	}

	elements := []string{tabs}
	if searchView != "" {
		elements = append(elements, searchView)
	}
	elements = append(elements, content)

	column := lipgloss.JoinVertical(
		lipgloss.Left,
		elements...,
	)
	columnHeight := maxInt(contentHeight+lipgloss.Height(tabs), 1)
	column = lipgloss.NewStyle().
		MaxWidth(contentWidth).
		MaxHeight(columnHeight).
		Render(column)
	return lipgloss.Place(
		contentWidth,
		columnHeight,
		lipgloss.Top,
		lipgloss.Left,
		column,
		lipgloss.WithWhitespaceChars(" "),
	)
}

func (m Model) renderPaneTabs(id responsePaneID, focused bool) string {
	pane := m.pane(id)
	if pane == nil {
		return ""
	}

	tabs := m.availableResponseTabs()
	labels := make([]string, 0, len(tabs))
	for _, tab := range tabs {
		label := m.responseTabLabel(tab)
		if tab == pane.activeTab {
			if focused {
				label = tabIndicatorPrefix + label
			}
			labels = append(labels, m.theme.TabActive.Render(label))
		} else {
			labels = append(labels, m.theme.TabInactive.Render(label))
		}
	}

	mode := "Pinned"
	if pane.followLatest {
		mode = "Live"
	}
	badge := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#A6A1BB")).
		PaddingLeft(2).
		Faint(!focused || m.focus != focusResponse).
		Render(strings.ToUpper(mode))

	row := strings.Join(labels, " ")
	row = lipgloss.JoinHorizontal(lipgloss.Top, row, badge)
	return m.theme.Tabs.Render(row)
}

func (m Model) renderResponseDivider(left, right string) string {
	if !m.responseSplit {
		return ""
	}
	height := maxInt(lipgloss.Height(left), lipgloss.Height(right))
	if height <= 0 {
		height = maxInt(m.paneContentHeight, 1)
	}
	line := strings.Repeat("│\n", height-1) + "│"
	return m.theme.PaneDivider.Render(line)
}

func (m Model) renderResponseDividerHorizontal(top, bottom string) string {
	if !m.responseSplit {
		return ""
	}
	width := maxInt(lipgloss.Width(top), lipgloss.Width(bottom))
	if width <= 0 {
		width = m.responseContentWidth()
	}
	if width <= 0 {
		return ""
	}
	line := strings.Repeat("─", width)
	return m.theme.PaneDivider.Render(line)
}

func (m Model) renderHistoryPaneFor(id responsePaneID) string {
	pane := m.pane(id)
	if pane == nil {
		return ""
	}

	contentWidth := maxInt(pane.viewport.Width, 1)
	contentHeight := maxInt(pane.viewport.Height, 1)

	if len(m.historyEntries) == 0 {
		body := lipgloss.NewStyle().
			MaxWidth(contentWidth).
			MaxHeight(contentHeight).
			Render("No history yet. Execute a request to populate this view.")
		return lipgloss.Place(
			contentWidth,
			contentHeight,
			lipgloss.Top,
			lipgloss.Left,
			body,
			lipgloss.WithWhitespaceChars(" "),
		)
	}

	listView := m.historyList.View()
	listView = lipgloss.NewStyle().
		MaxWidth(contentWidth).
		Render(listView)

	body := layoutHistoryContent(listView, "", contentHeight)
	body = lipgloss.NewStyle().
		MaxWidth(contentWidth).
		MaxHeight(contentHeight).
		Render(body)

	return lipgloss.Place(
		contentWidth,
		contentHeight,
		lipgloss.Top,
		lipgloss.Left,
		body,
		lipgloss.WithWhitespaceChars(" "),
	)
}

func layoutHistoryContent(listView, snippetView string, maxHeight int) string {
	height := maxInt(maxHeight, 1)
	if snippetView == "" {
		return lipgloss.NewStyle().
			MaxHeight(height).
			Render(listView)
	}

	snippet := lipgloss.NewStyle().
		MaxHeight(height).
		Render(snippetView)
	snippetHeight := lipgloss.Height(snippet)
	if snippetHeight >= height {
		return snippet
	}

	listHeight := height - snippetHeight
	if listHeight <= 0 {
		return snippet
	}

	trimmedList := lipgloss.NewStyle().
		MaxHeight(listHeight).
		Render(listView)
	trimmedListHeight := lipgloss.Height(trimmedList)
	if trimmedListHeight == 0 {
		return snippet
	}

	remaining := height - trimmedListHeight
	if remaining <= 0 {
		return trimmedList
	}

	trimmedSnippet := lipgloss.NewStyle().
		MaxHeight(remaining).
		Render(snippet)
	if lipgloss.Height(trimmedSnippet) == 0 {
		return trimmedList
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		trimmedList,
		trimmedSnippet,
	)
}

func (m Model) renderCommandBar() string {
	if m.showSearchPrompt {
		if m.searchTarget == searchTargetResponse {
			return m.renderResponseSearchInfo()
		}
		return m.renderSearchPrompt()
	}

	type hint struct {
		key   string
		label string
	}
	segments := []hint{
		{key: "Tab", label: "Focus"},
		{key: "Enter", label: "Run"},
		{key: "Ctrl+Enter", label: "Send"},
		{key: "Ctrl+S", label: "Save"},
		{key: "Ctrl+N", label: "New File"},
		{key: "Ctrl+O", label: "Open"},
		{key: "Ctrl+Q", label: "Quit"},
		{key: "?", label: "Help"},
	}

	var rendered []string
	for idx, seg := range segments {
		style := m.theme.CommandSegment(idx)
		button := renderCommandButton(seg.key, seg.label, style)
		rendered = append(rendered, button)
	}

	if len(rendered) == 0 {
		return m.theme.CommandBar.Render("")
	}
	divider := m.theme.CommandDivider.Render(" ")
	row := rendered[0]
	for i := 1; i < len(rendered); i++ {
		row = lipgloss.JoinHorizontal(
			lipgloss.Top,
			row,
			divider,
			rendered[i],
		)
	}
	return m.theme.CommandBar.Render(row)
}

func (m Model) renderSearchPrompt() string {
	mode := "literal"
	if m.searchIsRegex {
		mode = "regex"
	}
	m.searchInput.Width = 0
	label := lipgloss.NewStyle().Bold(true).Render("Search ")
	input := m.searchInput.View()
	modeBadge := lipgloss.NewStyle().
		Faint(true).
		PaddingLeft(2).
		Render(strings.ToUpper(mode))
	hints := lipgloss.NewStyle().
		Faint(true).
		PaddingLeft(2).
		Render("Enter confirm  Esc cancel  Ctrl+R toggle regex")
	row := lipgloss.JoinHorizontal(
		lipgloss.Top,
		label,
		input,
		modeBadge,
		hints,
	)
	return m.theme.CommandBar.Render(row)
}

func (m Model) renderResponseSearchPrompt(width int) string {
	if width <= 0 {
		width = defaultResponseViewportWidth
	}
	mode := "literal"
	if m.searchIsRegex {
		mode = "regex"
	}
	label := lipgloss.NewStyle().Bold(true).Render("Search ")
	modeBadge := lipgloss.NewStyle().
		Faint(true).
		PaddingLeft(1).
		Render(strings.ToUpper(mode))
	reserved := lipgloss.Width(label) + lipgloss.Width(modeBadge) + 2
	inputWidth := width - reserved
	if inputWidth < 4 {
		inputWidth = maxInt(4, width-8)
	}
	m.searchInput.Width = inputWidth
	input := lipgloss.NewStyle().MaxWidth(inputWidth).Render(m.searchInput.View())
	row := lipgloss.JoinHorizontal(
		lipgloss.Top,
		label,
		input,
		modeBadge,
	)
	return m.theme.CommandBar.
		Width(width).
		Render(row)
}

func (m Model) renderResponseSearchInfo() string {
	mode := "literal"
	if m.searchIsRegex {
		mode = "regex"
	}
	label := lipgloss.NewStyle().Bold(true).Render("Response Search ")
	modeBadge := lipgloss.NewStyle().
		Faint(true).
		PaddingLeft(1).
		Render(strings.ToUpper(mode))
	hints := lipgloss.NewStyle().
		Faint(true).
		PaddingLeft(1).
		Render("Enter confirm  Esc cancel  Ctrl+R toggle regex")
	row := lipgloss.JoinHorizontal(
		lipgloss.Top,
		label,
		modeBadge,
		hints,
	)
	return m.theme.CommandBar.Render(row)
}

func renderCommandButton(
	key string,
	label string,
	palette theme.CommandSegmentStyle,
) string {
	keyColor := palette.Key
	if keyColor == "" {
		keyColor = lipgloss.Color("#FFFFFF")
	}
	textColor := palette.Text
	if textColor == "" {
		textColor = lipgloss.Color("#E5E1FF")
	}

	button := lipgloss.NewStyle().
		Foreground(textColor).
		Padding(0, 2).
		Bold(true)
	if palette.Background != "" {
		button = button.Background(palette.Background)
	}

	keyStyle := lipgloss.NewStyle().
		Foreground(keyColor).
		Bold(true)
	labelStyle := lipgloss.NewStyle().
		Foreground(textColor).
		Bold(false)
	if palette.Background != "" {
		keyStyle = keyStyle.Background(palette.Background)
		labelStyle = labelStyle.Background(palette.Background)
	}
	keyText := keyStyle.Render(key)
	labelText := labelStyle.Render(" " + label)
	content := lipgloss.JoinHorizontal(lipgloss.Center, keyText, labelText)
	return button.Render(content)
}

func (m Model) renderHeader() string {
	workspace := filepath.Base(m.workspaceRoot)
	if workspace == "" {
		workspace = "."
	}
	env := m.cfg.EnvironmentName
	if env == "" {
		env = "default"
	}
	request := requestBaseTitle(m.currentRequest)
	if strings.TrimSpace(request) == "" {
		request = strings.TrimSpace(m.activeRequestTitle)
		if request == "" {
			request = "—"
		}
	}

	type segment struct {
		label string
		value string
	}

	segmentsData := []segment{
		{label: "Workspace", value: workspace},
		{label: "Env", value: env},
		{label: "Requests", value: fmt.Sprintf("%d", len(m.requestItems))},
		{label: "Active", value: request},
	}

	if summary, ok := m.headerTestSummary(); ok {
		segmentsData = append(segmentsData, segment{label: "Tests", value: summary})
	}

	segments := make([]string, 0, len(segmentsData)+1)
	brandContent := strings.ToUpper("RESTERM")
	brandSegment := m.theme.HeaderBrand.Render(" " + brandContent + " ")
	segments = append(segments, brandSegment)
	for i, seg := range segmentsData {
		segments = append(segments, m.renderHeaderButton(i, seg.label, seg.value))
	}

	separator := m.theme.HeaderSeparator.Render(" ")
	joined := segments[0]
	for i := 1; i < len(segments); i++ {
		joined = lipgloss.JoinHorizontal(
			lipgloss.Top,
			joined,
			separator,
			segments[i],
		)
	}

	width := maxInt(m.width, lipgloss.Width(joined))
	return m.theme.Header.Width(width).Render(joined)
}

func (m Model) renderHeaderButton(idx int, label, value string) string {
	palette := m.theme.HeaderSegment(idx)
	labelText := strings.ToUpper(strings.TrimSpace(label))
	if labelText == "" {
		labelText = "—"
	}
	valueText := strings.TrimSpace(value)
	if strings.HasPrefix(valueText, tabIndicatorPrefix) {
		valueText = strings.TrimSpace(
			strings.TrimPrefix(valueText, tabIndicatorPrefix),
		)
	}
	if valueText == "" {
		valueText = "—"
	}

	fg := palette.Foreground
	if fg == "" {
		fg = lipgloss.Color("#F5F2FF")
	}
	accent := palette.Accent
	if accent == "" {
		accent = fg
	}
	border := palette.Border
	if border == "" {
		border = accent
	}

	borderSpec := lipgloss.Border{
		Top:         "",
		Bottom:      "",
		Left:        "┃",
		Right:       "┃",
		TopLeft:     "",
		TopRight:    "",
		BottomLeft:  "",
		BottomRight: "",
	}

	button := lipgloss.NewStyle().
		BorderStyle(borderSpec).
		BorderForeground(border).
		Foreground(fg).
		Padding(0, 1)
	if palette.Background != "" {
		button = button.Background(palette.Background)
	}

	labelStyle := lipgloss.NewStyle().
		Foreground(accent).
		Bold(true)
	if palette.Background != "" {
		labelStyle = labelStyle.Background(palette.Background)
	}
	valueStyle := lipgloss.NewStyle().
		Foreground(fg).
		Bold(true)
	if palette.Background != "" {
		valueStyle = valueStyle.Background(palette.Background)
	}
	colonStyle := lipgloss.NewStyle().
		Foreground(accent)
	if palette.Background != "" {
		colonStyle = colonStyle.Background(palette.Background)
	}

	content := lipgloss.JoinHorizontal(lipgloss.Top,
		labelStyle.Render(labelText),
		colonStyle.Render(": "),
		valueStyle.Render(valueText),
	)

	return button.Render(content)
}

func (m Model) headerTestSummary() (string, bool) {
	if m.scriptError != nil {
		return "error", true
	}
	if len(m.testResults) == 0 {
		return "", false
	}
	failures := 0
	for _, result := range m.testResults {
		if !result.Passed {
			failures++
		}
	}
	if failures > 0 {
		return fmt.Sprintf("%d fail", failures), true
	}
	return fmt.Sprintf("%d pass", len(m.testResults)), true
}

func (m Model) renderStatusBar() string {
	statusText := m.statusMessage.text
	if statusText == "" {
		if m.dirty {
			statusText = "Unsaved changes"
		} else {
			statusText = "Ready"
		}
	}

	const sep = "    "
	sepWidth := lipgloss.Width(sep)

	segments := make([]string, 0, 4)
	if m.cfg.EnvironmentName != "" {
		segments = append(segments, fmt.Sprintf("Env: %s", m.cfg.EnvironmentName))
	}
	if m.currentFile != "" {
		segments = append(segments, filepath.Base(m.currentFile))
	}
	segments = append(segments, fmt.Sprintf("Focus: %s", m.focusLabel()))
	if m.focus == focusEditor {
		mode := "VIEW"
		if m.editorInsertMode {
			mode = "INSERT"
		}
		segments = append(segments, fmt.Sprintf("Mode: %s", mode))
	}

	staticText := strings.Join(segments, sep)
	maxContentWidth := maxInt(m.width-2, 1)
	messageText := statusText

	if staticText != "" {
		staticWidth := lipgloss.Width(staticText)
		if staticWidth > maxContentWidth {
			staticText = truncateToWidth(staticText, maxContentWidth)
			messageText = ""
		} else {
			available := maxContentWidth - staticWidth
			if messageText != "" {
				if available > sepWidth {
					available -= sepWidth
					messageText = truncateToWidth(messageText, available)
				} else {
					messageText = ""
				}
			}
		}
	} else {
		messageText = truncateToWidth(messageText, maxContentWidth)
	}

	var builder strings.Builder
	if messageText != "" {
		builder.WriteString(messageText)
	}
	if staticText != "" {
		if builder.Len() > 0 {
			builder.WriteString(sep)
		}
		builder.WriteString(staticText)
	}
	combined := builder.String()
	if combined == "" {
		combined = truncateToWidth(statusText, maxContentWidth)
	}

	return m.theme.StatusBar.Render(combined)
}

func truncateStatus(text string, width int) string {
	if width <= 0 {
		return text
	}
	maxWidth := maxInt(width-2, 1)
	return truncateToWidth(text, maxWidth)
}

func truncateToWidth(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= maxWidth {
		return text
	}
	ellipsisWidth := lipgloss.Width("…")
	if maxWidth <= ellipsisWidth {
		return "…"
	}
	available := maxWidth - ellipsisWidth
	var (
		builder       strings.Builder
		consumedWidth int
	)
	for _, r := range text {
		runeWidth := lipgloss.Width(string(r))
		if consumedWidth+runeWidth > available {
			break
		}
		builder.WriteRune(r)
		consumedWidth += runeWidth
	}
	trimmed := strings.TrimSpace(builder.String())
	if trimmed == "" {
		trimmed = builder.String()
	}
	if trimmed == "" {
		return "…"
	}
	return trimmed + "…"
}

func (m Model) renderHistoryPreviewModal() string {
	width := minInt(m.width-6, 100)
	if width < 48 {
		candidate := m.width - 4
		if candidate > 0 {
			width = maxInt(36, candidate)
		} else {
			width = 48
		}
	}
	contentWidth := maxInt(width-4, 32)
	title := strings.TrimSpace(m.historyPreviewTitle)
	if title == "" {
		title = "History Entry"
	}
	body := m.historyPreviewContent
	if strings.TrimSpace(body) == "" {
		body = "{}"
	}
	viewWidth := maxInt(contentWidth-4, 20)
	bodyHeight := maxInt(min(m.height-12, 30), 8)
	if bodyHeight > m.height-6 {
		bodyHeight = maxInt(m.height-6, 8)
	}
	if bodyHeight <= 0 {
		bodyHeight = 8
	}
	if viewWidth <= 0 {
		viewWidth = 20
	}

	var bodyView string
	if vp := m.historyPreviewViewport; vp != nil {
		wrapped := wrapPreformattedContent(body, viewWidth)
		vp.SetContent(wrapped)
		vp.Width = viewWidth
		vp.Height = bodyHeight
		bodyView = lipgloss.NewStyle().
			Padding(0, 2).
			Width(contentWidth).
			Render(vp.View())
	} else {
		bodyView = lipgloss.NewStyle().
			Padding(0, 2).
			Width(contentWidth).
			Render(wrapPreformattedContent(body, viewWidth))
	}

	headerView := m.theme.HeaderTitle.
		Width(contentWidth).
		Align(lipgloss.Center).
		Render(title)
	instructions := fmt.Sprintf(
		"%s / %s Close",
		m.theme.CommandBarHint.Render("Esc"),
		m.theme.CommandBarHint.Render("Enter"),
	)
	instructionsView := m.theme.HeaderValue.
		Padding(0, 2).
		Render(instructions)

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		headerView,
		"",
		bodyView,
		"",
		instructionsView,
	)
	box := m.theme.BrowserBorder.Width(width).Render(content)
	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		box,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("#1A1823")),
	)
}

func (m Model) renderErrorModal() string {
	width := m.width - 10
	if width > 72 {
		width = 72
	}
	if width < 32 {
		candidate := m.width - 4
		if candidate > 0 {
			width = maxInt(24, candidate)
		} else {
			width = 48
		}
	}
	contentWidth := maxInt(width-4, 24)
	message := strings.TrimSpace(m.errorModalMessage)
	if message == "" {
		message = "An unexpected error occurred."
	}
	wrapped := wrapToWidth(message, contentWidth)
	messageView := m.theme.Error.Render(wrapped)
	title := m.theme.HeaderTitle.
		Width(contentWidth).
		Align(lipgloss.Center).
		Render("Error")
	instructions := fmt.Sprintf(
		"%s / %s Dismiss",
		m.theme.CommandBarHint.Render("Esc"),
		m.theme.CommandBarHint.Render("Enter"),
	)
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		messageView,
		"",
		instructions,
	)
	boxStyle := m.theme.BrowserBorder.Width(width)
	box := boxStyle.Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("#1A1823")),
	)
}

func (m Model) renderEnvironmentModal() string {
	width := minInt(m.width-10, 48)
	if width < 24 {
		width = 24
	}
	commands := fmt.Sprintf(
		"%s Select    %s Cancel",
		m.theme.CommandBarHint.Render("Enter"),
		m.theme.CommandBarHint.Render("Esc"),
	)
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		m.envList.View(),
		"",
		commands,
	)
	box := m.theme.BrowserBorder.Width(width).Render(content)
	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		box,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("#1A1823")),
	)
}

func (m Model) renderHelpOverlay() string {
	width := minInt(m.width-10, 80)
	if width < 32 {
		width = 32
	}
	header := func(text string, align lipgloss.Position) string {
		return m.theme.HeaderTitle.
			Width(width - 4).
			Align(align).
			Render(text)
	}

	rows := []string{
		header("Key Bindings", lipgloss.Center),
		"",
		helpRow(m, "Tab", "Cycle focus"),
		helpRow(m, "Shift+Tab", "Reverse focus"),
		helpRow(m, "Enter", "Run selected request"),
		helpRow(m, "Space", "Preview selected request"),
		helpRow(m, "Ctrl+Enter", "Send active request"),
		helpRow(m, "Ctrl+S", "Save current file"),
		helpRow(m, "Ctrl+N", "Create request file"),
		helpRow(m, "Ctrl+O", "Open file or folder"),
		helpRow(m, "Ctrl+Shift+O", "Refresh workspace"),
		helpRow(m, "Ctrl+V / Ctrl+U", "Split response vertically / horizontally"),
		helpRow(m, "Ctrl+Shift+V", "Pin or unpin focused response pane"),
		helpRow(m, "Ctrl+F, ←/→", "Send future responses to selected pane"),
		helpRow(m, "Ctrl+G", "Show globals summary"),
		helpRow(m, "Ctrl+Shift+G", "Clear globals for environment"),
		helpRow(m, "Ctrl+E", "Environment selector"),
		helpRow(m, "gk / gj", "Adjust files/requests split"),
		helpRow(m, "gh / gl", "Adjust editor/response width"),
		helpRow(m, "gr / gi / gp", "Focus requests / editor / response"),
		helpRow(m, "Ctrl+T", "Temporary document"),
		helpRow(m, "Ctrl+P", "Reparse document"),
		helpRow(m, "Ctrl+Q", "Quit (Ctrl+D also works)"),
		helpRow(m, "?", "Toggle this help"),
		"",
		header("Editor motions", lipgloss.Left),
		helpRow(m, "h / j / k / l", "Move left / down / up / right"),
		helpRow(m, "w / b / e", "Next word / previous word / word end"),
		helpRow(m, "0 / ^ / $", "Line start / first non-blank / line end"),
		helpRow(m, "gg / G", "Top / bottom of buffer"),
		helpRow(m, "Ctrl+f / Ctrl+b", "Page down / up (Ctrl+d / Ctrl+u half-page)"),
		helpRow(m, "v / V / y", "Visual select (char / line) / yank selection"),
		helpRow(m, "d + motion", "Delete via Vim motions (dw, db, dk, dgg, dG)"),
		helpRow(m, "dd / D / x / c", "Delete line / to end / char / change line"),
		helpRow(m, "a", "Append after cursor (enter insert mode)"),
		helpRow(m, "p / P", "Paste after / before cursor"),
		helpRow(m, "f / t / T", "Find character (forward / till / backward)"),
		helpRow(m, "u / Ctrl+r", "Undo / redo last edit"),
		"",
		header("Search", lipgloss.Left),
		helpRow(m, "Shift+F", "Open search prompt (Ctrl+R toggles regex)"),
		helpRow(m, "n / p", "Next / previous match (wraps around)"),
		"",
		m.theme.HeaderValue.Render("Press Esc to close this help"),
	}
	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	box := m.theme.BrowserBorder.Width(width).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("#1A1823")),
	)
}

func (m Model) renderNewFileModal() string {
	width := minInt(m.width-10, 60)
	if width < 36 {
		width = 36
	}
	inputView := lipgloss.NewStyle().
		Width(width - 8).
		Render(m.newFileInput.View())

	var extLabels []string
	for idx, ext := range newFileExtensions {
		label := fmt.Sprintf("[%s]", ext)
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("#4D4663")).Bold(false)
		if idx == m.newFileExtIndex {
			style = m.theme.CommandBarHint.Bold(true)
		}
		extLabels = append(extLabels, style.Render(label))
	}

	enter := m.theme.CommandBarHint.Render("Enter")
	esc := m.theme.CommandBarHint.Render("Esc")
	switchHint := m.theme.CommandBarHint.Render("Tab/←/→")
	instructions := fmt.Sprintf(
		"%s Create    %s Cancel    %s Switch",
		enter,
		esc,
		switchHint,
	)

	lines := []string{
		m.theme.HeaderTitle.
			Width(width - 4).
			Align(lipgloss.Center).
			Render("New Request File"),
		"",
		lipgloss.NewStyle().
			Padding(0, 2).
			Render(inputView),
		lipgloss.NewStyle().
			Padding(0, 2).
			Render("Extension: " + strings.Join(extLabels, "  ")),
	}
	if m.newFileError != "" {
		errorLine := m.theme.Error.
			Padding(0, 2).
			Render(m.newFileError)
		lines = append(lines, "", errorLine)
	}
	headerValue := m.theme.HeaderValue.
		Padding(0, 2).
		Render(instructions)
	lines = append(lines, "", headerValue)

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	box := m.theme.BrowserBorder.Width(width).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("#1A1823")),
	)
}

func (m Model) renderOpenModal() string {
	width := minInt(m.width-10, 60)
	if width < 36 {
		width = 36
	}
	inputView := lipgloss.NewStyle().
		Width(width - 8).
		Render(m.openPathInput.View())

	enter := m.theme.CommandBarHint.Render("Enter")
	esc := m.theme.CommandBarHint.Render("Esc")
	info := fmt.Sprintf("%s Open    %s Cancel", enter, esc)

	lines := []string{
		m.theme.HeaderTitle.
			Width(width - 4).
			Align(lipgloss.Center).
			Render("Open File or Workspace"),
		"",
		lipgloss.NewStyle().
			Padding(0, 2).
			Render("Enter a path to a .http/.rest file or a folder"),
		lipgloss.NewStyle().
			Padding(0, 2).
			Render(inputView),
	}
	if m.openPathError != "" {
		errorLine := m.theme.Error.
			Padding(0, 2).
			Render(m.openPathError)
		lines = append(lines, "", errorLine)
	}
	headerInfo := m.theme.HeaderValue.
		Padding(0, 2).
		Render(info)
	lines = append(lines, "", headerInfo)

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	box := m.theme.BrowserBorder.Width(width).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("#1A1823")),
	)
}

func helpRow(m Model, key, description string) string {
	keyStyled := m.theme.HeaderTitle.
		Width(18).
		Align(lipgloss.Left).
		Render(key)
	descStyled := m.theme.HeaderValue.
		PaddingLeft(2).
		Render(description)
	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		keyStyled,
		descStyled,
	)
}

func (m Model) focusLabel() string {
	switch m.focus {
	case focusFile:
		return "Files"
	case focusRequests:
		return "Requests"
	case focusWorkflows:
		return "Workflows"
	case focusEditor:
		return "Editor"
	case focusResponse:
		return "Response"
	default:
		return ""
	}
}

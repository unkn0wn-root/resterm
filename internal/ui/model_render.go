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

	if m.showOpenModal {
		return m.renderWithinAppFrame(m.renderOpenModal())
	}

	if m.showNewFileModal {
		return m.renderWithinAppFrame(m.renderNewFileModal())
	}

	filePane := m.renderFilePane()
	editorPane := m.renderEditorPane()
	responsePane := m.renderResponsePane()

	panes := lipgloss.JoinHorizontal(lipgloss.Top, filePane, editorPane, responsePane)
	body := lipgloss.JoinVertical(lipgloss.Left, m.renderCommandBar(), panes, m.renderStatusBar())
	header := m.renderHeader()
	base := lipgloss.JoinVertical(lipgloss.Left, header, body)
	if m.showHelp {
		return m.renderWithinAppFrame(m.renderHelpOverlay())
	}
	if m.showEnvSelector {
		content := lipgloss.JoinVertical(lipgloss.Left, header, m.renderEnvironmentSelector(), body)
		return m.renderWithinAppFrame(content)
	}
	return m.renderWithinAppFrame(base)
}

func (m Model) renderWithinAppFrame(content string) string {
	innerWidth := maxInt(m.width, lipgloss.Width(content))
	innerHeight := maxInt(m.height, lipgloss.Height(content))

	if innerWidth > 0 {
		content = lipgloss.Place(innerWidth, lipgloss.Height(content), lipgloss.Top, lipgloss.Left, content,
			lipgloss.WithWhitespaceChars(" "))
	}
	if innerWidth > 0 && innerHeight > lipgloss.Height(content) {
		content = lipgloss.Place(innerWidth, innerHeight, lipgloss.Top, lipgloss.Left, content,
			lipgloss.WithWhitespaceChars(" "))
	}

	framed := m.theme.AppFrame.Render(content)

	frameWidth := maxInt(m.frameWidth, lipgloss.Width(framed))
	frameHeight := maxInt(m.frameHeight, lipgloss.Height(framed))

	if frameWidth > lipgloss.Width(framed) || frameHeight > lipgloss.Height(framed) {
		framed = lipgloss.Place(frameWidth, frameHeight, lipgloss.Top, lipgloss.Left, framed,
			lipgloss.WithWhitespaceChars(" "))
	}

	return framed
}

func (m Model) renderFilePane() string {
	style := m.theme.BrowserBorder
	paneActive := m.focus == focusFile || m.focus == focusRequests
	switch m.focus {
	case focusFile:
		style = style.Copy().
			BorderForeground(m.theme.PaneBorderFocusFile).
			Bold(true).
			BorderStyle(lipgloss.ThickBorder())
	case focusRequests:
		style = style.Copy().
			BorderForeground(m.theme.PaneBorderFocusRequests).
			Bold(true).
			BorderStyle(lipgloss.ThickBorder())
	}

	faintStyle := lipgloss.NewStyle().Faint(true)
	if !paneActive {
		style = style.Copy().Faint(true)
	}

	width := m.fileList.Width() + 4
	innerWidth := maxInt(1, width-4)
	titleBase := m.theme.PaneTitle.Copy().Width(innerWidth).Align(lipgloss.Center)
	filesTitle := titleBase.Render(strings.ToUpper("Files"))
	requestsTitle := titleBase.Render(strings.ToUpper("Requests"))
	if m.focus == focusFile {
		filesTitle = m.theme.PaneTitleFile.Copy().
			Width(innerWidth).
			Align(lipgloss.Center).
			Foreground(m.theme.PaneActiveForeground).
			Render(strings.ToUpper("Files"))
	}
	if m.focus == focusRequests {
		requestsTitle = m.theme.PaneTitleRequests.Copy().
			Width(innerWidth).
			Align(lipgloss.Center).
			Foreground(m.theme.PaneActiveForeground).
			Render(strings.ToUpper("Requests"))
	}

	listStyle := lipgloss.NewStyle().Width(innerWidth)
	filesView := listStyle.Render(m.fileList.View())
	requestsView := listStyle.Render(m.requestList.View())
	if m.focus == focusFile {
		filesView = listStyle.Copy().Foreground(m.theme.PaneBorderFocusFile).Render(m.fileList.View())
	}
	if m.focus == focusRequests {
		requestsView = listStyle.Copy().Foreground(m.theme.PaneBorderFocusRequests).Render(m.requestList.View())
	}
	if len(m.requestItems) == 0 {
		requestsView = lipgloss.NewStyle().Width(innerWidth).Align(lipgloss.Center).Render(m.theme.HeaderValue.Render("No requests parsed"))
	}
	separator := m.theme.PaneDivider.Copy().Width(innerWidth).Render(strings.Repeat("─", innerWidth))

	filesSection := lipgloss.JoinVertical(lipgloss.Left, filesTitle, separator, filesView)
	requestsSection := lipgloss.JoinVertical(lipgloss.Left, requestsTitle, separator, requestsView)

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
	content := lipgloss.JoinVertical(lipgloss.Left, filesSection, "", requestsSection)
	if !paneActive {
		content = faintStyle.Render(content)
	}

	minHeight := m.fileList.Height() + m.requestList.Height() + 8
	targetHeight := m.responseViewport.Height + 6
	if targetHeight < minHeight {
		targetHeight = minHeight
	}
	return style.Width(width).Height(targetHeight).Render(content)
}

func (m Model) renderEditorPane() string {
	style := m.theme.EditorBorder
	content := m.editor.View()
	if m.focus == focusEditor {
		style = style.Copy().
			BorderForeground(lipgloss.Color("#B794F6")).
			Bold(true).
			BorderStyle(lipgloss.ThickBorder())
	} else {
		style = style.Copy().Faint(true)
		content = lipgloss.NewStyle().Faint(true).Render(content)
	}
	return style.Width(m.editor.Width() + 4).Height(m.editor.Height() + 4).Render(content)
}

func (m Model) renderResponsePane() string {
	style := m.theme.ResponseBorder
	active := m.focus == focusResponse
	faintStyle := lipgloss.NewStyle().Faint(true)
	if m.focus == focusResponse {
		style = style.Copy().
			BorderForeground(lipgloss.Color("#6CC4C4")).
			Bold(true).
			BorderStyle(lipgloss.ThickBorder())
	} else {
		style = style.Copy().Faint(true)
	}

	tabBar := m.renderTabs()
	var content string
	if m.activeTab == responseTabHistory {
		content = m.renderHistoryPane()
	} else {
		content = m.responseViewport.View()
	}
	body := lipgloss.JoinVertical(lipgloss.Left, tabBar, content)
	if !active {
		body = faintStyle.Render(body)
	}
	width := m.responseViewport.Width + 4
	height := m.responseViewport.Height + 6
	return style.Width(width).Height(height).Render(body)
}

func (m Model) renderTabs() string {
	var rendered []string
	for idx, tab := range m.responseTabs {
		if idx == int(m.activeTab) {
			label := tabIndicatorPrefix + tab
			rendered = append(rendered, m.theme.TabActive.Render(label))
		} else {
			rendered = append(rendered, m.theme.TabInactive.Render(tab))
		}
	}
	return m.theme.Tabs.Render(strings.Join(rendered, " "))
}

func (m Model) renderHistoryPane() string {
	if len(m.historyEntries) == 0 {
		return "No history yet. Execute a request to populate this view."
	}
	view := m.historyList.View()
	if item, ok := m.historyList.SelectedItem().(historyItem); ok {
		snippet := strings.TrimSpace(stripANSIEscape(item.entry.BodySnippet))
		wrapWidth := m.responseViewport.Width
		if wrapWidth <= 0 {
			wrapWidth = m.width - 6
		}
		if wrapWidth <= 0 {
			wrapWidth = 80
		}
		formatted := formatHistorySnippet(snippet, wrapWidth)
		if formatted != "" {
			snippetStyle := lipgloss.NewStyle().MarginTop(1).Foreground(lipgloss.Color("#A6A1BB"))
			view = lipgloss.JoinVertical(lipgloss.Left, view, snippetStyle.Render(formatted))
		}
	}
	return view
}

func (m Model) renderCommandBar() string {
	if m.showSearchPrompt {
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
		row = lipgloss.JoinHorizontal(lipgloss.Top, row, divider, rendered[i])
	}
	return m.theme.CommandBar.Render(row)
}

func (m Model) renderSearchPrompt() string {
	mode := "literal"
	if m.searchIsRegex {
		mode = "regex"
	}
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
	row := lipgloss.JoinHorizontal(lipgloss.Top, label, input, modeBadge, hints)
	return m.theme.CommandBar.Render(row)
}

func renderCommandButton(key, label string, palette theme.CommandSegmentStyle) string {
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

	keyStyle := lipgloss.NewStyle().Foreground(keyColor).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(textColor).Bold(false)
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
	request := m.activeRequestTitle
	if strings.TrimSpace(request) == "" {
		request = "—"
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
		joined = lipgloss.JoinHorizontal(lipgloss.Top, joined, separator, segments[i])
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
		valueText = strings.TrimSpace(strings.TrimPrefix(valueText, tabIndicatorPrefix))
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
	builder := strings.Builder{}
	if m.statusMessage.text != "" {
		builder.WriteString(m.statusMessage.text)
	} else if m.dirty {
		builder.WriteString("Unsaved changes")
	} else {
		builder.WriteString("Ready")
	}

	if m.cfg.EnvironmentName != "" {
		builder.WriteString("    ")
		builder.WriteString(fmt.Sprintf("Env: %s", m.cfg.EnvironmentName))
	}
	if m.currentFile != "" {
		builder.WriteString("    ")
		builder.WriteString(filepath.Base(m.currentFile))
	}
	builder.WriteString("    ")
	builder.WriteString(fmt.Sprintf("Focus: %s", m.focusLabel()))
	if m.focus == focusEditor {
		builder.WriteString("    ")
		mode := "VIEW"
		if m.editorInsertMode {
			mode = "INSERT"
		}
		builder.WriteString(fmt.Sprintf("Mode: %s", mode))
	}

	return m.theme.StatusBar.Render(builder.String())
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
	messageView := m.theme.Error.Copy().Render(wrapped)
	title := m.theme.HeaderTitle.Width(contentWidth).Align(lipgloss.Center).Render("Error")
	instructions := fmt.Sprintf("%s / %s Dismiss", m.theme.CommandBarHint.Render("Esc"), m.theme.CommandBarHint.Render("Enter"))
	content := lipgloss.JoinVertical(lipgloss.Left, title, "", messageView, "", instructions)
	boxStyle := m.theme.BrowserBorder.Copy().
		Width(width)
	box := boxStyle.Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("#1A1823")),
	)
}

func (m Model) renderEnvironmentSelector() string {
	width := minInt(m.width-10, 48)
	if width < 24 {
		width = 24
	}
	content := lipgloss.JoinVertical(lipgloss.Left,
		m.envList.View(),
		"",
		fmt.Sprintf("%s Select    %s Cancel", m.theme.CommandBarHint.Render("Enter"), m.theme.CommandBarHint.Render("Esc")),
	)
	box := m.theme.BrowserBorder.Copy().Width(width).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("#1A1823")),
	)
}

func (m Model) renderHelpOverlay() string {
	width := minInt(m.width-10, 64)
	if width < 32 {
		width = 32
	}
	rows := []string{
		m.theme.HeaderTitle.Width(width - 4).Align(lipgloss.Center).Render("Key Bindings"),
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
		helpRow(m, "Ctrl+E", "Environment selector"),
		helpRow(m, "gk / gj", "Adjust files/requests split"),
		helpRow(m, "gh / gl", "Adjust editor/response width"),
		helpRow(m, "Ctrl+R", "Reparse document"),
		helpRow(m, "Ctrl+Q", "Quit (Ctrl+D also works)"),
		helpRow(m, "?", "Toggle this help"),
		"",
		m.theme.HeaderTitle.Width(width - 4).Align(lipgloss.Left).Render("Editor motions"),
		helpRow(m, "h / j / k / l", "Move left / down / up / right"),
		helpRow(m, "w / b / e", "Next word / previous word / word end"),
		helpRow(m, "0 / ^ / $", "Line start / first non-blank / line end"),
		helpRow(m, "gg / G", "Top / bottom of buffer"),
		helpRow(m, "Ctrl+f / Ctrl+b", "Page down / up (Ctrl+d / Ctrl+u half-page)"),
		helpRow(m, "v / y", "Visual select / yank selection"),
		"",
		m.theme.HeaderTitle.Width(width - 4).Align(lipgloss.Left).Render("Search"),
		helpRow(m, "Shift+F", "Open search prompt (Ctrl+R toggles regex)"),
		helpRow(m, "n", "Next match (wraps around)"),
		"",
		m.theme.HeaderValue.Render("Press Esc to close this help"),
	}
	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	box := m.theme.BrowserBorder.Copy().Width(width).Render(content)
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
	inputView := lipgloss.NewStyle().Width(width - 8).Render(m.newFileInput.View())

	var extLabels []string
	for idx, ext := range newFileExtensions {
		label := fmt.Sprintf("[%s]", ext)
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("#4D4663")).Bold(false)
		if idx == m.newFileExtIndex {
			style = m.theme.CommandBarHint.Copy().Bold(true)
		}
		extLabels = append(extLabels, style.Render(label))
	}

	enter := m.theme.CommandBarHint.Render("Enter")
	esc := m.theme.CommandBarHint.Render("Esc")
	switchHint := m.theme.CommandBarHint.Render("Tab/←/→")
	instructions := fmt.Sprintf("%s Create    %s Cancel    %s Switch", enter, esc, switchHint)

	lines := []string{
		m.theme.HeaderTitle.Width(width - 4).Align(lipgloss.Center).Render("New Request File"),
		"",
		lipgloss.NewStyle().Padding(0, 2).Render(inputView),
		lipgloss.NewStyle().Padding(0, 2).Render("Extension: " + strings.Join(extLabels, "  ")),
	}
	if m.newFileError != "" {
		errorLine := m.theme.Error.Copy().Padding(0, 2).Render(m.newFileError)
		lines = append(lines, "", errorLine)
	}
	lines = append(lines, "", m.theme.HeaderValue.Copy().Padding(0, 2).Render(instructions))

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	box := m.theme.BrowserBorder.Copy().Width(width).Render(content)
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
	inputView := lipgloss.NewStyle().Width(width - 8).Render(m.openPathInput.View())

	enter := m.theme.CommandBarHint.Render("Enter")
	esc := m.theme.CommandBarHint.Render("Esc")
	info := fmt.Sprintf("%s Open    %s Cancel", enter, esc)

	lines := []string{
		m.theme.HeaderTitle.Width(width - 4).Align(lipgloss.Center).Render("Open File or Workspace"),
		"",
		lipgloss.NewStyle().Padding(0, 2).Render("Enter a path to a .http/.rest file or a folder"),
		lipgloss.NewStyle().Padding(0, 2).Render(inputView),
	}
	if m.openPathError != "" {
		errorLine := m.theme.Error.Copy().Padding(0, 2).Render(m.openPathError)
		lines = append(lines, "", errorLine)
	}
	lines = append(lines, "", m.theme.HeaderValue.Copy().Padding(0, 2).Render(info))

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	box := m.theme.BrowserBorder.Copy().Width(width).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("#1A1823")),
	)
}

func helpRow(m Model, key, description string) string {
	keyStyled := m.theme.HeaderTitle.Render(fmt.Sprintf("%-12s", key))
	descStyled := m.theme.HeaderValue.Render(description)
	return fmt.Sprintf("%s %s", keyStyled, descStyled)
}

func (m Model) focusLabel() string {
	switch m.focus {
	case focusFile:
		return "Files"
	case focusRequests:
		return "Requests"
	case focusEditor:
		return "Editor"
	case focusResponse:
		return "Response"
	default:
		return ""
	}
}

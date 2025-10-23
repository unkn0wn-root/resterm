package ui

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/theme"
)

const statusBarLeftMaxRatio = 0.7

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
	fileWidth := lipgloss.Width(filePane)
	editorPane := m.renderEditorPane()
	editorWidth := lipgloss.Width(editorPane)

	var panes string
	if m.mainSplitOrientation == mainSplitHorizontal {
		availableRight := m.width - fileWidth
		if availableRight < 0 {
			availableRight = 0
		}
		rightWidth := editorWidth
		if availableRight > rightWidth {
			rightWidth = availableRight
		}
		responsePane := m.renderResponsePane(rightWidth)
		rightColumn := lipgloss.JoinVertical(lipgloss.Left, editorPane, responsePane)
		panes = lipgloss.JoinHorizontal(
			lipgloss.Top,
			filePane,
			rightColumn,
		)
	} else {
		availableResponseWidth := m.width - fileWidth - editorWidth
		if availableResponseWidth < 0 {
			availableResponseWidth = 0
		}
		responsePane := m.renderResponsePane(availableResponseWidth)
		panes = lipgloss.JoinHorizontal(
			lipgloss.Top,
			filePane,
			editorPane,
			responsePane,
		)
	}
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
	if m.showThemeSelector {
		return m.renderWithinAppFrame(m.renderThemeModal())
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
	if len(m.fileList.Items()) == 0 {
		filesView = centeredListView(
			filesView,
			innerWidth,
			m.theme.HeaderValue.Render("No items"))
	}
	if len(m.requestItems) == 0 {
		requestsView = centeredListView(
			requestsView,
			innerWidth,
			m.theme.HeaderValue.Render("No requests parsed"))
	}
	if len(m.workflowItems) == 0 {
		workflowsView = centeredListView(
			workflowsView,
			innerWidth,
			m.theme.HeaderValue.Render("No workflows defined"))
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

	inactiveSection := lipgloss.NewStyle().Faint(true)
	if m.focus != focusFile {
		filesSection = inactiveSection.Render(filesSection)
	}
	if m.focus != focusRequests {
		requestsSection = inactiveSection.Render(requestsSection)
	}
	if m.focus != focusWorkflows {
		workflowsSection = inactiveSection.Render(workflowsSection)
	}

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

func centeredListView(view string, width int, content string) string {
	height := lipgloss.Height(view)
	if height < 1 {
		height = 1
	}
	if width < 1 {
		width = 1
	}
	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		content,
	)
}

func (m Model) renderEditorPane() string {
	style := m.theme.EditorBorder
	if m.focus == focusEditor && m.editorInsertMode {
		if items, selection, ok := m.editor.metadataHintsDisplay(metadataHintDisplayLimit); ok && len(items) > 0 {
			overlay := m.buildMetadataHintOverlay(items, selection, m.editor.Width())
			m.editor.SetOverlayLines(overlay)
		} else {
			m.editor.ClearOverlay()
		}
	} else {
		m.editor.ClearOverlay()
	}

	content := m.editor.View()
	innerWidth := lipgloss.Width(content)
	minInnerWidth := m.editor.Width() + 4
	if innerWidth < minInnerWidth {
		innerWidth = minInnerWidth
	}
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
	editorContentHeight := m.editorContentHeight
	if editorContentHeight <= 0 {
		editorContentHeight = m.paneContentHeight
	}
	innerHeight := maxInt(m.editor.Height(), editorContentHeight)
	height := innerHeight + frameHeight
	return style.
		Width(innerWidth).
		Height(height).
		Render(content)
}

func (m Model) buildMetadataHintOverlay(items []metadataHintOption, selection int, width int) []string {
	if len(items) == 0 || width <= 0 {
		return nil
	}
	lines := make([]string, len(items))
	for i, item := range items {
		labelStyle := m.theme.EditorHintItem
		if i == selection {
			labelStyle = m.theme.EditorHintSelected
		}
		label := labelStyle.Render(item.Label)
		if item.Summary != "" {
			annotation := m.theme.EditorHintAnnotation.Render(item.Summary)
			lines[i] = lipgloss.JoinHorizontal(lipgloss.Top, label, " ", annotation)
		} else {
			lines[i] = label
		}
	}
	boxWidth := width
	if boxWidth > 60 {
		boxWidth = 60
	}
	content := strings.Join(lines, "\n")
	box := m.theme.EditorHintBox.Width(boxWidth).Render(content)
	rawLines := strings.Split(box, "\n")
	overlay := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		trimmed := ansi.Truncate(line, width, "")
		overlay = append(overlay, trimmed)
	}
	return overlay
}

func (m Model) renderResponsePane(availableWidth int) string {
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

	frameWidth := style.GetHorizontalFrameSize()
	targetOuterWidth := availableWidth
	if targetOuterWidth < frameWidth {
		targetOuterWidth = frameWidth
	}
	contentBudget := targetOuterWidth - frameWidth
	if contentBudget < 1 {
		contentBudget = 1
	}

	var body string
	if m.responseSplit {
		primaryFocused := active && m.responsePaneFocus == responsePanePrimary
		secondaryFocused := active && m.responsePaneFocus == responsePaneSecondary
		if m.responseSplitOrientation == responseSplitHorizontal {
			columnWidth := maxInt(contentBudget, 1)
			primaryPane := m.pane(responsePanePrimary)
			secondaryPane := m.pane(responsePaneSecondary)
			primaryWidth := clampPositive(1, columnWidth)
			secondaryWidth := clampPositive(1, columnWidth)
			if primaryPane != nil {
				primaryWidth = clampPositive(primaryPane.viewport.Width, columnWidth)
			}
			if secondaryPane != nil {
				secondaryWidth = clampPositive(secondaryPane.viewport.Width, columnWidth)
			}
			top := m.renderResponseColumn(responsePanePrimary, primaryFocused, primaryWidth)
			bottom := m.renderResponseColumn(responsePaneSecondary, secondaryFocused, secondaryWidth)
			divider := m.renderResponseDividerHorizontal(top, bottom)
			if divider != "" {
				body = lipgloss.JoinVertical(lipgloss.Left, top, divider, bottom)
			} else {
				body = lipgloss.JoinVertical(lipgloss.Left, top, bottom)
			}
		} else {
			dividerWidth := responseSplitSeparatorWidth
			availableForColumns := contentBudget - dividerWidth
			if availableForColumns < 1 {
				availableForColumns = contentBudget
				dividerWidth = 0
			}
			primary := m.pane(responsePanePrimary)
			secondary := m.pane(responsePaneSecondary)
			primaryWidth := 1
			secondaryWidth := 1
			if primary != nil {
				primaryWidth = maxInt(primary.viewport.Width, 1)
			}
			if secondary != nil {
				secondaryWidth = maxInt(secondary.viewport.Width, 1)
			}
			totalColumns := primaryWidth + secondaryWidth
			if availableForColumns > 0 && totalColumns > availableForColumns {
				scale := float64(availableForColumns) / float64(totalColumns)
				primaryWidth = int(math.Round(float64(primaryWidth) * scale))
				if primaryWidth < 1 {
					primaryWidth = 1
				}
				secondaryWidth = availableForColumns - primaryWidth
				if secondaryWidth < 1 {
					secondaryWidth = 1
					if availableForColumns > 1 {
						primaryWidth = availableForColumns - secondaryWidth
					}
				}
			}
			if dividerWidth > 0 && primaryWidth+secondaryWidth > availableForColumns {
				excess := primaryWidth + secondaryWidth - availableForColumns
				if primaryWidth >= secondaryWidth {
					primaryWidth -= excess
					if primaryWidth < 1 {
						primaryWidth = 1
					}
				} else {
					secondaryWidth -= excess
					if secondaryWidth < 1 {
						secondaryWidth = 1
					}
				}
			}
			left := m.renderResponseColumn(responsePanePrimary, primaryFocused, clampPositive(primaryWidth, contentBudget))
			right := m.renderResponseColumn(responsePaneSecondary, secondaryFocused, clampPositive(secondaryWidth, contentBudget))
			divider := m.renderResponseDivider(left, right)
			if divider != "" {
				body = lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right)
			} else {
				body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
			}
		}
	} else {
		primary := m.pane(responsePanePrimary)
		columnWidth := 1
		if primary != nil {
			columnWidth = maxInt(primary.viewport.Width, 1)
		}
		if contentBudget > 0 && columnWidth > contentBudget {
			columnWidth = contentBudget
		}
		column := m.renderResponseColumn(responsePanePrimary, active, columnWidth)
		if !active {
			column = lipgloss.NewStyle().Faint(true).Render(column)
		}
		body = column
	}

	width := targetOuterWidth
	frameHeight := style.GetVerticalFrameSize()
	responseHeight := m.responseContentHeight
	if responseHeight <= 0 {
		responseHeight = m.paneContentHeight
	}
	height := responseHeight + frameHeight
	if height < frameHeight {
		height = frameHeight
	}
	contentWidth := maxInt(width-frameWidth, 1)
	return style.Width(contentWidth).MaxWidth(width).Height(height).Render(body)
}

func (m Model) renderResponseColumn(id responsePaneID, focused bool, maxWidth int) string {
	pane := m.pane(id)
	if pane == nil {
		return ""
	}

	contentWidth := maxInt(pane.viewport.Width, 1)
	if maxWidth > 0 && maxWidth < contentWidth {
		contentWidth = maxWidth
	}
	contentHeight := maxInt(pane.viewport.Height, 1)

	tabs := m.renderPaneTabs(id, focused, contentWidth)
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

func (m Model) renderPaneTabs(id responsePaneID, focused bool, width int) string {
	pane := m.pane(id)
	if pane == nil {
		return ""
	}

	tabs := m.availableResponseTabs()
	lineWidth := maxInt(width, 1)
	rowStyle := m.theme.Tabs.Width(lineWidth).Align(lipgloss.Center)
	contentLimit := lineWidth
	if contentLimit < 1 {
		contentLimit = 1
	}
	rowContent := m.buildTabRowContent(tabs, pane.activeTab, focused, pane.followLatest, contentLimit)
	row := rowStyle.Render(rowContent)
	row = clampLines(row, 1)
	divider := m.theme.PaneDivider.Width(lineWidth).Render(strings.Repeat("─", lineWidth))
	block := lipgloss.JoinVertical(lipgloss.Left, row, divider)
	return block
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

func (m Model) buildTabRowContent(tabs []responseTab, active responseTab, focused bool, followLatest bool, limit int) string {
	if limit <= 0 {
		limit = 1
	}
	mode := "Pinned"
	if followLatest {
		mode = "Live"
	}
	baseBadgeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#A6A1BB"))
	if !focused || m.focus != focusResponse {
		baseBadgeStyle = baseBadgeStyle.Faint(true)
	}
	type plan struct {
		activeStyle   lipgloss.Style
		inactiveStyle lipgloss.Style
		badgeStyle    lipgloss.Style
		badgeText     string
		labelFn       func(full string, isActive bool) string
	}
	plans := []plan{
		{
			activeStyle:   m.theme.TabActive,
			inactiveStyle: m.theme.TabInactive,
			badgeStyle:    baseBadgeStyle.PaddingLeft(2),
			badgeText:     strings.ToUpper(mode),
			labelFn: func(full string, isActive bool) string {
				text := full
				if isActive && focused {
					text = tabIndicatorPrefix + text
				}
				return text
			},
		},
		{
			activeStyle:   m.theme.TabActive.Padding(0, 1),
			inactiveStyle: m.theme.TabInactive.Padding(0),
			badgeStyle:    baseBadgeStyle.PaddingLeft(1),
			badgeText:     strings.ToUpper(mode),
			labelFn: func(full string, isActive bool) string {
				if isActive {
					if focused {
						return tabIndicatorPrefix + full
					}
					return full
				}
				return shortenLabel(full, 4)
			},
		},
		{
			activeStyle:   m.theme.TabActive.Padding(0),
			inactiveStyle: m.theme.TabInactive.Padding(0),
			badgeStyle:    baseBadgeStyle.PaddingLeft(1),
			badgeText:     firstRuneUpper(mode),
			labelFn: func(full string, isActive bool) string {
				label := firstRuneUpper(full)
				if label == "" {
					label = "-"
				}
				if isActive && focused {
					return tabIndicatorPrefix + label
				}
				return label
			},
		},
	}

	for idx, plan := range plans {
		segments := make([]string, 0, len(tabs))
		for _, tab := range tabs {
			full := m.responseTabLabel(tab)
			text := plan.labelFn(full, tab == active)
			style := plan.inactiveStyle
			if tab == active {
				style = plan.activeStyle
			}
			segments = append(segments, style.Render(text))
		}
		row := strings.Join(segments, " ")
		badge := plan.badgeStyle.Render(plan.badgeText)
		row = lipgloss.JoinHorizontal(lipgloss.Top, row, badge)
		if lipgloss.Width(row) <= limit && !strings.Contains(row, "\n") {
			return row
		}
		if idx == len(plans)-1 {
			return ansi.Truncate(row, limit, "…")
		}
	}
	return ""
}

func clampLines(content string, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) <= maxLines {
		return content
	}
	return strings.Join(lines[:maxLines], "\n")
}

func shortenLabel(label string, maxRunes int) string {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" || maxRunes <= 0 {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= maxRunes {
		return trimmed
	}
	return string(runes[:maxRunes])
}

func firstRuneUpper(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	r, _ := utf8.DecodeRuneInString(trimmed)
	return strings.ToUpper(string(r))
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

func clampPositive(value, maxValue int) int {
	if value < 1 {
		value = 1
	}
	if maxValue > 0 && value > maxValue {
		return maxValue
	}
	return value
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
	return renderCommandBarContainer(m.theme.CommandBar, row)
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
	return renderCommandBarContainer(
		m.theme.CommandBar,
		row,
		withColoredLeadingSpaces(searchCommandBarLeadingColorSpaces),
	)
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
	reserved := lipgloss.Width(label) + lipgloss.Width(modeBadge) + 2 + searchCommandBarLeadingColorSpaces
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
	return renderCommandBarContainer(
		m.theme.CommandBar.Width(width),
		row,
		withColoredLeadingSpaces(searchCommandBarLeadingColorSpaces),
	)
}

const searchCommandBarLeadingColorSpaces = 1

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
	return renderCommandBarContainer(
		m.theme.CommandBar,
		row,
		withColoredLeadingSpaces(searchCommandBarLeadingColorSpaces),
	)
}

type commandBarContainerConfig struct {
	leadingColoredSpaces int
}

type commandBarContainerOption func(*commandBarContainerConfig)

func withColoredLeadingSpaces(spaces int) commandBarContainerOption {
	if spaces < 0 {
		spaces = 0
	}
	return func(cfg *commandBarContainerConfig) {
		cfg.leadingColoredSpaces = spaces
	}
}

func renderCommandBarContainer(
	style lipgloss.Style,
	content string,
	opts ...commandBarContainerOption,
) string {
	var cfg commandBarContainerConfig
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(&cfg)
	}
	padLeft := style.GetPaddingLeft()
	padRight := style.GetPaddingRight()
	width := style.GetWidth()
	maxWidth := style.GetMaxWidth()

	// Remove horizontal padding from the styled region so themes can set
	// a background colour without colouring the edge gutter.
	baseStyle := style.PaddingLeft(0).PaddingRight(0)

	innerWidth := width
	if innerWidth > 0 {
		innerWidth = maxInt(innerWidth-padLeft-padRight, 0)
	}
	innerMaxWidth := maxWidth
	if innerMaxWidth > 0 {
		innerMaxWidth = maxInt(innerMaxWidth-padLeft-padRight, 0)
	}

	leadingSpaces := cfg.leadingColoredSpaces
	if leadingSpaces > 0 {
		if innerWidth > 0 {
			leadingSpaces = minInt(leadingSpaces, innerWidth)
		}
		if innerMaxWidth > 0 {
			leadingSpaces = minInt(leadingSpaces, innerMaxWidth)
		}
	}
	innerSegments := make([]string, 0, 2)
	if leadingSpaces > 0 {
		leadingStyle := baseStyle
		if innerWidth > 0 {
			leadingStyle = leadingStyle.Width(leadingSpaces)
		}
		if innerMaxWidth > 0 {
			leadingStyle = leadingStyle.MaxWidth(leadingSpaces)
		}
		innerSegments = append(innerSegments, leadingStyle.Render(strings.Repeat(" ", leadingSpaces)))
	}

	contentStyle := baseStyle
	if innerWidth > 0 {
		remaining := maxInt(innerWidth-leadingSpaces, 0)
		contentStyle = contentStyle.Width(remaining)
	}
	if innerMaxWidth > 0 {
		remainingMax := maxInt(innerMaxWidth-leadingSpaces, 0)
		contentStyle = contentStyle.MaxWidth(remainingMax)
	}
	innerSegments = append(innerSegments, contentStyle.Render(content))

	inner := lipgloss.JoinHorizontal(lipgloss.Top, innerSegments...)

	if padLeft == 0 && padRight == 0 {
		return inner
	}

	outer := make([]string, 0, 3)
	if padLeft > 0 {
		outer = append(outer, strings.Repeat(" ", padLeft))
	}
	outer = append(outer, inner)
	if padRight > 0 {
		outer = append(outer, strings.Repeat(" ", padRight))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, outer...)
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
	brandLabel := strings.ToUpper("RESTERM")
	brandSegment := m.theme.HeaderBrand.Render(brandLabel)
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

	versionText := strings.TrimSpace(m.cfg.Version)
	if versionText == "" {
		versionText = strings.TrimSpace(m.updateVersion)
	}
	lineWidth := maxInt(m.width-2, 1)
	if versionText != "" {
		versionText = truncateToWidth(versionText, lineWidth)
	}
	versionWidth := lipgloss.Width(versionText)
	minGap := 1
	if versionWidth == 0 || lineWidth <= versionWidth {
		minGap = 0
	}

	leftAvailable := lineWidth
	maxLeftWidth := lineWidth
	if statusBarLeftMaxRatio > 0 && statusBarLeftMaxRatio < 1 {
		ratioWidth := int(math.Round(float64(lineWidth) * statusBarLeftMaxRatio))
		if ratioWidth < maxLeftWidth {
			maxLeftWidth = ratioWidth
		}
	}
	if versionWidth > 0 {
		available := lineWidth - versionWidth - minGap
		if minGap == 0 {
			available = lineWidth - versionWidth
		}
		if available < 0 {
			available = 0
		}
		leftAvailable = available
		if available < maxLeftWidth {
			maxLeftWidth = available
		}
	}

	const sep = "    "
	sepWidth := lipgloss.Width(sep)
	ellipsisWidth := lipgloss.Width("…")

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
	staticWidth := lipgloss.Width(staticText)
	if staticWidth > 0 {
		if staticWidth > leftAvailable {
			maxLeftWidth = leftAvailable
		} else if staticWidth > maxLeftWidth {
			maxLeftWidth = staticWidth
		}
	}
	if statusText != "" && staticWidth > 0 {
		minRequired := staticWidth + sepWidth + ellipsisWidth
		if minRequired <= leftAvailable && maxLeftWidth < minRequired {
			maxLeftWidth = minRequired
		}
	}
	if maxLeftWidth > leftAvailable {
		maxLeftWidth = leftAvailable
	}
	if maxLeftWidth < 0 {
		maxLeftWidth = 0
	}

	maxContentWidth := maxLeftWidth
	messageText := statusText

	if maxContentWidth <= 0 {
		staticText = ""
		messageText = ""
	} else if staticText != "" {
		staticWidth := lipgloss.Width(staticText)
		if staticWidth > maxContentWidth {
			staticText = truncateToWidth(staticText, maxContentWidth)
			messageText = ""
		} else {
			available := maxContentWidth - staticWidth
			if available < 0 {
				available = 0
			}
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

	lineContent := builder.String()
	if lineContent == "" && maxContentWidth > 0 {
		lineContent = truncateToWidth(statusText, maxContentWidth)
	}

	if versionWidth > 0 {
		if maxLeftWidth > 0 {
			lineContent = truncateToWidth(lineContent, maxLeftWidth)
		}
		leftWidth := lipgloss.Width(lineContent)
		spaceWidth := lineWidth - versionWidth - leftWidth
		if spaceWidth < 0 {
			spaceWidth = 0
		}
		if leftWidth > 0 {
			if minGap > 0 && spaceWidth < minGap {
				spaceWidth = minGap
			}
			lineContent = lineContent + strings.Repeat(" ", spaceWidth) + versionText
		} else {
			pad := maxInt(lineWidth-versionWidth, 0)
			if minGap > 0 && pad > lineWidth-versionWidth-minGap {
				pad = lineWidth - versionWidth - minGap
				if pad < 0 {
					pad = 0
				}
			}
			lineContent = strings.Repeat(" ", pad) + versionText
		}
	}

	if lineContent == "" {
		lineContent = truncateToWidth(statusText, lineWidth)
	}

	return m.theme.StatusBar.Render(lineContent)
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

func (m Model) renderThemeModal() string {
	width := minInt(m.width-10, 60)
	if width < 28 {
		width = 28
	}
	commands := fmt.Sprintf(
		"%s Apply    %s Cancel",
		m.theme.CommandBarHint.Render("Enter"),
		m.theme.CommandBarHint.Render("Esc"),
	)
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		m.themeList.View(),
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
		helpRow(m, "Ctrl+Alt+T / g t", "Theme selector"),
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

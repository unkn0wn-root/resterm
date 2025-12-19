package ui

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/bindings"
	"github.com/unkn0wn-root/resterm/internal/theme"
	"github.com/unkn0wn-root/resterm/internal/ui/hint"
	"github.com/unkn0wn-root/resterm/internal/ui/navigator"
)

const (
	statusBarLeftMaxRatio = 0.7
	helpKeyColumnWidth    = 32
)

var headerSegmentIcons = map[string]string{
	"resterm":   "✦",
	"workspace": "▣",
	"env":       "⬢",
	"requests":  "⇄",
	"active":    "⚡",
	"tests":     "✓",
}

func headerIconFor(label string) string {
	key := strings.ToLower(strings.TrimSpace(label))
	if icon, ok := headerSegmentIcons[key]; ok {
		return icon
	}
	return "✦"
}

func headerLabelText(label string) string {
	labelText := strings.ToUpper(strings.TrimSpace(label))
	if labelText == "" {
		labelText = "—"
	}
	icon := headerIconFor(label)
	if icon == "" {
		return labelText
	}
	return fmt.Sprintf("%s %s", icon, labelText)
}

func (m Model) View() string {
	if !m.ready {
		return m.renderWithinAppFrame("Initialising...")
	}

	if m.showErrorModal {
		return m.renderWithinAppFrame(m.renderErrorModal())
	}

	if m.showFileChangeModal {
		return m.renderWithinAppFrame(m.renderFileChangeModal())
	}

	if m.showHistoryPreview {
		return m.renderWithinAppFrame(m.renderHistoryPreviewModal())
	}

	if m.showRequestDetails {
		return m.renderWithinAppFrame(m.renderRequestDetailsModal())
	}

	if m.showResponseSaveModal {
		return m.renderWithinAppFrame(m.renderResponseSaveModal())
	}

	if m.showOpenModal {
		return m.renderWithinAppFrame(m.renderOpenModal())
	}

	if m.showNewFileModal {
		return m.renderWithinAppFrame(m.renderNewFileModal())
	}
	if m.showLayoutSaveModal {
		return m.renderWithinAppFrame(m.renderLayoutSaveModal())
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
		pw := m.responseTargetWidth(fileWidth, editorWidth)
		var responsePane string
		if pw > 0 {
			responsePane = m.renderResponsePane(pw)
			rw := lipgloss.Width(responsePane)
			ex := fileWidth + editorWidth + rw - m.width
			if ex > 0 {
				adj := pw - ex
				if adj > 0 {
					responsePane = m.renderResponsePane(adj)
					rw = lipgloss.Width(responsePane)
					if fileWidth+editorWidth+rw > m.width {
						responsePane = ""
					}
				} else {
					responsePane = ""
				}
			}
		}
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
	collapsed := m.effectiveRegionCollapsed(paneRegionSidebar)
	switch m.focus {
	case focusFile:
		style = style.
			BorderForeground(m.theme.PaneBorderFocusFile).
			Bold(true).
			BorderStyle(lipgloss.ThickBorder())
	case focusRequests, focusWorkflows:
		style = style.
			BorderForeground(m.theme.PaneBorderFocusRequests).
			Bold(true).
			BorderStyle(lipgloss.ThickBorder())
	}
	if !paneActive {
		style = style.Faint(true)
	}
	width := m.sidebarWidthPx
	if width <= 0 {
		width = m.fileList.Width() + 4
	}
	if collapsed {
		height := maxInt(m.paneContentHeight, collapsedPaneHeightRows) + style.GetVerticalFrameSize()
		zoomHidden := m.zoomActive && m.zoomRegion != paneRegionSidebar
		return m.renderCollapsedPane(style, width, height, "Navigator", "g1", zoomHidden, paneActive)
	}

	innerWidth := maxInt(1, width-4)
	filter := m.renderNavigatorFilter(innerWidth, paneActive)
	filterSep := dividerLine(m.theme.PaneDivider, innerWidth)
	available := m.paneContentHeight - lipgloss.Height(filter) - lipgloss.Height(filterSep)
	if available < 1 {
		available = 1
	}

	listHeight := available

	listView := navigator.ListView(m.navigator, m.theme, innerWidth, listHeight, paneActive)
	if listView == "" {
		listView = centerBox(innerWidth, listHeight, m.theme.HeaderValue.Render("No requests discovered"))
	}
	listView = lipgloss.NewStyle().Width(innerWidth).Height(listHeight).Render(listView)

	bodyParts := []string{filter, filterSep, listView}

	content := lipgloss.JoinVertical(lipgloss.Left, bodyParts...)
	content = clampPane(content, innerWidth, m.paneContentHeight)
	targetHeight := m.paneContentHeight + style.GetVerticalFrameSize()
	return style.
		Width(width).
		Height(targetHeight).
		Render(content)
}

func centerBox(width, height int, content string) string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		content,
	)
}

func paddedLeftLine(width, pad int, text string) string {
	if width < 1 {
		width = 1
	}
	if pad < 0 {
		pad = 0
	}

	inner := maxInt(width-(pad*2), 1)
	wrapped := wrapToWidth(text, inner)
	return lipgloss.NewStyle().
		Width(width).
		Padding(0, pad).
		Align(lipgloss.Left).
		Render(wrapped)
}

// clampPane ensures the navigator pane renders within a fixed rectangle.
func clampPane(content string, width, height int) string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}

	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}

	for i, line := range lines {
		line = ansi.Truncate(line, width, "")
		lineWidth := lipgloss.Width(line)
		if lineWidth < width {
			line += strings.Repeat(" ", width-lineWidth)
		}
		lines[i] = line
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return strings.Join(lines, "\n")
}

func dividerLine(st lipgloss.Style, width int) string {
	if width < 1 {
		width = 1
	}
	return st.Width(width).Render(strings.Repeat("─", width))
}

func (m Model) renderNavigatorFilter(width int, active bool) string {
	m.ensureNavigatorFilter()
	input := m.navigatorFilter
	if width > 4 {
		input.Width = width - 2
		if input.Width < 1 {
			input.Width = 1
		}
	}
	filterView := input.View()
	row := filterView
	if chips := m.navigatorMethodChips(); chips != "" {
		row = lipgloss.JoinHorizontal(lipgloss.Left, row, " ", chips)
	}
	if tags := m.navigatorTagChips(); tags != "" {
		row = lipgloss.JoinHorizontal(lipgloss.Left, row, " ", tags)
	}
	if !active && !input.Focused() {
		row = lipgloss.NewStyle().Faint(true).Render(row)
	}
	return lipgloss.NewStyle().Width(width).Render(row)
}

func (m Model) navigatorMethodChips() string {
	if m.navigator == nil {
		return ""
	}
	active := m.navigator.MethodFilters()
	show := m.navigatorFilter.Focused() || len(active) > 0
	if !show {
		return ""
	}
	badge := m.theme.NavigatorBadge.Padding(0, 0)
	dim := len(active) > 0 || !m.navigatorFilter.Focused()
	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "GRPC", "WS"}
	parts := make([]string, 0, len(methods))
	for _, method := range methods {
		on := active[strings.ToUpper(method)]
		style := badge.Foreground(methodColor(m.theme, method))
		if on {
			style = style.Bold(true).Underline(true)
		} else if dim {
			style = style.Faint(true)
		}
		parts = append(parts, style.Render(method))
	}
	return strings.Join(parts, " ")
}

func (m Model) navigatorTagChips() string {
	if m.navigator == nil {
		return ""
	}
	active := m.navigator.TagFilters()
	show := m.navigatorFilter.Focused() || len(active) > 0
	if !show {
		return ""
	}
	tags, more := m.collectNavigatorTagsFiltered(10, filterQueryTokens(m.navigatorFilter.Value()))
	parts := make([]string, 0, len(tags)+1)
	for _, tag := range tags {
		on := active[strings.ToLower(tag)]
		style := m.theme.NavigatorTag
		if on {
			style = style.Bold(true).Underline(true)
		} else {
			style = style.Faint(true)
		}
		parts = append(parts, style.Render("#"+tag))
	}
	if more {
		parts = append(parts, m.theme.NavigatorTag.Faint(true).Render("..."))
	}
	return strings.Join(parts, " ")
}

func (m Model) collectNavigatorTagsFiltered(limit int, queryTokens []string) ([]string, bool) {
	if m.navigator == nil || limit <= 0 {
		return nil, false
	}
	seen := make(map[string]struct{})
	max := limit + 1
	out := make([]string, 0, max)
	var walk func(nodes []*navigator.Node[any])
	shouldStop := func() bool {
		return len(out) >= max
	}
	walk = func(nodes []*navigator.Node[any]) {
		if shouldStop() {
			return
		}
		for _, n := range nodes {
			if shouldStop() {
				return
			}
			for _, t := range n.Tags {
				if shouldStop() {
					return
				}
				key := strings.ToLower(strings.TrimSpace(t))
				if key == "" {
					continue
				}
				if len(queryTokens) > 0 && !tagMatchesQuery(key, queryTokens) {
					continue
				}
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				out = append(out, key)
			}
			walk(n.Children)
		}
	}
	for _, row := range m.navigator.Rows() {
		if shouldStop() {
			break
		}
		if row.Node != nil {
			walk([]*navigator.Node[any]{row.Node})
		}
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more
}

func filterQueryTokens(val string) []string {
	if strings.TrimSpace(val) == "" {
		return nil
	}
	fields := strings.Fields(strings.ToLower(val))
	tokens := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		tokens = append(tokens, f)
	}
	return tokens
}

func tagMatchesQuery(tag string, query []string) bool {
	if tag == "" || len(query) == 0 {
		return true
	}
	for _, q := range query {
		if q == "" {
			continue
		}
		if strings.Contains(tag, q) {
			return true
		}
	}
	return false
}

func methodColor(th theme.Theme, method string) lipgloss.Color {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "GET":
		return th.MethodColors.GET
	case "POST":
		return th.MethodColors.POST
	case "PUT":
		return th.MethodColors.PUT
	case "PATCH":
		return th.MethodColors.PATCH
	case "DELETE":
		return th.MethodColors.DELETE
	case "HEAD":
		return th.MethodColors.HEAD
	case "OPTIONS":
		return th.MethodColors.OPTIONS
	case "GRPC":
		return th.MethodColors.GRPC
	case "WS", "WEBSOCKET":
		return th.MethodColors.WS
	default:
		return th.MethodColors.Default
	}
}

func (m Model) renderEditorPane() string {
	style := m.theme.EditorBorder
	collapsed := m.effectiveRegionCollapsed(paneRegionEditor)
	if m.focus == focusEditor && m.editorInsertMode && !collapsed {
		if items, selection, ok := m.editor.metadataHintsDisplay(metadataHintDisplayLimit); ok && len(items) > 0 {
			overlay := m.buildMetadataHintOverlay(items, selection, m.editor.Width())
			m.editor.SetOverlayLines(overlay)
		} else {
			m.editor.ClearOverlay()
		}
	} else {
		m.editor.ClearOverlay()
	}

	if collapsed {
		if m.focus == focusEditor {
			style = style.
				BorderForeground(lipgloss.Color("#B794F6")).
				Bold(true).
				BorderStyle(lipgloss.ThickBorder())
		} else {
			style = style.Faint(true)
		}
		width := m.editor.Width() + 4
		height := maxInt(m.editorContentHeight, collapsedPaneHeightRows)
		if height < collapsedPaneHeightRows {
			height = collapsedPaneHeightRows
		}
		height += style.GetVerticalFrameSize()
		zoomHidden := m.zoomActive && m.zoomRegion != paneRegionEditor
		return m.renderCollapsedPane(style, width, height, "Editor", "g2", zoomHidden, m.focus == focusEditor)
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

func (m Model) buildMetadataHintOverlay(items []hint.Hint, selection int, width int) []string {
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
	collapsed := m.effectiveRegionCollapsed(paneRegionResponse)
	if active {
		style = style.
			BorderForeground(lipgloss.Color("#6CC4C4")).
			Bold(true).
			BorderStyle(lipgloss.ThickBorder())
	} else {
		style = style.Faint(true)
	}

	frameWidth := style.GetHorizontalFrameSize()
	if availableWidth < 0 {
		availableWidth = 0
	}
	targetOuterWidth := availableWidth
	if targetOuterWidth < frameWidth {
		targetOuterWidth = frameWidth
	}
	contentBudget := targetOuterWidth - frameWidth
	if contentBudget < 1 {
		contentBudget = 1
	}

	if collapsed {
		height := m.responseContentHeight
		if height <= 0 {
			height = maxInt(m.paneContentHeight, collapsedPaneHeightRows)
		}
		height += style.GetVerticalFrameSize()
		stubWidth := collapsedPaneWidthPx
		if stubWidth > targetOuterWidth || availableWidth == 0 {
			stubWidth = collapsedPaneWidthPx
		}
		minOuter := frameWidth + 1
		if stubWidth < minOuter {
			stubWidth = minOuter
		}
		if stubWidth < targetOuterWidth {
			targetOuterWidth = stubWidth
		}
		zoomHidden := m.zoomActive && m.zoomRegion != paneRegionResponse
		return m.renderCollapsedPane(style, targetOuterWidth, height, "Response", "g3", zoomHidden, active)
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

func (m Model) responseTargetWidth(fileWidth, editorWidth int) int {
	pw := m.responseWidthPx
	if pw <= 0 {
		frame := m.theme.ResponseBorder.GetHorizontalFrameSize()
		pw = m.responseContentWidth() + frame
		if pw < 0 {
			pw = 0
		}
	}

	ef := m.theme.EditorBorder.GetHorizontalFrameSize()
	eo := m.editor.Width() + ef
	if eo < 0 {
		eo = 0
	}

	la := m.width - m.sidebarWidthPx - eo
	if la < 0 {
		la = 0
	}
	if pw > la {
		pw = la
	}

	aa := m.width - fileWidth - editorWidth
	if aa < 0 {
		pw += aa
	} else if pw < aa {
		if la < aa {
			pw = la
		} else {
			pw = aa
		}
	}
	if pw < 0 {
		pw = 0
	}
	return pw
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
	plans := []tabRowPlan{
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
			adaptive:      true,
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
		var (
			row  string
			fits bool
		)
		if plan.adaptive {
			row, fits = m.buildAdaptiveTabRow(tabs, active, focused, plan, limit)
		} else {
			row, fits = m.buildStaticTabRow(tabs, active, plan, limit)
		}
		if fits {
			return row
		}
		if idx == len(plans)-1 {
			return ansi.Truncate(row, limit, "…")
		}
	}
	return ""
}

func (m Model) buildStaticTabRow(tabs []responseTab, active responseTab, plan tabRowPlan, limit int) (string, bool) {
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
	return row, lipgloss.Width(row) <= limit && !strings.Contains(row, "\n")
}

func (m Model) buildAdaptiveTabRow(tabs []responseTab, active responseTab, focused bool, plan tabRowPlan, limit int) (string, bool) {
	states := make([]tabLabelState, 0, len(tabs))
	for _, tab := range tabs {
		runes := []rune(m.responseTabLabel(tab))
		state := tabLabelState{
			runes:     runes,
			isActive:  tab == active,
			maxLength: len(runes),
		}
		if state.isActive {
			state.length = state.maxLength
		} else {
			state.length = minInt(state.maxLength, 4)
		}
		states = append(states, state)
	}

	row, width := m.renderTabRowFromStates(states, plan, focused)
	if width > limit || strings.Contains(row, "\n") {
		return row, false
	}

	for {
		expanded := false
		for i := range states {
			state := &states[i]
			if state.isActive || state.length >= state.maxLength {
				continue
			}
			state.length++
			candidate, candidateWidth := m.renderTabRowFromStates(states, plan, focused)
			if candidateWidth <= limit && !strings.Contains(candidate, "\n") {
				row = candidate
				expanded = true
				continue
			}
			state.length--
		}
		if !expanded {
			break
		}
	}

	return row, true
}

func (m Model) renderTabRowFromStates(states []tabLabelState, plan tabRowPlan, focused bool) (string, int) {
	segments := make([]string, 0, len(states))
	for _, state := range states {
		length := state.length
		if length < 0 {
			length = 0
		}
		if length > state.maxLength {
			length = state.maxLength
		}
		label := string(state.runes[:length])
		if state.isActive && focused {
			label = tabIndicatorPrefix + label
		}
		style := plan.inactiveStyle
		if state.isActive {
			style = plan.activeStyle
		}
		segments = append(segments, style.Render(label))
	}
	row := strings.Join(segments, " ")
	badge := plan.badgeStyle.Render(plan.badgeText)
	row = lipgloss.JoinHorizontal(lipgloss.Top, row, badge)
	return row, lipgloss.Width(row)
}

type tabLabelState struct {
	runes     []rune
	isActive  bool
	length    int
	maxLength int
}

type tabRowPlan struct {
	activeStyle   lipgloss.Style
	inactiveStyle lipgloss.Style
	badgeStyle    lipgloss.Style
	badgeText     string
	labelFn       func(full string, isActive bool) string
	adaptive      bool
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

func (m Model) renderCollapsedPane(style lipgloss.Style, width, height int, label, key string, zoomHidden bool, focused bool) string {
	frameWidth := style.GetHorizontalFrameSize()
	frameHeight := style.GetVerticalFrameSize()
	if width < frameWidth+1 {
		width = frameWidth + 1
	}
	if height < frameHeight+1 {
		height = frameHeight + 1
	}
	innerWidth := maxInt(width-frameWidth, 1)
	innerHeight := maxInt(height-frameHeight, 1)
	_ = label
	_ = key
	markerColor := lipgloss.Color("#3BD671")
	if zoomHidden {
		markerColor = lipgloss.Color("#FBBF24")
	}
	marker := lipgloss.NewStyle().
		Foreground(markerColor).
		Bold(true).
		Render("●")
	if !focused {
		marker = lipgloss.NewStyle().Faint(true).Render(marker)
	}
	content := lipgloss.Place(
		innerWidth,
		innerHeight,
		lipgloss.Center,
		lipgloss.Center,
		marker,
		lipgloss.WithWhitespaceChars(" "),
	)
	return style.Width(width).Height(height).Render(content)
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
			request = "∅"
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
	brandLabel := headerLabelText("RESTERM")
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
	labelText := headerLabelText(label)
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
		switch {
		case m.fileMissing:
			statusText = "File missing on disk"
		case m.fileStale:
			statusText = "File changed on disk"
		case m.dirty:
			statusText = "Unsaved changes"
		default:
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
	if m.currentFile != "" {
		segments = append(segments, filepath.Base(m.currentFile))
	}
	segments = append(segments, fmt.Sprintf("Focus: %s", m.focusLabel()))
	if m.focus == focusEditor {
		mode := "VIEW"
		if m.editorInsertMode {
			mode = "INSERT"
		} else if m.editor.isVisualLineMode() {
			mode = "VISUAL LINE"
		} else if m.editor.isVisualMode() {
			mode = "VISUAL"
		}
		segments = append(segments, fmt.Sprintf("Mode: %s", mode))
	}
	if m.sidebarCollapsed {
		segments = append(segments, "Sidebar:min")
	}
	if m.editorCollapsed {
		segments = append(segments, "Editor:min")
	}
	if m.responseCollapsed {
		segments = append(segments, "Response:min")
	}
	if m.zoomActive {
		segments = append(segments, fmt.Sprintf("Zoom: %s", m.collapsedStatusLabel(m.zoomRegion)))
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

func (m Model) renderRequestDetailsModal() string {
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
	title := strings.TrimSpace(m.requestDetailTitle)
	if title == "" {
		title = "Request Details"
	}
	viewWidth := maxInt(contentWidth-4, 20)
	bodyHeight := maxInt(min(m.height-8, 18), 8)
	if bodyHeight > m.height-6 {
		bodyHeight = maxInt(m.height-6, 8)
	}
	if bodyHeight <= 0 {
		bodyHeight = 8
	}
	if viewWidth <= 0 {
		viewWidth = 20
	}

	body := renderDetailFields(m.requestDetailFields, viewWidth, m.theme)
	if strings.TrimSpace(body) == "" {
		body = "No request details available."
	}

	var bodyView string
	if vp := m.requestDetailViewport; vp != nil {
		vp.SetContent(body)
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
			Render(body)
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

func (m Model) renderLayoutSaveModal() string {
	bodyText := "Save current pane sizes and splits to your settings file?"
	hintsText := fmt.Sprintf(
		"Yes (%s)    No (%s)    Exit (%s)",
		m.theme.CommandBarHint.Render("Y/y"),
		m.theme.CommandBarHint.Render("N/n"),
		m.theme.CommandBarHint.Render("Esc"),
	)
	pad := 2
	frame := m.theme.BrowserBorder.GetHorizontalFrameSize()
	longest := maxInt(lipgloss.Width(bodyText), lipgloss.Width(hintsText))
	minContent := maxInt(32, longest+(pad*2))

	width := m.width - 10
	if width > 68 {
		width = 68
	}

	minOuter := minContent + frame
	if width < minOuter {
		candidate := m.width - 4
		if candidate > 0 {
			width = maxInt(candidate, minOuter)
		} else {
			width = minOuter
		}
	}

	contentWidth := maxInt(width-frame, minContent)
	title := m.theme.HeaderTitle.
		Width(contentWidth).
		Align(lipgloss.Center).
		Render("Save Layout")
	body := paddedLeftLine(contentWidth, pad, bodyText)
	hints := paddedLeftLine(contentWidth, pad, hintsText)
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		body,
		"",
		"",
		hints,
	)
	box := m.theme.BrowserBorder.Width(width).Render(content)
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

func (m Model) renderFileChangeModal() string {
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
	message := strings.TrimSpace(m.fileChangeMessage)
	if message == "" {
		message = "File changed outside this session."
	}
	reloadKey := m.helpActionKey(bindings.ActionReloadFileFromDisk, "g Shift+R")
	body := paddedLeftLine(contentWidth, 2, message)
	info := fmt.Sprintf("%s Reload    %s Dismiss", m.theme.CommandBarHint.Render(reloadKey), m.theme.CommandBarHint.Render("Esc"))
	infoLine := paddedLeftLine(contentWidth, 2, info)
	title := m.theme.HeaderTitle.
		Width(contentWidth).
		Align(lipgloss.Center).
		Render("File Change Detected")

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		body,
		"",
		infoLine,
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
	width := minInt(m.width-6, 120)
	if width < 48 {
		width = 48
	}

	contentWidth := maxInt(width-6, 30)
	viewWidth := maxInt(contentWidth-6, 22)
	maxBodyHeight := m.height - 8
	if maxBodyHeight < 6 {
		maxBodyHeight = 6
	}

	bodyHeight := maxInt(min(28, maxBodyHeight), 6)

	header := func(text string, align lipgloss.Position) string {
		return m.theme.HeaderTitle.
			Width(viewWidth).
			Align(align).
			Render(text)
	}

	sections := []struct {
		title   string
		entries []helpEntry
	}{
		{
			title: "Navigation & Focus",
			entries: sortedHelpEntries([]helpEntry{
				{m.helpActionKey(bindings.ActionCycleFocusNext, "Tab"), "Cycle focus"},
				{m.helpActionKey(bindings.ActionCycleFocusPrev, "Shift+Tab"), "Reverse focus"},
				{m.helpCombinedKey([]bindings.ActionID{bindings.ActionToggleZoom, bindings.ActionClearZoom}, "g z / g Z"), "Zoom focused pane / reset zoom"},
				{m.helpCombinedKey([]bindings.ActionID{bindings.ActionFocusRequests, bindings.ActionFocusEditorNormal, bindings.ActionFocusResponse}, "g r / g i / g p"), "Focus navigator / editor / response"},
			}),
		},
		{
			title: "Requests & Files",
			entries: sortedHelpEntries([]helpEntry{
				{"Enter", "Run selected request"},
				{"Space", "Preview selected request / toggle file expansion"},
				{m.helpActionKey(bindings.ActionShowRequestDetails, "g ,"), "Show selected request details"},
				{m.helpActionKey(bindings.ActionSendRequest, "Ctrl+Enter"), "Send active request"},
				{m.helpActionKey(bindings.ActionCancelRun, "Ctrl+C"), "Cancel in-flight run/request"},
				{m.helpActionKey(bindings.ActionSaveFile, "Ctrl+S"), "Save current file"},
				{m.helpActionKey(bindings.ActionSaveLayout, "g Shift+L"), "Save layout to settings"},
				{m.helpActionKey(bindings.ActionOpenNewFileModal, "Ctrl+N"), "Create request file"},
				{m.helpActionKey(bindings.ActionOpenPathModal, "Ctrl+O"), "Open file or folder"},
				{m.helpActionKey(bindings.ActionReloadWorkspace, "Ctrl+Shift+O"), "Refresh workspace"},
				{m.helpActionKey(bindings.ActionOpenTempDocument, "Ctrl+T"), "Temporary document"},
				{m.helpActionKey(bindings.ActionReparseDocument, "Ctrl+P"), "Reparse document"},
				{m.helpActionKey(bindings.ActionReloadFileFromDisk, "Ctrl+Alt+R"), "Reload file from disk"},
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
				{m.helpCombinedKey([]bindings.ActionID{bindings.ActionSidebarHeightDecrease, bindings.ActionSidebarHeightIncrease}, "g j / g k"), "Collapse / expand current navigator branch"},
				{m.helpCombinedKey([]bindings.ActionID{bindings.ActionWorkflowHeightIncrease, bindings.ActionWorkflowHeightDecrease}, "g Shift+J / g Shift+K"), "Collapse all / expand all navigator branches"},
			}),
		},
		{
			title: "Layout & View",
			entries: sortedHelpEntries([]helpEntry{
				{m.helpCombinedKey([]bindings.ActionID{bindings.ActionToggleResponseSplitVert, bindings.ActionToggleResponseSplitHorz}, "Ctrl+V / Ctrl+U"), "Split response vertically / horizontally"},
				{m.helpActionKey(bindings.ActionTogglePaneFollowLatest, "Ctrl+Shift+V"), "Pin or unpin focused response pane"},
				{m.helpActionKey(bindings.ActionCopyResponseTab, "Ctrl+Shift+C"), "Copy Pretty / Raw / Headers response tab"},
				{m.helpCombinedKey([]bindings.ActionID{bindings.ActionScrollResponseTop, bindings.ActionScrollResponseBottom}, "gg / G"), "Response tab: top / bottom"},
				{m.helpActionKey(bindings.ActionToggleHeaderPreview, "g Shift+H"), "Toggle request/response headers view"},
				{m.helpActionKey(bindings.ActionCycleRawView, "g b"), "Cycle raw view: text / hex / base64 (summary for large binary)"},
				{m.helpActionKey(bindings.ActionShowRawDump, "g Shift+D"), "Load full raw dump (hex)"},
				{m.helpActionKey(bindings.ActionSaveResponseBody, "g Shift+S"), "Save response body to file"},
				{m.helpActionKey(bindings.ActionOpenResponseExternally, "g Shift+E"), "Open response in external app"},
				{"Ctrl+F or Ctrl+B, ←/→", "Send future responses to selected pane"},
				{m.helpCombinedKey([]bindings.ActionID{bindings.ActionSidebarWidthDecrease, bindings.ActionSidebarWidthIncrease}, "g h / g l"), "Adjust editor/response width"},
				{m.helpCombinedKey([]bindings.ActionID{bindings.ActionToggleSidebarCollapse, bindings.ActionToggleEditorCollapse, bindings.ActionToggleResponseCollapse}, "g1 / g2 / g3"), "Toggle sidebar / editor / response minimize"},
			}),
		},
		{
			title: "Environment & Themes",
			entries: sortedHelpEntries([]helpEntry{
				{m.helpActionKey(bindings.ActionShowGlobals, "Ctrl+G"), "Show globals summary"},
				{m.helpActionKey(bindings.ActionClearGlobals, "Ctrl+Shift+G"), "Clear globals for environment"},
				{m.helpActionKey(bindings.ActionOpenEnvSelector, "Ctrl+E"), "Environment selector"},
				{m.helpActionKey(bindings.ActionSelectTimelineTab, "Ctrl+Alt+L / g t"), "Timeline tab"},
				{m.helpActionKey(bindings.ActionOpenThemeSelector, "Ctrl+Alt+T / g m"), "Theme selector"},
			}),
		},
		{
			title: "Editor motions",
			entries: []helpEntry{
				{"h / j / k / l", "Move left / down / up / right"},
				{"w / b / e", "Next word / previous word / word end"},
				{"0 / ^ / $", "Line start / first non-blank / line end"},
				{"gg / G", "Top / bottom of buffer"},
				{"Ctrl+f / Ctrl+b", "Page down / up (Ctrl+d / Ctrl+u half-page)"},
				{"v / V / y", "Visual select (char / line) / yank selection"},
				{"d + motion", "Delete via Vim motions (dw, db, dk, dgg, dG)"},
				{"dd / D / x / c", "Delete line / to end / char / change line"},
				{"a", "Append after cursor (enter insert mode)"},
				{"p / P", "Paste after / before cursor"},
				{"f / t / T", "Find character (forward / till / backward)"},
				{"u / Ctrl+r", "Undo / redo last edit"},
			},
		},
		{
			title: "Search",
			entries: []helpEntry{
				{"Shift+F", "Open search prompt (Ctrl+R toggles regex)"},
				{"n / p", "Next / previous match (wraps around)"},
			},
		},
	}

	rows := []string{
		header("Key Bindings", lipgloss.Center),
		m.theme.HeaderValue.Render("Esc closes • ↑/↓ scroll • PgUp/PgDn page"),
		"",
	}

	var orderedSections []struct {
		title   string
		entries []helpEntry
	}
	for _, section := range sections {
		if len(section.entries) == 0 {
			continue
		}
		orderedSections = append(orderedSections, section)
	}

	for idx, section := range orderedSections {
		rows = append(rows, header(section.title, lipgloss.Left))
		for _, entry := range section.entries {
			rows = append(rows, helpRow(m, entry.key, entry.description))
		}
		if idx < len(orderedSections)-1 {
			rows = append(rows, "")
		}
	}
	body := lipgloss.JoinVertical(lipgloss.Left, rows...)

	var bodyView string
	if vp := m.helpViewport; vp != nil {
		vp.Width = viewWidth
		vp.Height = bodyHeight
		vp.SetContent(body)
		bodyView = lipgloss.NewStyle().
			Padding(0, 2).
			Width(contentWidth).
			Render(vp.View())
	} else {
		bodyView = lipgloss.NewStyle().
			Padding(0, 2).
			Width(contentWidth).
			Render(body)
	}

	box := m.theme.BrowserBorder.Width(width).Render(bodyView)
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

func (m Model) renderResponseSaveModal() string {
	width := minInt(m.width-10, 72)
	if width < 40 {
		width = 40
	}
	bg := lipgloss.Color("#1c1a23")
	inputView := lipgloss.NewStyle().
		Width(width - 8).
		Background(bg).
		Render(m.responseSaveInput.View())
	inputBox := lipgloss.NewStyle().
		Width(width - 8).
		Background(bg).
		Render(inputView)

	enter := m.theme.CommandBarHint.Render("Enter")
	esc := m.theme.CommandBarHint.Render("Esc")
	info := fmt.Sprintf("%s Save    %s Cancel", enter, esc)

	lines := []string{
		m.theme.HeaderTitle.
			Width(width - 4).
			Align(lipgloss.Center).
			Render("Save Response Body"),
		"",
		lipgloss.NewStyle().
			Padding(0, 2).
			Bold(true).
			Render("Choose a path to save the response body"),
		lipgloss.NewStyle().
			Padding(0, 2).
			Render(inputBox),
	}
	if m.responseSaveError != "" {
		errorLine := m.theme.Error.
			Padding(0, 2).
			Render(m.responseSaveError)
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

type helpEntry struct {
	key         string
	description string
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

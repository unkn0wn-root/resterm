package ui

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/bindings"
	"github.com/unkn0wn-root/resterm/internal/theme"
	"github.com/unkn0wn-root/resterm/internal/ui/navigator"
)

const (
	statusBarLeftMaxRatio = 0.7
	statusBarSep          = "    "
	statusBarSepWidth     = 4
	helpKeyColumnWidth    = 32
)

const (
	statusInfoLightColor    = "#64748b"
	statusInfoDarkColor     = "#cbd5e1"
	statusWarnLightColor    = "#d97706"
	statusWarnDarkColor     = "#FACC15"
	statusErrorLightColor   = "#dc2626"
	statusErrorDarkColor    = "#FF6E6E"
	statusSuccessLightColor = "#15803d"
	statusSuccessDarkColor  = "#6EF17E"
)

var headerSegmentIcons = map[string]string{
	"resterm":   ">_",
	"workspace": "▣",
	"env":       "⬢",
	"requests":  "⇄",
	"active":    "◧",
	"tests":     "⧗",
}

type testStatus string

const (
	testStatusPass  testStatus = "pass"
	testStatusFail  testStatus = "fail"
	testStatusError testStatus = "error"
)

type statusBarSeg struct {
	key string
	val string
}

type statusBarLeft struct {
	msg          string
	level        statusLevel
	ctx          string
	ctxTruncated bool
	segs         []statusBarSeg
}

type statusBarPart struct {
	text string
	view string
}

type statusBarFrame struct {
	width         int
	leftAvailable int
	maxLeft       int
	minGap        int
}

type statusBarLayout struct {
	status string
	level  statusLevel
	frame  statusBarFrame
	left   statusBarLeft
	right  statusBarPart
}

func headerIconFor(label string) string {
	key := strings.ToLower(strings.TrimSpace(label))
	if icon, ok := headerSegmentIcons[key]; ok {
		return icon
	}
	return "✦"
}

func headerLabelText(label string) string {
	return headerLabelTextWithIcon(label, "")
}

func headerLabelTextWithIcon(label, iconOverride string) string {
	labelText := strings.ToUpper(strings.TrimSpace(label))
	if labelText == "" {
		labelText = "—"
	}
	icon := iconOverride
	if icon == "" {
		icon = headerIconFor(label)
	}
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
		rightParts := make([]string, 0, 2)
		if editorPane != "" {
			if responsePane == "" && availableRight > 0 {
				editorPane = padToWidth(editorPane, availableRight)
			}
			rightParts = append(rightParts, editorPane)
		}
		if responsePane != "" {
			rightParts = append(rightParts, responsePane)
		}
		rightColumn := ""
		if len(rightParts) > 0 {
			rightColumn = lipgloss.JoinVertical(lipgloss.Left, rightParts...)
		}
		if rightColumn == "" {
			panes = filePane
		} else {
			panes = lipgloss.JoinHorizontal(
				lipgloss.Top,
				filePane,
				rightColumn,
			)
		}
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
		if responsePane == "" && m.width > fileWidth {
			target := m.width - fileWidth
			editorPane = padToWidth(editorPane, target)
		}
		parts := []string{filePane, editorPane}
		if responsePane != "" {
			parts = append(parts, responsePane)
		}
		panes = lipgloss.JoinHorizontal(
			lipgloss.Top,
			parts...,
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
		style = m.themeRuntime.inactiveStyle(style)
	}
	frameWidth := style.GetHorizontalFrameSize()
	width := m.sidebarWidthPx
	if width <= 0 {
		width = paneOuterWidthFromContent(m.fileList.Width(), frameWidth)
		if width <= 0 {
			width = paneOuterWidthFromContent(1, frameWidth)
		}
	}
	if collapsed {
		return ""
	}

	contentWidth := maxInt(paneContentWidth(width, frameWidth), 1)
	filter := m.renderNavigatorFilter(contentWidth, paneActive)
	filterSep := dividerLine(m.theme.PaneDivider, contentWidth)
	available := m.paneContentHeight - lipgloss.Height(filter) - lipgloss.Height(filterSep)
	if available < 1 {
		available = 1
	}

	listHeight := available

	listView := navigator.ListView(
		m.navigator,
		m.theme,
		contentWidth,
		listHeight,
		paneActive,
		m.themeRuntime.appearance,
	)
	if listView == "" {
		listView = centerBox(
			contentWidth,
			listHeight,
			m.theme.HeaderValue.Render("No workspace files discovered"),
		)
	}
	listView = lipgloss.NewStyle().Width(contentWidth).Height(listHeight).Render(listView)

	bodyParts := []string{filter, filterSep, listView}

	content := lipgloss.JoinVertical(lipgloss.Left, bodyParts...)
	content = clampPane(content, contentWidth, m.paneContentHeight)
	content = padHorizontal(content, paneHorizontalPadding)
	targetHeight := m.paneContentHeight + style.GetVerticalFrameSize()
	innerWidth := maxInt(paneInnerWidth(width, frameWidth), 1)
	return style.
		Width(innerWidth).
		MaxWidth(width).
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

func padToWidth(content string, width int) string {
	if width <= 0 {
		return content
	}
	height := lipgloss.Height(content)
	if height < 1 {
		height = 1
	}
	return lipgloss.Place(
		width,
		height,
		lipgloss.Top,
		lipgloss.Left,
		content,
		lipgloss.WithWhitespaceChars(" "),
	)
}

func padHorizontal(content string, padding int) string {
	if padding <= 0 {
		return content
	}
	return lipgloss.NewStyle().Padding(0, padding).Render(content)
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
		row = m.themeRuntime.inactiveRendered(row)
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
			style = m.themeRuntime.inactiveStyle(style)
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
			style = m.themeRuntime.inactiveStyle(style)
		}
		parts = append(parts, style.Render("#"+tag))
	}
	if more {
		parts = append(parts, m.themeRuntime.inactiveStyle(m.theme.NavigatorTag).Render("..."))
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
	if collapsed {
		return ""
	}

	content := m.editor.View()
	if m.focus == focusEditor && m.editorInsertMode {
		content = m.renderMetadataHintPopup(content)
	}
	contentWidth := lipgloss.Width(content)
	if contentWidth < 1 {
		contentWidth = 1
	}
	content = padHorizontal(content, paneHorizontalPadding)
	innerWidth := contentWidth + (paneHorizontalPadding * 2)
	if m.focus == focusEditor {
		style = style.
			BorderForeground(lipgloss.Color("#B794F6")).
			Bold(true).
			BorderStyle(lipgloss.ThickBorder())
	} else {
		style = m.themeRuntime.inactiveStyle(style)
		content = m.themeRuntime.inactiveRendered(content)
	}
	frameHeight := style.GetVerticalFrameSize()
	editorContentHeight := m.editorContentHeight
	if editorContentHeight <= 0 {
		editorContentHeight = m.paneContentHeight
	}
	innerHeight := maxInt(m.editor.Height(), editorContentHeight)
	height := innerHeight + frameHeight
	outerWidth := paneOuterWidthFromContent(contentWidth, style.GetHorizontalFrameSize())
	if outerWidth < 1 {
		outerWidth = innerWidth + style.GetHorizontalFrameSize()
	}
	return style.
		Width(innerWidth).
		MaxWidth(outerWidth).
		Height(height).
		Render(content)
}

func (m Model) renderResponsePane(availableWidth int) string {
	active := m.focus == focusResponse
	style := m.respFrameStyle(active)
	collapsed := m.effectiveRegionCollapsed(paneRegionResponse)

	frameWidth := style.GetHorizontalFrameSize()
	if availableWidth < 0 {
		availableWidth = 0
	}
	targetOuterWidth := availableWidth
	if targetOuterWidth < frameWidth {
		targetOuterWidth = frameWidth
	}
	innerWidth := paneInnerWidth(targetOuterWidth, frameWidth)
	if innerWidth < 1 {
		innerWidth = 1
	}
	contentBudget := innerWidth - (paneHorizontalPadding * 2)
	if contentBudget < 1 {
		contentBudget = 1
	}
	if innerWidth < contentBudget+(paneHorizontalPadding*2) {
		innerWidth = contentBudget + (paneHorizontalPadding * 2)
	}

	if collapsed {
		return ""
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
			bottom := m.renderResponseColumn(
				responsePaneSecondary,
				secondaryFocused,
				secondaryWidth,
			)
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
			left := m.renderResponseColumn(
				responsePanePrimary,
				primaryFocused,
				clampPositive(primaryWidth, contentBudget),
			)
			right := m.renderResponseColumn(
				responsePaneSecondary,
				secondaryFocused,
				clampPositive(secondaryWidth, contentBudget),
			)
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
	body = lipgloss.NewStyle().Width(contentBudget).Render(body)
	body = padHorizontal(body, paneHorizontalPadding)
	return style.Width(innerWidth).MaxWidth(width).Height(height).Render(body)
}

func (m Model) respFrameStyle(active bool) lipgloss.Style {
	st := m.theme.ResponseBorder
	if active {
		st = st.
			BorderForeground(lipgloss.Color("#6CC4C4")).
			BorderStyle(lipgloss.ThickBorder())
	} else {
		st = m.dimRespFrame(st)
	}
	return stripTextAttrs(st)
}

func (m Model) dimRespFrame(st lipgloss.Style) lipgloss.Style {
	if fg := m.theme.PaneDivider.GetForeground(); theme.ColorDefined(fg) {
		st = st.BorderForeground(fg)
	}
	return st
}

// Pretty content carries its own ANSI syntax colors
// text attributes on the frame leak into unstyled tokens until the next inner reset.
func stripTextAttrs(st lipgloss.Style) lipgloss.Style {
	return st.
		UnsetForeground().
		UnsetBold().
		UnsetItalic().
		UnsetUnderline().
		UnsetStrikethrough().
		UnsetReverse().
		UnsetBlink().
		UnsetFaint()
}

func (m Model) responseTargetWidth(fileWidth, editorWidth int) int {
	if m.effectiveRegionCollapsed(paneRegionResponse) {
		return 0
	}
	pw := m.responseWidthPx
	if pw <= 0 {
		frame := m.theme.ResponseBorder.GetHorizontalFrameSize()
		pw = paneOuterWidthFromContent(m.responseContentWidth(), frame)
		if pw < 0 {
			pw = 0
		}
	}

	eo := editorWidth
	if m.effectiveRegionCollapsed(paneRegionEditor) {
		eo = 0
	} else if eo <= 0 {
		ef := m.theme.EditorBorder.GetHorizontalFrameSize()
		eo = paneOuterWidthFromContent(lipgloss.Width(m.editor.View()), ef)
		if eo < 0 {
			eo = 0
		}
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
	if send := m.sendingView(pane, contentWidth, contentHeight); send != "" {
		content = send
	} else if formatting := m.formattingView(pane, contentWidth, contentHeight); formatting != "" {
		content = formatting
	} else if reflowing := m.reflowView(pane, contentWidth, contentHeight); reflowing != "" {
		content = reflowing
	}
	content = lipgloss.NewStyle().
		MaxWidth(contentWidth).
		MaxHeight(contentHeight).
		Render(content)

	if !focused && m.focus == focusResponse {
		if searchView != "" {
			searchView = m.themeRuntime.inactiveRendered(searchView)
		}
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
	rowContent := m.buildTabRowContent(
		tabs,
		pane,
		focused,
		contentLimit,
	)
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

func (m Model) buildTabRowContent(
	tabs []responseTab,
	pane *responsePaneState,
	focused bool,
	limit int,
) string {
	if limit <= 0 {
		limit = 1
	}
	active := responseTabPretty
	followLatest := false
	var snapshot *responseSnapshot
	if pane != nil {
		active = pane.activeTab
		followLatest = pane.followLatest
		snapshot = pane.snapshot
	}
	labels := make([]string, len(tabs))
	for i, tab := range tabs {
		labels[i] = responseTabLabelForSnapshot(tab, snapshot)
	}
	mode := "Pinned"
	if followLatest {
		mode = "Live"
	}
	badge := m.tabBadgeText(mode)
	shortBadge := m.tabBadgeShort(mode)
	actTab := m.theme.TabActive
	inactTab := m.theme.TabInactive
	if !focused || m.focus != focusResponse {
		actTab = m.themeRuntime.inactiveStyle(actTab)
		inactTab = m.themeRuntime.inactiveStyle(inactTab)
	}
	badgeSt := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#A6A1BB"))
	if !focused || m.focus != focusResponse {
		badgeSt = m.themeRuntime.inactiveStyle(badgeSt)
	}
	plans := []tabRowPlan{
		{
			activeStyle:   actTab,
			inactiveStyle: inactTab,
			badgeStyle:    badgeSt.PaddingLeft(2),
			badgeText:     badge,
			labelFn: func(full string) string {
				return full
			},
		},
		{
			activeStyle:   actTab.Padding(0, 1),
			inactiveStyle: inactTab.Padding(0),
			badgeStyle:    badgeSt.PaddingLeft(1),
			badgeText:     badge,
			adaptive:      true,
		},
		{
			activeStyle:   actTab.Padding(0),
			inactiveStyle: inactTab.Padding(0),
			badgeStyle:    badgeSt.PaddingLeft(1),
			badgeText:     shortBadge,
			labelFn: func(full string) string {
				label := firstRuneUpper(full)
				if label == "" {
					label = "-"
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
			row, fits = m.buildAdaptiveTabRow(tabs, labels, active, focused, plan, limit)
		} else {
			row, fits = m.buildStaticTabRow(tabs, labels, active, focused, plan, limit)
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

var tabSpinFrames = []string{
	"⠋",
	"⠙",
	"⠹",
	"⠸",
	"⠼",
	"⠴",
	"⠦",
	"⠧",
	"⠇",
	"⠏",
}

const responseSendingBase = "Sending request"

func (m Model) tabSpinner() string {
	if !m.spinnerActive() || len(tabSpinFrames) == 0 {
		return ""
	}
	idx := m.tabSpinIdx
	if idx < 0 {
		idx = 0
	}
	return tabSpinFrames[idx%len(tabSpinFrames)]
}

func (m Model) spinnerView(
	pane *responsePaneState,
	width, height int,
	base string,
	active bool,
) string {
	if !m.paneAllowsOverlay(pane) || !pane.followLatest || !active {
		return ""
	}
	spin := m.tabSpinner()
	if spin == "" {
		return ""
	}
	msg := base + " " + spin
	centered := centerContent(msg, width, height)
	return m.applyResponseContentStyles(pane.activeTab, centered)
}

func (m Model) sendingView(pane *responsePaneState, width, height int) string {
	return m.spinnerView(pane, width, height, responseSendingBase, m.sending)
}

func (m Model) formattingView(pane *responsePaneState, width, height int) string {
	return m.spinnerView(pane, width, height, responseFormattingBase, m.responseLoading)
}

func (m Model) reflowView(pane *responsePaneState, width, height int) string {
	if !m.reflowActiveForPane(pane) {
		return ""
	}
	msg := m.responseReflowMessage()
	if msg == "" {
		return ""
	}
	centered := centerContent(msg, width, height)
	return m.applyResponseContentStyles(pane.activeTab, centered)
}

func (m Model) paneAllowsOverlay(pane *responsePaneState) bool {
	if pane == nil {
		return false
	}
	return tabAllowsOverlay(pane.activeTab)
}

func (m Model) reflowActiveForPane(pane *responsePaneState) bool {
	if !m.paneAllowsOverlay(pane) {
		return false
	}
	key, ok := reflowKeyForPane(pane)
	if !ok || pane.reflow == nil {
		return false
	}
	state, ok := pane.reflow[key]
	if !ok || state.token == "" || state.tab != pane.activeTab || state.snapshotID == "" {
		return false
	}
	snap := pane.snapshot
	if snap == nil || !snap.ready || snap.id != state.snapshotID {
		return false
	}
	switch state.tab {
	case responseTabRaw:
		if snap.rawMode != state.mode {
			return false
		}
	case responseTabHeaders:
		if pane.headersView != state.headers {
			return false
		}
	}
	return true
}

func (m Model) tabBadgeText(mode string) string {
	return strings.ToUpper(strings.TrimSpace(mode))
}

func (m Model) tabBadgeShort(mode string) string {
	return firstRuneUpper(mode)
}

func (m Model) buildStaticTabRow(
	tabs []responseTab,
	labels []string,
	active responseTab,
	focused bool,
	plan tabRowPlan,
	limit int,
) (string, bool) {
	segments := make([]string, 0, len(tabs))
	for i, tab := range tabs {
		full := labels[i]
		text := plan.labelFn(full)
		style := plan.inactiveStyle
		if tab == active {
			style = plan.activeStyle
		}
		segments = append(segments, renderTabSegment(style, text, tab == active && focused))
	}
	row := strings.Join(segments, " ")
	badge := plan.badgeStyle.Render(plan.badgeText)
	row = lipgloss.JoinHorizontal(lipgloss.Top, row, badge)
	return row, lipgloss.Width(row) <= limit && !strings.Contains(row, "\n")
}

func (m Model) buildAdaptiveTabRow(
	tabs []responseTab,
	labels []string,
	active responseTab,
	focused bool,
	plan tabRowPlan,
	limit int,
) (string, bool) {
	states := make([]tabLabelState, 0, len(tabs))
	for i, tab := range tabs {
		runes := []rune(labels[i])
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

func (m Model) renderTabRowFromStates(
	states []tabLabelState,
	plan tabRowPlan,
	focused bool,
) (string, int) {
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
		style := plan.inactiveStyle
		if state.isActive {
			style = plan.activeStyle
		}
		segments = append(segments, renderTabSegment(style, label, state.isActive && focused))
	}
	row := strings.Join(segments, " ")
	badge := plan.badgeStyle.Render(plan.badgeText)
	row = lipgloss.JoinHorizontal(lipgloss.Top, row, badge)
	return row, lipgloss.Width(row)
}

func renderTabSegment(st lipgloss.Style, label string, marked bool) string {
	if marked {
		label = tabIndicatorPrefix + label
		if left := st.GetPaddingLeft(); left > 0 {
			st = st.PaddingLeft(left - 1)
		}
	}
	return st.Render(label)
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
	labelFn       func(full string) string
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

func (m Model) renderHistoryPaneFor(id responsePaneID) string {
	pane := m.pane(id)
	if pane == nil {
		return ""
	}

	contentWidth := maxInt(pane.viewport.Width, 1)
	contentHeight := maxInt(pane.viewport.Height, 1)
	header := m.renderHistoryHeader(contentWidth)
	filter := m.renderHistoryFilterLine(contentWidth)
	filterSep := dividerLine(m.theme.PaneDivider, contentWidth)
	bodyHeight := contentHeight - m.historyHeaderHeight()
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	if len(m.historyEntries) == 0 {
		msg := lipgloss.NewStyle().
			MaxWidth(contentWidth).
			MaxHeight(bodyHeight).
			Render(m.historyEmptyMessage())
		listView := lipgloss.Place(
			contentWidth,
			bodyHeight,
			lipgloss.Top,
			lipgloss.Left,
			msg,
			lipgloss.WithWhitespaceChars(" "),
		)
		body := lipgloss.JoinVertical(lipgloss.Left, header, filter, filterSep, listView)
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

	listView := m.historyList.View()
	listView = lipgloss.NewStyle().
		MaxWidth(contentWidth).
		MaxHeight(bodyHeight).
		Render(listView)
	body := lipgloss.JoinVertical(lipgloss.Left, header, filter, filterSep, listView)
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

func (m Model) renderHistoryHeader(width int) string {
	scope := historyScopeLabel(m.historyScope)
	sortLabel := historySortLabel(m.historySort)
	text := fmt.Sprintf("Scope (c): %s  Sort (s): %s", scope, sortLabel)
	return m.theme.HeaderValue.Width(width).MaxHeight(1).Render(text)
}

func (m Model) renderHistoryFilterLine(width int) string {
	input := m.historyFilterInput
	if width > 2 {
		input.Width = width - 2
	} else {
		input.Width = width
	}
	return lipgloss.NewStyle().Width(width).Render(input.View())
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
		{key: "^C", label: "Cancel"},
		{key: "^S", label: "Save"},
		{key: "^N", label: "New"},
		{key: "^O", label: "Open"},
		{key: "^Q", label: "Quit"},
		{key: "g s/v", label: "Split"},
		{key: "g1/2/3", label: "Minimize"},
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
	subtle := m.themeRuntime.subtleTextStyle(m.theme)
	modeBadge := subtle.
		PaddingLeft(2).
		Render(strings.ToUpper(mode))
	hints := subtle.
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
	modeBadge := m.themeRuntime.subtleTextStyle(m.theme).
		PaddingLeft(1).
		Render(strings.ToUpper(mode))
	reserved := lipgloss.Width(
		label,
	) + lipgloss.Width(
		modeBadge,
	) + 2 + searchCommandBarLeadingColorSpaces
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
	subtle := m.themeRuntime.subtleTextStyle(m.theme)
	modeBadge := subtle.
		PaddingLeft(1).
		Render(strings.ToUpper(mode))
	hints := subtle.
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
		innerSegments = append(
			innerSegments,
			leadingStyle.Render(strings.Repeat(" ", leadingSpaces)),
		)
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
		icon  string
	}

	segmentsData := []segment{
		{label: "Workspace", value: workspace},
		{label: "Env", value: env},
		{label: "Requests", value: fmt.Sprintf("%d", len(m.requestItems))},
		{label: "Active", value: request},
	}

	if summary, status, ok := m.headerTestStatus(); ok {
		segmentsData = append(segmentsData, segment{
			label: "Tests",
			value: summary,
			icon:  headerTestIcon(status),
		})
	}

	segments := make([]string, 0, len(segmentsData)+1)
	brandLabel := headerLabelText("RESTERM")
	brandSegment := m.theme.HeaderBrand.Render(brandLabel)
	segments = append(segments, brandSegment)
	for i, seg := range segmentsData {
		segments = append(segments, m.renderHeaderButton(i, seg.label, seg.value, seg.icon))
	}

	separator := m.theme.HeaderSeparator.Render(" ")

	rightText := ""
	rightStyle := m.theme.HeaderValue
	if m.latencySeries != nil {
		rightText = m.latencyText()
		rightStyle = m.latencyStyle()
	}

	totalWidth := maxInt(m.width, 1)
	contentWidth := headerContentWidth(totalWidth, m.theme.Header)
	headerLine := buildHeaderLine(
		segments,
		separator,
		rightText,
		rightStyle,
		contentWidth,
	)
	return m.theme.Header.Width(totalWidth).Render(headerLine)
}

func (m Model) renderHeaderButton(idx int, label, value, icon string) string {
	palette := m.theme.HeaderSegment(idx)
	labelText := headerLabelTextWithIcon(label, icon)
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

func (m Model) headerTestStatus() (string, testStatus, bool) {
	if m.scriptError != nil {
		return "error", testStatusError, true
	}
	if len(m.testResults) == 0 {
		return "", "", false
	}
	failures := 0
	for _, result := range m.testResults {
		if !result.Passed {
			failures++
		}
	}
	if failures > 0 {
		return fmt.Sprintf("%d fail", failures), testStatusFail, true
	}
	return fmt.Sprintf("%d pass", len(m.testResults)), testStatusPass, true
}

func headerTestIcon(status testStatus) string {
	switch status {
	case testStatusPass:
		return "✔"
	case testStatusFail:
		return "✗"
	case testStatusError:
		return "⚠"
	default:
		return headerIconFor("tests")
	}
}

func (m Model) renderStatusBar() string {
	status, level := m.statusBarMessage()
	ly := m.statusBarLayout(status, level)
	return m.theme.StatusBar.Render(m.renderStatusBarLine(ly))
}

func (m Model) statusBarLayout(status string, level statusLevel) statusBarLayout {
	width := maxInt(m.width-2, 1)
	right := m.statusBarRight(width)
	frame := newStatusBarFrame(width, right.text)

	segs := m.statusBarSegments()
	ctx := statusBarSegmentsText(segs)
	frame.fit(status, ctx)

	return statusBarLayout{
		status: status,
		level:  level,
		frame:  frame,
		left:   statusBarLeftFor(status, level, segs, frame.maxLeft),
		right:  right,
	}
}

func newStatusBarFrame(width int, right string) statusBarFrame {
	rightWidth := lipgloss.Width(right)
	minGap := 1
	if rightWidth == 0 || width <= rightWidth {
		minGap = 0
	}

	leftAvailable := width
	maxLeft := width
	if statusBarLeftMaxRatio > 0 && statusBarLeftMaxRatio < 1 {
		ratioWidth := int(math.Round(float64(width) * statusBarLeftMaxRatio))
		if ratioWidth < maxLeft {
			maxLeft = ratioWidth
		}
	}
	if rightWidth > 0 {
		available := width - rightWidth - minGap
		if minGap == 0 {
			available = width - rightWidth
		}
		if available < 0 {
			available = 0
		}
		leftAvailable = available
		if available < maxLeft {
			maxLeft = available
		}
	}

	return statusBarFrame{
		width:         width,
		leftAvailable: leftAvailable,
		maxLeft:       maxLeft,
		minGap:        minGap,
	}
}

func (f *statusBarFrame) fit(status, ctx string) {
	ctxWidth := lipgloss.Width(ctx)
	if ctxWidth > 0 {
		if ctxWidth > f.leftAvailable {
			f.maxLeft = f.leftAvailable
		} else if ctxWidth > f.maxLeft {
			f.maxLeft = ctxWidth
		}
	}
	if status != "" && ctxWidth > 0 {
		minRequired := ctxWidth + statusBarSepWidth + lipgloss.Width("…")
		if minRequired <= f.leftAvailable && f.maxLeft < minRequired {
			f.maxLeft = minRequired
		}
	}
	if f.maxLeft > f.leftAvailable {
		f.maxLeft = f.leftAvailable
	}
	if f.maxLeft < 0 {
		f.maxLeft = 0
	}
}

func statusBarLeftFor(
	status string,
	level statusLevel,
	segs []statusBarSeg,
	width int,
) statusBarLeft {
	msg := status
	ctx := statusBarSegmentsText(segs)
	ctxTruncated := false

	switch {
	case width <= 0:
		msg = ""
		ctx = ""
	case ctx != "":
		ctxWidth := lipgloss.Width(ctx)
		if ctxWidth > width {
			ctx = truncateToWidth(ctx, width)
			ctxTruncated = true
			msg = ""
			break
		}
		available := width - ctxWidth
		if available < 0 {
			available = 0
		}
		if msg != "" {
			if available > statusBarSepWidth {
				msg = truncateToWidth(msg, available-statusBarSepWidth)
			} else {
				msg = ""
			}
		}
	default:
		msg = truncateToWidth(msg, width)
	}

	left := statusBarLeft{
		msg:          msg,
		level:        level,
		ctx:          ctx,
		ctxTruncated: ctxTruncated,
		segs:         segs,
	}
	if left.String() == "" && width > 0 {
		left = statusBarLeft{
			msg:   truncateToWidth(status, width),
			level: level,
		}
	}
	return left
}

func (m Model) renderStatusBarLine(ly statusBarLayout) string {
	left := ly.fitLeft()
	if ly.right.text != "" {
		return m.renderStatusBarLineWithRight(ly, left)
	}
	if left.String() == "" {
		left = statusBarLeft{
			msg:   truncateToWidth(ly.status, ly.frame.width),
			level: ly.level,
		}
	}
	return m.renderStatusBarLeft(left)
}

func (ly statusBarLayout) fitLeft() statusBarLeft {
	left := ly.left
	if ly.frame.maxLeft <= 0 {
		return left
	}
	text := left.String()
	truncated := truncateToWidth(text, ly.frame.maxLeft)
	if truncated == text {
		return left
	}
	return statusBarLeft{
		msg:   truncated,
		level: ly.level,
	}
}

func (m Model) renderStatusBarLineWithRight(ly statusBarLayout, left statusBarLeft) string {
	leftText := left.String()
	leftWidth := lipgloss.Width(leftText)
	rightWidth := lipgloss.Width(ly.right.text)
	if leftWidth == 0 {
		pad := maxInt(ly.frame.width-rightWidth, 0)
		if ly.frame.minGap > 0 && pad > ly.frame.width-rightWidth-ly.frame.minGap {
			pad = ly.frame.width - rightWidth - ly.frame.minGap
			if pad < 0 {
				pad = 0
			}
		}
		return strings.Repeat(" ", pad) + ly.right.view
	}

	gap := ly.frame.width - rightWidth - leftWidth
	if gap < 0 {
		gap = 0
	}
	if ly.frame.minGap > 0 && gap < ly.frame.minGap {
		gap = ly.frame.minGap
	}
	return m.renderStatusBarLeft(left) + strings.Repeat(" ", gap) + ly.right.view
}

func (m Model) statusBarMessage() (string, statusLevel) {
	if m.statusMessage.text != "" {
		return m.statusMessage.text, m.statusMessage.level
	}
	switch {
	case m.fileMissing:
		return "File missing on disk", statusWarn
	case m.fileStale:
		return "File changed on disk", statusWarn
	case m.dirty:
		return "Unsaved changes", statusWarn
	default:
		return "Ready", statusInfo
	}
}

func (m Model) statusBarSegments() []statusBarSeg {
	segs := make([]statusBarSeg, 0, 4)
	if m.currentFile != "" {
		segs = append(segs, statusBarSeg{key: "File", val: filepath.Base(m.currentFile)})
	}
	segs = append(segs, statusBarSeg{key: "Focus", val: m.focusLabel()})
	if m.focus == focusEditor {
		segs = append(segs, statusBarSeg{key: "Mode", val: m.editorModeLabel()})
	}
	if m.zoomActive {
		segs = append(segs, statusBarSeg{
			key: "Zoom",
			val: m.collapsedStatusLabel(m.zoomRegion),
		})
	}
	return segs
}

func (m Model) editorModeLabel() string {
	switch {
	case m.editorInsertMode:
		return "INSERT"
	case m.editor.isVisualLineMode():
		return "VISUAL LINE"
	case m.editor.isVisualMode():
		return "VISUAL"
	default:
		return "VIEW"
	}
}

func (m Model) statusBarRight(width int) statusBarPart {
	version := strings.TrimSpace(m.cfg.Version)
	if version == "" {
		version = strings.TrimSpace(m.updateVersion)
	}
	if version != "" {
		version = truncateToWidth(version, width)
	}

	type part struct {
		text string
		view string
	}
	parts := make([]part, 0, 2)
	if min := m.minimizedStatusText(); min != "" {
		parts = append(parts, part{text: ansi.Strip(min), view: min})
	}
	if version != "" {
		parts = append(parts, part{
			text: version,
			view: m.theme.StatusBarValue.Render(version),
		})
	}

	texts := make([]string, 0, len(parts))
	views := make([]string, 0, len(parts))
	for _, p := range parts {
		texts = append(texts, p.text)
		views = append(views, p.view)
	}
	text := strings.Join(texts, "  ")
	view := strings.Join(views, "  ")
	if lipgloss.Width(text) > width {
		text = truncateToWidth(text, width)
		view = m.theme.StatusBarValue.Render(text)
	}
	return statusBarPart{text: text, view: view}
}

func (m Model) renderStatusBarLeft(left statusBarLeft) string {
	parts := make([]string, 0, 2)
	if left.msg != "" {
		parts = append(parts, m.statusBarMessageStyle(left.level).Render(left.msg))
	}
	if left.ctx != "" {
		parts = append(parts, m.renderStatusBarContext(left.ctx, left.ctxTruncated, left.segs))
	}
	return strings.Join(parts, statusBarSep)
}

func (m Model) renderStatusBarContext(text string, truncated bool, segs []statusBarSeg) string {
	if truncated || len(segs) == 0 {
		return m.theme.StatusBarValue.Render(text)
	}
	parts := make([]string, 0, len(segs))
	for _, seg := range segs {
		if seg.key == "" {
			parts = append(parts, m.theme.StatusBarValue.Render(seg.val))
			continue
		}
		key := m.theme.StatusBarKey.Render(seg.key + ":")
		val := m.theme.StatusBarValue.Render(seg.val)
		parts = append(parts, key+" "+val)
	}
	return strings.Join(parts, statusBarSep)
}

func (m Model) statusBarMessageStyle(level statusLevel) lipgloss.Style {
	switch level {
	case statusWarn:
		return m.statusBarFg(
			m.theme.StatusBarKey,
			statusWarnLightColor,
			statusWarnDarkColor,
		)
	case statusError:
		return m.statusBarFg(
			m.theme.Error,
			statusErrorLightColor,
			statusErrorDarkColor,
		)
	case statusSuccess:
		return m.statusBarFg(
			m.theme.Success,
			statusSuccessLightColor,
			statusSuccessDarkColor,
		)
	default:
		return m.statusBarFg(
			m.theme.StatusBarInfo,
			statusInfoLightColor,
			statusInfoDarkColor,
		)
	}
}

func (m Model) statusBarFg(st lipgloss.Style, light, dark string) lipgloss.Style {
	if fg := st.GetForeground(); theme.ColorDefined(fg) {
		return lipgloss.NewStyle().Foreground(fg)
	}
	return lipgloss.NewStyle().
		Foreground(theme.ColorForAppearance(m.themeRuntime.appearance, light, dark))
}

func (s statusBarSeg) String() string {
	if s.key == "" {
		return s.val
	}
	return s.key + ": " + s.val
}

func (l statusBarLeft) String() string {
	parts := make([]string, 0, 2)
	if l.msg != "" {
		parts = append(parts, l.msg)
	}
	if l.ctx != "" {
		parts = append(parts, l.ctx)
	}
	return strings.Join(parts, statusBarSep)
}

func statusBarSegmentsText(segs []statusBarSeg) string {
	parts := make([]string, 0, len(segs))
	for _, seg := range segs {
		parts = append(parts, seg.String())
	}
	return strings.Join(parts, statusBarSep)
}

func (m Model) minimizedStatusText() string {
	if !m.sidebarCollapsed && !m.editorCollapsed && !m.responseCollapsed {
		return ""
	}
	items := []struct {
		on    bool
		label string
	}{
		{m.sidebarCollapsed, "Nav"},
		{m.editorCollapsed, "Editor"},
		{m.responseCollapsed, "Resp"},
	}
	marker := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#3BD671")).
		Bold(true).
		Render("●")
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if !item.on {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %s", marker, item.label))
	}
	return strings.Join(parts, "  ")
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
		lipgloss.WithWhitespaceForeground(m.themeRuntime.modalBackdropColor(m.theme)),
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
	bodyStyle := lipgloss.NewStyle().
		Padding(0, 2).
		Width(contentWidth)
	if m.themeRuntime.isLight() {
		bodyStyle = bodyStyle.Inherit(theme.ActiveTextStyle(m.theme))
	}
	if vp := m.historyPreviewViewport; vp != nil {
		wrapped := wrapPreformattedContent(body, viewWidth)
		vp.SetContent(wrapped)
		vp.Width = viewWidth
		vp.Height = bodyHeight
		bodyView = bodyStyle.Render(vp.View())
	} else {
		bodyView = bodyStyle.Render(wrapPreformattedContent(body, viewWidth))
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
		lipgloss.WithWhitespaceForeground(m.themeRuntime.modalBackdropColor(m.theme)),
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
		lipgloss.WithWhitespaceForeground(m.themeRuntime.modalBackdropColor(m.theme)),
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
		lipgloss.WithWhitespaceForeground(m.themeRuntime.modalBackdropColor(m.theme)),
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
		lipgloss.WithWhitespaceForeground(m.themeRuntime.modalBackdropColor(m.theme)),
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
	info := fmt.Sprintf(
		"%s Reload    %s Dismiss",
		m.theme.CommandBarHint.Render(reloadKey),
		m.theme.CommandBarHint.Render("Esc"),
	)
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
		lipgloss.WithWhitespaceForeground(m.themeRuntime.modalBackdropColor(m.theme)),
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
		lipgloss.WithWhitespaceForeground(m.themeRuntime.modalBackdropColor(m.theme)),
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

	header := func(text string, align lipgloss.Position) string {
		return m.theme.HeaderTitle.
			Width(viewWidth).
			Align(align).
			Render(text)
	}
	title := header("Key Bindings", lipgloss.Center)
	subtitle := m.theme.HeaderValue.
		Width(viewWidth).
		Align(lipgloss.Center).
		Render("/ search • Esc clear/close • ↑/↓ scroll • PgUp/PgDn page")
	filterView := m.renderHelpFilter(viewWidth)

	top := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		subtitle,
		"",
		filterView,
		"",
	)
	topView := lipgloss.NewStyle().
		Padding(0, 2).
		Width(contentWidth).
		Render(top)

	bodyHeight := maxBodyHeight - lipgloss.Height(topView)
	if bodyHeight > 28 {
		bodyHeight = 28
	}
	if bodyHeight < 6 {
		bodyHeight = 6
	}

	sections := m.filteredHelpSections()
	rows := make([]string, 0, len(sections)*8)
	if len(sections) == 0 {
		rows = append(rows, m.theme.HeaderValue.Render("No help entries match the current filter."))
	} else {
		for idx, section := range sections {
			rows = append(rows, header(section.title, lipgloss.Left))
			for _, entry := range section.entries {
				rows = append(rows, helpRow(m, entry.key, entry.description))
			}
			if idx < len(sections)-1 {
				rows = append(rows, "")
			}
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
			Height(bodyHeight).
			Render(vp.View())
	} else {
		bodyView = lipgloss.NewStyle().
			Padding(0, 2).
			Width(contentWidth).
			Height(bodyHeight).
			Render(body)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, topView, bodyView)
	box := m.theme.BrowserBorder.Width(width).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(m.themeRuntime.modalBackdropColor(m.theme)),
	)
}

func (m Model) renderHelpFilter(width int) string {
	if width < 16 {
		width = 16
	}
	m.helpFilter.Width = width
	input := lipgloss.NewStyle().
		Width(width).
		Render(m.helpFilter.View())

	hintText := "Type to filter the help • Enter done • Esc clear/close"
	if !m.helpFilter.Focused() {
		hintText = "/ or Shift+F to search • Esc clear/close"
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		input,
		m.themeRuntime.helpHintStyle(m.theme).Width(width).Render(hintText),
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
		style := m.themeRuntime.modalOptionStyle(m.theme).Bold(false)
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
		lipgloss.WithWhitespaceForeground(m.themeRuntime.modalBackdropColor(m.theme)),
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
			Render("Enter a path to a request, RTS, env file, or a folder"),
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
		lipgloss.WithWhitespaceForeground(m.themeRuntime.modalBackdropColor(m.theme)),
	)
}

func (m Model) renderResponseSaveModal() string {
	width := minInt(m.width-10, 72)
	if width < 40 {
		width = 40
	}
	bg := m.themeRuntime.modalInputBackground(m.theme)
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
		lipgloss.WithWhitespaceForeground(m.themeRuntime.modalBackdropColor(m.theme)),
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

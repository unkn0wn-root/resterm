package ui

import (
	"strings"
	"unicode/utf8"

	udiff "github.com/aymanbagabas/go-udiff"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// responsePaneID identifies a pane within the response area.
type responsePaneID int

const (
	responsePanePrimary responsePaneID = iota
	responsePaneSecondary
)

// responseSnapshot captures rendered response strings for presentation.
type responseSnapshot struct {
	id      string
	pretty  string
	raw     string
	headers string
	ready   bool
}

// responsePaneState tracks UI state for a single response pane.
type responsePaneState struct {
	viewport       viewport.Model
	activeTab      responseTab
	lastContentTab responseTab
	followLatest   bool
	snapshot       *responseSnapshot
	wrapCache      map[responseTab]cachedWrap
}

func newResponsePaneState(vp viewport.Model, followLatest bool) responsePaneState {
	return responsePaneState{
		viewport:       vp,
		activeTab:      responseTabPretty,
		lastContentTab: responseTabPretty,
		followLatest:   followLatest,
		wrapCache:      make(map[responseTab]cachedWrap),
	}
}

func (pane *responsePaneState) invalidateCaches() {
	for k := range pane.wrapCache {
		pane.wrapCache[k] = cachedWrap{}
	}
}

func (pane *responsePaneState) invalidateCacheFor(tab responseTab) {
	pane.wrapCache[tab] = cachedWrap{}
}

func (pane *responsePaneState) setActiveTab(tab responseTab) {
	pane.activeTab = tab
	if tab == responseTabPretty || tab == responseTabRaw || tab == responseTabHeaders {
		pane.lastContentTab = tab
	}
}

func (pane *responsePaneState) ensureContentTab() responseTab {
	switch pane.lastContentTab {
	case responseTabPretty, responseTabRaw, responseTabHeaders:
		return pane.lastContentTab
	default:
		return responseTabPretty
	}
}

func (m *Model) responseTargetPane() responsePaneID {
	if !m.responseSplit {
		return responsePanePrimary
	}
	switch m.responseLastFocused {
	case responsePaneSecondary:
		return responsePaneSecondary
	default:
		return responsePanePrimary
	}
}

func (m *Model) setLivePane(id responsePaneID) {
	if !m.responseSplit {
		id = responsePanePrimary
	}
	if id != responsePanePrimary && id != responsePaneSecondary {
		id = responsePanePrimary
	}
	m.responseLastFocused = id
	if pane := m.pane(responsePanePrimary); pane != nil {
		pane.followLatest = id == responsePanePrimary || !m.responseSplit
	}
	if m.responseSplit {
		if pane := m.pane(responsePaneSecondary); pane != nil {
			pane.followLatest = id == responsePaneSecondary
		}
	}
}

func (m *Model) syncResponsePanes() tea.Cmd {
	var cmds []tea.Cmd
	for _, id := range m.visiblePaneIDs() {
		if cmd := m.syncResponsePane(id); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	switch len(cmds) {
	case 0:
		return nil
	case 1:
		return cmds[0]
	default:
		return tea.Batch(cmds...)
	}
}

func (m *Model) pane(id responsePaneID) *responsePaneState {
	if id < 0 || int(id) >= len(m.responsePanes) {
		return nil
	}
	return &m.responsePanes[int(id)]
}

func (m *Model) visiblePaneIDs() []responsePaneID {
	if m.responseSplit {
		return []responsePaneID{responsePanePrimary, responsePaneSecondary}
	}
	return []responsePaneID{responsePanePrimary}
}

func (m *Model) otherPane(id responsePaneID) *responsePaneState {
	switch id {
	case responsePanePrimary:
		return m.pane(responsePaneSecondary)
	case responsePaneSecondary:
		return m.pane(responsePanePrimary)
	default:
		return nil
	}
}

func (m *Model) focusedPane() *responsePaneState {
	return m.pane(m.responsePaneFocus)
}

func (m *Model) ensurePaneFocusValid() {
	if !m.responseSplit {
		m.responsePaneFocus = responsePanePrimary
		return
	}
	if m.responsePaneFocus != responsePanePrimary && m.responsePaneFocus != responsePaneSecondary {
		m.responsePaneFocus = responsePanePrimary
	}
}

func (m *Model) responseContentWidth() int {
	primary := m.pane(responsePanePrimary)
	width := 0
	if primary != nil {
		width = primary.viewport.Width
	}
	if m.responseSplit {
		secondary := m.pane(responsePaneSecondary)
		if m.responseSplitOrientation == responseSplitVertical {
			if secondary != nil {
				width += responseSplitSeparatorWidth + secondary.viewport.Width
			}
		} else {
			if secondary != nil && secondary.viewport.Width > width {
				width = secondary.viewport.Width
			}
		}
	}
	if width <= 0 {
		width = defaultResponseViewportWidth
	}
	return width
}

func (m *Model) responseContentHeight() int {
	primary := m.pane(responsePanePrimary)
	height := 0
	if primary != nil {
		height = primary.viewport.Height
	}
	if m.responseSplit && m.responseSplitOrientation == responseSplitHorizontal {
		if secondary := m.pane(responsePaneSecondary); secondary != nil {
			height += responseSplitSeparatorHeight + secondary.viewport.Height
		}
	}
	if height <= 0 {
		height = defaultResponseViewportWidth
	}
	return height
}

func (m *Model) syncResponsePane(id responsePaneID) tea.Cmd {
	pane := m.pane(id)
	if pane == nil {
		return nil
	}

	m.ensurePaneActiveTabValid(pane)

	tab := pane.activeTab
	if tab == responseTabHistory {
		return nil
	}

	width := pane.viewport.Width
	if width <= 0 {
		width = defaultResponseViewportWidth
	}
	height := pane.viewport.Height

	content, cacheKey := m.paneContentForTab(id, tab)
	if content == "" {
		centered := centerContent(noResponseMessage, width, height)
		wrapped := wrapToWidth(centered, width)
		pane.wrapCache[cacheKey] = cachedWrap{width: width, content: wrapped, valid: true}
		pane.viewport.SetContent(wrapped)
		return nil
	}

	cache := pane.wrapCache[cacheKey]
	if cache.valid && cache.width == width {
		pane.viewport.SetContent(cache.content)
		return nil
	}

	var wrapped string
	if cacheKey == responseTabDiff {
		wrapped = wrapDiffContent(content, width)
	} else {
		wrapped = wrapToWidth(content, width)
	}
	pane.wrapCache[cacheKey] = cachedWrap{width: width, content: wrapped, valid: true}
	pane.viewport.SetContent(wrapped)
	return nil
}

func (m *Model) paneContentForTab(id responsePaneID, tab responseTab) (string, responseTab) {
	pane := m.pane(id)
	if pane == nil {
		return "", tab
	}
	snapshot := pane.snapshot
	if snapshot == nil {
		return "", tab
	}
	if !snapshot.ready {
		return m.responseLoadingMessage(), tab
	}

	switch tab {
	case responseTabPretty:
		return ensureTrailingNewline(snapshot.pretty), tab
	case responseTabRaw:
		return ensureTrailingNewline(snapshot.raw), tab
	case responseTabHeaders:
		if snapshot.headers == "" {
			return "<no headers>\n", tab
		}
		return ensureTrailingNewline(snapshot.headers), tab
	case responseTabDiff:
		baseTab := pane.ensureContentTab()
		if diff, ok := m.computeDiffFor(id, baseTab); ok {
			return diff, tab
		}
		return "Diff unavailable", tab
	default:
		return "", tab
	}
}

func (m *Model) computeDiffFor(id responsePaneID, baseTab responseTab) (string, bool) {
	leftPane := m.pane(id)
	rightPane := m.otherPane(id)
	if leftPane == nil || rightPane == nil {
		return "", false
	}
	left := leftPane.snapshot
	right := rightPane.snapshot
	if left == nil || right == nil {
		return "", false
	}
	if !left.ready || !right.ready {
		return "", false
	}

	leftLabel := "pane-primary"
	rightLabel := "pane-secondary"
	if id == responsePaneSecondary {
		leftLabel, rightLabel = rightLabel, leftLabel
	}

	var sections []string
	appendDiff := func(title, lhs, rhs, lhsLabel, rhsLabel string) {
		leftContent := ensureTrailingNewline(lhs)
		rightContent := ensureTrailingNewline(rhs)
		if leftContent == rightContent {
			return
		}
		diff := udiff.Unified(lhsLabel, rhsLabel, leftContent, rightContent)
		if strings.TrimSpace(diff) == "" {
			sections = append(sections, "Responses differ but diff is empty")
			return
		}
		if title != "" {
			sections = append(sections, title)
		}
		sections = append(sections, diff)
	}

	switch baseTab {
	case responseTabRaw:
		appendDiff("", left.raw, right.raw, leftLabel, rightLabel)
	case responseTabHeaders:
		// Always include the response body diff when users land here from Headers.
		appendDiff("", left.pretty, right.pretty, leftLabel, rightLabel)
		leftHeaders := left.headers
		if leftHeaders == "" {
			leftHeaders = "<no headers>\n"
		}
		rightHeaders := right.headers
		if rightHeaders == "" {
			rightHeaders = "<no headers>\n"
		}
		appendDiff("Headers", leftHeaders, rightHeaders, leftLabel+" headers", rightLabel+" headers")
	default:
		appendDiff("", left.pretty, right.pretty, leftLabel, rightLabel)
	}

	if len(sections) == 0 {
		return "Responses are identical", true
	}
	combined := strings.Join(sections, "\n\n")
	return colorizeDiff(combined), true
}

func colorizeDiff(diff string) string {
	lines := strings.Split(diff, "\n")
	green := lipgloss.NewStyle().Foreground(lipgloss.Color("#44C25B"))
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("#F25F5C"))
	hunk := lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Bold(true)
	meta := lipgloss.NewStyle().Foreground(lipgloss.Color("#A6A1BB")).Italic(true)

	var builder strings.Builder
	for i, line := range lines {
		styled := line
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			styled = meta.Render(line)
		case strings.HasPrefix(line, "@@"):
			styled = hunk.Render(line)
		case strings.HasPrefix(line, "+"):
			styled = green.Render(line)
		case strings.HasPrefix(line, "-"):
			styled = red.Render(line)
		}
		builder.WriteString(styled)
		if i < len(lines)-1 {
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

func ensureTrailingNewline(content string) string {
	if content == "" {
		return "\n"
	}
	if strings.HasSuffix(content, "\n") {
		return content
	}
	return content + "\n"
}

func wrapDiffContent(content string, width int) string {
	if width <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		segments := wrapDiffLine(line, width)
		wrapped = append(wrapped, segments...)
	}
	return strings.Join(wrapped, "\n")
}

func wrapDiffLine(line string, width int) []string {
	if width <= 0 {
		return []string{line}
	}
	if line == "" {
		return []string{""}
	}
	if visibleWidth(line) <= width {
		return []string{line}
	}
	marker, markerWidth, remainder, ok := splitDiffMarker(line)
	if !ok {
		return wrapLineSegments(line, width)
	}
	if markerWidth >= width {
		return wrapLineSegments(line, width)
	}
	segmentWidth := width - markerWidth
	if segmentWidth <= 0 {
		return wrapLineSegments(line, width)
	}
	segments := wrapLineSegments(remainder, segmentWidth)
	if len(segments) == 0 {
		return []string{marker}
	}
	result := make([]string, len(segments))
	for i, seg := range segments {
		result[i] = marker + seg
	}
	return result
}

func splitDiffMarker(line string) (marker string, markerWidth int, remainder string, ok bool) {
	if line == "" {
		return "", 0, "", false
	}
	index := 0
	for index < len(line) {
		if loc := ansiSequenceRegex.FindStringIndex(line[index:]); loc != nil && loc[0] == 0 {
			index += loc[1]
			continue
		}
		break
	}
	if index >= len(line) {
		return "", 0, line, false
	}
	r, size := utf8.DecodeRuneInString(line[index:])
	if size <= 0 {
		return "", 0, line, false
	}
	switch r {
	case '+', '-', ' ':
		marker = line[:index+size]
		remainder = line[index+size:]
		markerWidth = visibleWidth(marker)
		return marker, markerWidth, remainder, true
	default:
		return "", 0, line, false
	}
}

func (m *Model) ensurePaneActiveTabValid(pane *responsePaneState) {
	tabs := m.availableResponseTabs()
	if len(tabs) == 0 {
		pane.setActiveTab(responseTabPretty)
		return
	}
	if indexOfResponseTab(tabs, pane.activeTab) == -1 {
		fallback := pane.ensureContentTab()
		if indexOfResponseTab(tabs, fallback) == -1 {
			fallback = tabs[0]
		}
		pane.setActiveTab(fallback)
	}
}

func (m *Model) disableResponseSplit() tea.Cmd {
	if !m.responseSplit {
		return nil
	}
	m.responseSplit = false
	m.responseSplitOrientation = responseSplitVertical
	m.responsePaneFocus = responsePanePrimary
	m.setLivePane(responsePanePrimary)
	if secondary := m.pane(responsePaneSecondary); secondary != nil {
		secondary.snapshot = nil
		secondary.invalidateCaches()
	}
	if primary := m.pane(responsePanePrimary); primary != nil {
		primary.wrapCache[responseTabDiff] = cachedWrap{}
	}
	cmd := m.applyLayout()
	status := func() tea.Msg {
		return statusMsg{text: "Response split disabled", level: statusInfo}
	}
	if cmd != nil {
		return tea.Batch(cmd, status)
	}
	return status
}

func (m *Model) enableResponseSplit(orientation responseSplitOrientation) tea.Cmd {
	wasSplit := m.responseSplit
	previousOrientation := m.responseSplitOrientation
	m.responseSplit = true
	m.responseSplitOrientation = orientation
	m.ensurePaneFocusValid()
	if !wasSplit {
		if secondary := m.pane(responsePaneSecondary); secondary != nil {
			secondary.snapshot = m.responseLatest
			secondary.invalidateCaches()
			secondary.setActiveTab(responseTabPretty)
		}
	}
	if wasSplit {
		m.setLivePane(m.responseTargetPane())
	} else {
		m.setLivePane(m.responsePaneFocus)
	}
	var statusText string
	switch orientation {
	case responseSplitHorizontal:
		if wasSplit && previousOrientation != orientation {
			statusText = "Response split switched to horizontal"
		} else {
			statusText = "Response split enabled (horizontal)"
		}
	default:
		if wasSplit && previousOrientation != orientation {
			statusText = "Response split switched to vertical"
		} else {
			statusText = "Response split enabled (vertical)"
		}
	}
	cmd := m.applyLayout()
	status := func() tea.Msg {
		return statusMsg{text: statusText, level: statusInfo}
	}
	if cmd != nil {
		return tea.Batch(cmd, status)
	}
	return status
}

func (m *Model) toggleResponseSplitVertical() tea.Cmd {
	if m.responseSplit && m.responseSplitOrientation == responseSplitVertical {
		return m.disableResponseSplit()
	}
	return m.enableResponseSplit(responseSplitVertical)
}

func (m *Model) toggleResponseSplitHorizontal() tea.Cmd {
	if m.responseSplit && m.responseSplitOrientation == responseSplitHorizontal {
		return m.disableResponseSplit()
	}
	return m.enableResponseSplit(responseSplitHorizontal)
}

func (m *Model) togglePaneFollowLatest(id responsePaneID) tea.Cmd {
	pane := m.pane(id)
	if pane == nil {
		return nil
	}

	pane.followLatest = !pane.followLatest
	var note string
	if pane.followLatest {
		pane.snapshot = m.responseLatest
		note = "Pane now following latest responses"
		m.setLivePane(id)
	} else {
		note = "Pane pinned to current response"
		if m.responseLastFocused == id {
			if m.responseSplit {
				alt := responsePanePrimary
				if id == responsePanePrimary {
					alt = responsePaneSecondary
				}
				m.setLivePane(alt)
			} else {
				m.setLivePane(responsePanePrimary)
			}
		}
	}
	pane.invalidateCaches()
	for _, otherID := range m.visiblePaneIDs() {
		if other := m.pane(otherID); other != nil {
			other.wrapCache[responseTabDiff] = cachedWrap{}
		}
	}

	if pane.snapshot == nil {
		width := pane.viewport.Width
		if width <= 0 {
			width = defaultResponseViewportWidth
		}
		centered := centerContent(noResponseMessage, width, pane.viewport.Height)
		pane.viewport.SetContent(wrapToWidth(centered, width))
	} else if !pane.snapshot.ready {
		pane.viewport.SetContent(m.responseLoadingMessage())
	}
	pane.viewport.GotoTop()

	syncCmd := m.syncResponsePane(id)
	status := func() tea.Msg {
		return statusMsg{text: note, level: statusInfo}
	}
	if syncCmd != nil {
		return tea.Batch(syncCmd, status)
	}
	return status
}

func (m *Model) focusResponsePane(id responsePaneID) {
	if !m.responseSplit && id == responsePaneSecondary {
		return
	}
	m.responsePaneFocus = id
	m.setLivePane(id)
}

func (m *Model) cycleResponsePaneFocus(forward bool) {
	if !m.responseSplit {
		m.responsePaneFocus = responsePanePrimary
		m.setLivePane(responsePanePrimary)
		return
	}
	if forward {
		if m.responsePaneFocus == responsePanePrimary {
			m.responsePaneFocus = responsePaneSecondary
		} else {
			m.responsePaneFocus = responsePanePrimary
		}
	} else {
		if m.responsePaneFocus == responsePaneSecondary {
			m.responsePaneFocus = responsePanePrimary
		} else {
			m.responsePaneFocus = responsePaneSecondary
		}
	}
	m.setLivePane(m.responsePaneFocus)
}

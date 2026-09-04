package ui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/bindings"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

const (
	headerBrandName = ">_ RESTERM"
	headerGroupSep  = " │ "
	headerCellSep   = "  "
	headerMetricSep = " · "
	headerAnchorSep = " │ "
)

const (
	iconHeaderWorkspace  = "▣"
	iconHeaderEnv        = "⬢"
	iconHeaderRequests   = "⇄"
	iconHeaderActive     = "›"
	iconHeaderMock       = statusBarMockIcon
	labelHeaderEnv       = "env"
	labelHeaderRequests  = "req"
	labelHeaderWorkspace = "workspace"
	labelHeaderMock      = "mock"
	headerLabelNone      = ""
)

type testStatus string

const (
	testStatusPass  testStatus = "pass"
	testStatusFail  testStatus = "fail"
	testStatusError testStatus = "error"
)

// headerCell is a header segment before rendering. labels and values each carry
// the same information from longest to shortest, so a cell can give up words
// before it gives up characters. A labels list ending in headerLabelNone drops
// the label entirely and leaves the icon to stand for it. separator is the glue
// drawn before the cell, so it survives whichever neighbour the fitter keeps.
type headerCell struct {
	icon      string
	labels    []string
	values    []string
	style     lipgloss.Style
	priority  headerPriority
	separator string
}

// Each style includes the header background because ANSI resets clear it.
type headerStyles struct {
	bg     lipgloss.TerminalColor
	brand  lipgloss.Style
	icon   lipgloss.Style
	label  lipgloss.Style
	value  lipgloss.Style
	pass   lipgloss.Style
	warn   lipgloss.Style
	fail   lipgloss.Style
	help   lipgloss.Style
	fill   lipgloss.Style
	group  string
	sep    string
	metric string
	anchor string
}

func headerStyleOnBackground(
	st lipgloss.Style,
	bg lipgloss.TerminalColor,
) lipgloss.Style {
	if theme.ColorDefined(bg) {
		return st.Background(bg)
	}
	return st
}

func (m *Model) headerStyles() headerStyles {
	bg := m.theme.Header.GetBackground()
	on := func(st lipgloss.Style) lipgloss.Style { return headerStyleOnBackground(st, bg) }
	separator := on(m.theme.HeaderSeparator)
	warn := lipgloss.NewStyle().
		Foreground(foregroundColor(m.theme.HeaderWarn, headerWarnFallback)).
		Bold(true)
	return headerStyles{
		bg:     bg,
		brand:  m.theme.HeaderBrand,
		icon:   on(m.theme.HeaderIcon),
		label:  on(m.theme.HeaderLabel),
		value:  on(m.theme.HeaderValue.Bold(true)),
		pass:   on(m.theme.Success.Bold(true)),
		warn:   on(warn),
		fail:   on(m.theme.Error.Bold(true)),
		help:   on(m.theme.HeaderHelp),
		fill:   on(lipgloss.NewStyle()),
		group:  separator.Render(headerGroupSep),
		sep:    separator.Render(headerCellSep),
		metric: separator.Render(headerMetricSep),
		anchor: separator.Render(headerAnchorSep),
	}
}

func (s headerStyles) forLevel(level statusLevel) lipgloss.Style {
	switch level {
	case statusSuccess:
		return s.pass
	case statusWarn:
		return s.warn
	default:
		return s.fail
	}
}

func (s headerStyles) forTests(status testStatus) lipgloss.Style {
	if status == testStatusPass {
		return s.pass
	}
	return s.fail
}

func (s headerStyles) cell(c headerCell, label, value string) string {
	prefix := s.icon.Render(c.icon)
	if label != "" {
		prefix += s.label.Render(" " + label)
	}
	return prefix + s.label.Render(" ") + c.style.Render(value)
}

// cellVariants renders a cell widest first: every label wording against the
// widest value, then the shortest label against each narrower value. Both lists
// must run widest to narrowest, since the fitter needs each step to save room.
func (s headerStyles) cellVariants(c headerCell, limit int) ([]string, int) {
	values, lossy := headerCellVariants(c.values, limit)
	labels := c.labels
	if len(labels) == 0 {
		labels = []string{""}
	}

	short := labels[len(labels)-1]
	text := make([]string, 0, len(labels)-1+len(values))
	for _, label := range labels[:len(labels)-1] {
		text = append(text, s.cell(c, label, values[0]))
	}
	for _, value := range values {
		text = append(text, s.cell(c, short, value))
	}

	// Shorter label wordings say the same thing, so truncation still starts at
	// the same value, just further down the list. A lossy of 0 stays 0: values[0]
	// is already truncated, so every label wording of it is too.
	if lossy > 0 {
		lossy += len(labels) - 1
	}
	return text, lossy
}

func (m *Model) renderHeader() string {
	styles := m.headerStyles()
	cells := []headerCell{m.headerEnvCell(styles), m.headerWorkspaceCell(styles)}
	if m.activeMockServer() != nil {
		cells = append(cells, m.headerMockCell(styles))
	}
	cells = append(cells, m.headerRequestsCell(styles))
	if request := m.headerRequestTitle(); request != "" {
		cells = append(cells, m.headerActiveCell(styles, request))
	}
	if cell, ok := m.headerTestsCell(styles); ok {
		cells = append(cells, cell)
	}

	totalWidth := max(m.width, 1)
	contentWidth := headerContentWidth(totalWidth, m.theme.Header)
	limit := headerValueCap(contentWidth)

	segments := make([]headerSegment, 0, len(cells)+1)
	segments = append(segments, headerSegment{
		text:     []string{styles.brand.Render(headerBrandName)},
		priority: headerPriorityBrand,
	})
	for _, cell := range cells {
		text, lossy := styles.cellVariants(cell, limit)
		segments = append(segments, headerSegment{
			text:      text,
			lossy:     lossy,
			priority:  cell.priority,
			separator: cell.separator,
		})
	}

	anchors := m.renderHeaderAnchors(styles, contentWidth-headerGap-minHeaderWidth(segments))
	headerLine := buildHeaderLine(
		segments,
		styles.sep,
		anchors,
		lipgloss.NewStyle(),
		styles.fill,
		contentWidth,
	)
	band := m.theme.Header
	return band.Width(max(totalWidth-band.GetHorizontalBorderSize(), 1)).Render(headerLine) + "\n" +
		headerRule(m.theme.PaneDivider.Faint(true), totalWidth)
}

func headerRule(st lipgloss.Style, width int) string {
	if width < 3 {
		return dividerLine(st, width)
	}
	return st.Render(" " + strings.Repeat("─", width-2) + " ")
}

func (m *Model) headerEnvCell(styles headerStyles) headerCell {
	return headerCell{
		icon:      iconHeaderEnv,
		labels:    []string{labelHeaderEnv},
		values:    m.headerEnvVariants(),
		style:     styles.value,
		priority:  headerPriorityEnv,
		separator: styles.group,
	}
}

func (m *Model) headerWorkspaceCell(styles headerStyles) headerCell {
	workspace := filepath.Base(m.ws.root)
	if workspace == "" {
		workspace = "."
	}
	return headerCell{
		icon:     iconHeaderWorkspace,
		labels:   []string{labelHeaderWorkspace, headerLabelNone},
		values:   []string{workspace},
		style:    styles.value,
		priority: headerPriorityWorkspace,
	}
}

func (m *Model) headerMockCell(styles headerStyles) headerCell {
	return headerCell{
		icon:     iconHeaderMock,
		labels:   []string{labelHeaderMock, headerLabelNone},
		values:   headerMockVariants(m.mockSources()),
		style:    styles.value,
		priority: headerPriorityMock,
	}
}

func (m *Model) headerRequestsCell(styles headerStyles) headerCell {
	return headerCell{
		icon:     iconHeaderRequests,
		labels:   []string{labelHeaderRequests},
		values:   []string{strconv.Itoa(len(m.requestItems))},
		style:    styles.value,
		priority: headerPriorityRequests,
	}
}

func (m *Model) headerTestsCell(styles headerStyles) (headerCell, bool) {
	summary, status, ok := m.headerTestStatus()
	if !ok {
		return headerCell{}, false
	}
	return headerCell{
		icon:     headerTestIcon(status),
		values:   []string{summary},
		style:    styles.forTests(status),
		priority: headerPriorityTests,
	}, true
}

func (m *Model) headerRequestTitle() string {
	if request := requestBaseTitle(m.currentRequest); strings.TrimSpace(request) != "" {
		return request
	}
	return strings.TrimSpace(m.activeRequestTitle)
}

func (m *Model) headerActiveCell(styles headerStyles, request string) headerCell {
	cell := headerCell{
		icon:      iconHeaderActive,
		values:    []string{request},
		style:     styles.value,
		priority:  headerPriorityActive,
		separator: styles.fill.Render(" "),
	}
	if m.currentRequest != nil {
		cell.style = cell.style.Foreground(methodColor(m.theme, m.currentRequest.Method))
	}
	return cell
}

func (m *Model) renderHeaderAnchors(styles headerStyles, limit int) string {
	readings := m.renderLatencyOn(styles.bg)
	if status := m.renderTransportStatus(styles); status != "" {
		reading := status + styles.metric + readings
		if lipgloss.Width(reading) <= limit {
			readings = reading
		}
	}

	rest := limit - lipgloss.Width(readings) - lipgloss.Width(styles.anchor)
	if rest <= 0 {
		return readings
	}

	help := m.helpAnchor(styles, rest)
	if help == "" {
		return readings
	}
	return readings + styles.anchor + help
}

// Hide the label when Help would use more than half of the available space.
func (m *Model) helpAnchor(styles headerStyles, limit int) string {
	hint := m.commandActionHint(bindings.ActionToggleHelp, "Help")
	if hint.key == "" {
		return ""
	}

	key := styles.help.Render(hint.key)
	cell := key + styles.label.Render(" "+hint.label)
	if limit <= 0 {
		return cell
	}
	if lipgloss.Width(cell) > limit/2 {
		cell = key
	}
	if lipgloss.Width(cell) > limit {
		return ""
	}
	return cell
}

func (m *Model) renderTransportStatus(styles headerStyles) string {
	if m.headerTransport.label == "" {
		return ""
	}
	return styles.forLevel(m.headerTransport.level).Render(m.headerTransport.label)
}

func (m *Model) headerTestStatus() (string, testStatus, bool) {
	if m.scriptError != nil {
		return "error", testStatusError, true
	}
	if len(m.testResults) == 0 {
		return "", "", false
	}
	if fails := countTestFailures(m.testResults); fails > 0 {
		return fmt.Sprintf("%d fail", fails), testStatusFail, true
	}
	return fmt.Sprintf("%d pass", len(m.testResults)), testStatusPass, true
}

// U+FE0E forces text (not color-emoji) presentation; macOS clips the emoji glyph.
const iconTestPass = "✔\ufe0e"

func headerTestIcon(status testStatus) string {
	switch status {
	case testStatusPass:
		return iconTestPass
	case testStatusFail:
		return "✗"
	default:
		return "!"
	}
}

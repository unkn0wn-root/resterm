package ui

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/gitstatus"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

const (
	statusBarVersionIcon   = "◇"
	statusBarHTTPFileIcon  = "⇄"
	statusBarEditorIcon    = "▣"
	statusBarViewIcon      = "□"
	statusBarInsertIcon    = "▸"
	statusBarVisualIcon    = "◫"
	statusBarGitIcon       = "⎇"
	statusBarHorizontalPad = 1
	statusBarSectionPad    = 1
	statusBarMinLeftWidth  = 12
)

type statusBarSeg struct {
	key statusBarSegmentKind
	val string
}

type statusBarSegmentKind string

const (
	statusBarSegmentFile      statusBarSegmentKind = "File"
	statusBarSegmentFocus     statusBarSegmentKind = "Focus"
	statusBarSegmentMode      statusBarSegmentKind = "Mode"
	statusBarSegmentEditorPos statusBarSegmentKind = "EditorPos"
	statusBarSegmentZoom      statusBarSegmentKind = "Zoom"
)

type statusBarSection struct {
	text       string
	style      theme.StatusBarSegmentStyle
	runs       []styledRun
	valueStyle lipgloss.Style
}

type styledRun struct {
	text  string
	style lipgloss.Style
}

func styledRunsText(runs []styledRun) string {
	var b strings.Builder
	for _, run := range runs {
		b.WriteString(run.text)
	}
	return b.String()
}

func (m Model) renderStatusBar() string {
	status, level := m.statusBarMessage()
	width := max(m.width, 1)
	inset := statusBarUsesOuterInset(width)
	contentWidth := width
	if inset {
		contentWidth -= statusBarHorizontalPad * 2
	}
	palette := statusBarPalette(m.theme.StatusBarPalette)
	line := m.renderStatusBarLine(status, level, contentWidth, palette)
	if inset {
		line = insetStatusBarLine(line, palette)
	}
	return line
}

func (m Model) renderStatusBarLine(
	status string,
	level statusLevel,
	width int,
	palette theme.StatusBarPalette,
) string {
	if width <= 0 {
		return ""
	}

	leftSections := m.statusBarLeftSections(status, level, palette)
	rightLimit := width - statusBarLeftReserve(leftSections, width)
	if rightLimit < 0 {
		rightLimit = 0
	}
	right := fitStatusBarSections(
		m.statusBarRightSections(palette),
		rightLimit,
	)
	rightWidth := statusBarSectionsWidth(right)
	left := fitStatusBarSections(
		leftSections,
		width-rightWidth,
	)
	leftView := renderStatusBarSections(left)
	rightView := renderStatusBarSections(right)

	gap := width - lipgloss.Width(leftView) - lipgloss.Width(rightView)
	if gap < 0 {
		gap = 0
	}
	return leftView + renderStatusBarBaseFill(gap, palette) + rightView
}

func statusBarUsesOuterInset(width int) bool {
	return width > statusBarHorizontalPad*2
}

func insetStatusBarLine(line string, palette theme.StatusBarPalette) string {
	pad := renderStatusBarBaseFill(statusBarHorizontalPad, palette)
	return pad + line + pad
}

func renderStatusBarBaseFill(width int, palette theme.StatusBarPalette) string {
	if width <= 0 {
		return ""
	}
	fill := strings.Repeat(" ", width)
	if !theme.ColorDefined(palette.Base) {
		return fill
	}
	return lipgloss.NewStyle().
		Background(palette.Base).
		Render(fill)
}

func statusBarPalette(palette theme.StatusBarPalette) theme.StatusBarPalette {
	defaults := theme.DefaultStatusBarPalette()
	palette.Info = statusBarSegmentStyle(palette.Info, defaults.Info)
	palette.Warn = statusBarSegmentStyle(palette.Warn, defaults.Warn)
	palette.Error = statusBarSegmentStyle(palette.Error, defaults.Error)
	palette.Success = statusBarSegmentStyle(palette.Success, defaults.Success)
	palette.File = statusBarSegmentStyle(palette.File, defaults.File)
	palette.Focus = statusBarSegmentStyle(palette.Focus, defaults.Focus)
	palette.Mode = statusBarSegmentStyle(palette.Mode, defaults.Mode)
	palette.Zoom = statusBarSegmentStyle(palette.Zoom, defaults.Zoom)
	palette.Minimized = statusBarSegmentStyle(palette.Minimized, defaults.Minimized)
	palette.Version = statusBarSegmentStyle(palette.Version, defaults.Version)
	palette.User = statusBarSegmentStyle(palette.User, defaults.User)
	palette.Host = statusBarSegmentStyle(palette.Host, defaults.Host)
	return palette
}

func statusBarSegmentStyle(
	style theme.StatusBarSegmentStyle,
	fallback theme.StatusBarSegmentStyle,
) theme.StatusBarSegmentStyle {
	if !theme.ColorDefined(style.Foreground) {
		style.Foreground = fallback.Foreground
	}
	if !theme.ColorDefined(style.Background) {
		style.Background = fallback.Background
	}
	return style
}

func (m Model) statusBarMessage() (string, statusLevel) {
	if m.statusMessage.text != "" {
		return m.statusMessage.text, m.statusMessage.level
	}
	switch {
	case m.dirty:
		return "Unsaved changes", statusWarn
	case m.fileMissing:
		return "File missing on disk", statusWarn
	case m.fileStale:
		return "File changed on disk", statusWarn
	default:
		return "Ready", statusInfo
	}
}

func (m Model) statusBarSegments() []statusBarSeg {
	segs := make([]statusBarSeg, 0, 5)
	if m.currentFile != "" {
		segs = append(segs, statusBarSeg{
			key: statusBarSegmentFile,
			val: filepath.Base(m.currentFile),
		})
	}
	segs = append(segs, statusBarSeg{key: statusBarSegmentFocus, val: m.focusLabel()})
	if m.focus == focusEditor {
		segs = append(segs, statusBarSeg{key: statusBarSegmentMode, val: m.editorModeLabel()})
		segs = append(
			segs,
			statusBarSeg{key: statusBarSegmentEditorPos, val: m.editorPositionLabel()},
		)
	}
	if m.zoomActive {
		segs = append(segs, statusBarSeg{
			key: statusBarSegmentZoom,
			val: m.collapsedStatusLabel(m.zoomRegion),
		})
	}
	return segs
}

func (m Model) editorPositionLabel() string {
	line := m.editor.Line() + 1
	total := max(m.editor.LineCount(), 1)
	col := m.editor.LineInfo().ColumnOffset + 1
	return fmt.Sprintf("Ln %d/%d Col %d", line, total, col)
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

func (m Model) statusBarLeftSections(
	status string,
	level statusLevel,
	palette theme.StatusBarPalette,
) []statusBarSection {
	segs := []statusBarSection{{
		text:  strings.TrimSpace(status),
		style: statusBarStatusStyle(level, palette),
	}}
	if section, ok := m.statusBarTestSection(palette); ok {
		segs = append(segs, section)
	}

	for _, item := range m.statusBarSegments() {
		section, ok := m.statusBarContextSection(item, palette)
		if !ok {
			continue
		}
		segs = append(segs, section)
	}
	return segs
}

func (m Model) statusBarTestSection(
	palette theme.StatusBarPalette,
) (statusBarSection, bool) {
	if m.statusMessage.testSummary == "" {
		return statusBarSection{}, false
	}
	return statusBarSection{
		text:  m.statusMessage.testSummary,
		style: statusBarStatusStyle(m.statusMessage.testLevel, palette),
	}, true
}

func (m Model) statusBarRightSections(
	palette theme.StatusBarPalette,
) []statusBarSection {
	segs := make([]statusBarSection, 0, 4)
	if min := strings.TrimSpace(ansi.Strip(m.minimizedStatusText())); min != "" {
		segs = append(segs, statusBarSection{
			text:  min,
			style: palette.Minimized,
		})
	}
	gitValueStyle := statusBarModeInlineStyle(m.theme.StatusBarValue)
	if runs := m.statusBarGitRuns(gitValueStyle); len(runs) > 0 {
		segs = append(segs, statusBarSection{
			text:       styledRunsText(runs),
			runs:       runs,
			valueStyle: gitValueStyle,
		})
	}
	if version := m.statusBarVersion(); version != "" {
		segs = append(segs, statusBarSection{
			text:  statusBarVersionText(version),
			style: palette.Version,
		})
	}
	if m.statusUser != "" {
		segs = append(segs, statusBarSection{
			text:  m.statusUser,
			style: palette.User,
		})
	}
	if m.statusHost != "" {
		segs = append(segs, statusBarSection{
			text:  m.statusHost,
			style: palette.Host,
		})
	}
	return segs
}

func (m Model) statusBarGitRuns(valueStyle lipgloss.Style) []styledRun {
	if m.gitStatus.RepoRoot == "" {
		return nil
	}

	counts := m.gitStatus.Counts()
	branch := strings.TrimSpace(m.gitStatus.Branch)
	if branch == "(detached)" {
		branch = "detached"
	}
	if branch == "" && !counts.Any() && m.gitStatus.Ahead == 0 && m.gitStatus.Behind == 0 {
		return nil
	}

	colors := m.theme.GitColors
	runs := []styledRun{{text: statusBarGitIcon, style: statusBarGitForeground(colors.Branch)}}
	add := func(text string, color lipgloss.Color) {
		runs = append(runs,
			styledRun{text: " ", style: valueStyle},
			styledRun{text: text, style: statusBarGitForeground(color)},
		)
	}
	addCount := func(status gitstatus.Status, count int, color lipgloss.Color) {
		if count > 0 {
			add(fmt.Sprintf("%s%d", status.Label(), count), color)
		}
	}

	if branch != "" {
		add(branch, colors.Branch)
	}
	// Counts are listed most- to least-severe, mirroring gitstatus.Status priority.
	addCount(gitstatus.StatusConflict, counts.Conflict, colors.Conflict)
	addCount(gitstatus.StatusDeleted, counts.Deleted, colors.Deleted)
	addCount(gitstatus.StatusRenamed, counts.Renamed, colors.Renamed)
	addCount(gitstatus.StatusAdded, counts.Added, colors.Added)
	addCount(gitstatus.StatusModified, counts.Modified, colors.Modified)
	addCount(gitstatus.StatusUntracked, counts.Untracked, colors.Untracked)
	if m.gitStatus.Ahead > 0 {
		add(fmt.Sprintf("↑%d", m.gitStatus.Ahead), colors.Branch)
	}
	if m.gitStatus.Behind > 0 {
		add(fmt.Sprintf("↓%d", m.gitStatus.Behind), colors.Branch)
	}
	return runs
}

func (m Model) statusBarVersion() string {
	version := strings.TrimSpace(m.cfg.Version)
	if version == "" {
		version = strings.TrimSpace(m.updateVersion)
	}
	return version
}

func statusBarVersionText(version string) string {
	if version == "" {
		return ""
	}
	return statusBarVersionIcon + " " + version
}

func statusBarContextText(seg statusBarSeg) string {
	val := seg.val
	if val == "" {
		return ""
	}
	switch seg.key {
	case statusBarSegmentFile:
		return statusBarHTTPFileIcon + " " + val
	case statusBarSegmentFocus:
		if strings.EqualFold(val, "Editor") {
			return statusBarEditorIcon + " " + val
		}
		return val
	case statusBarSegmentMode:
		return statusBarModeText(val)
	case "", statusBarSegmentZoom, statusBarSegmentEditorPos:
		return val
	default:
		return string(seg.key) + ": " + val
	}
}

func statusBarModeText(mode string) string {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return ""
	}
	icon := statusBarModeIcon(mode)
	if icon == "" {
		return mode
	}
	return icon + " " + mode
}

func statusBarModeIcon(mode string) string {
	switch strings.ToUpper(strings.TrimSpace(mode)) {
	case "INSERT":
		return statusBarInsertIcon
	case "VIEW":
		return statusBarViewIcon
	case "VISUAL", "VISUAL LINE":
		return statusBarVisualIcon
	default:
		return ""
	}
}

func (m Model) statusBarContextSection(
	seg statusBarSeg,
	palette theme.StatusBarPalette,
) (statusBarSection, bool) {
	if seg.key == statusBarSegmentMode {
		return m.statusBarModeSection(seg), true
	}
	text := statusBarContextText(seg)
	if text == "" {
		return statusBarSection{}, false
	}
	if seg.key == statusBarSegmentEditorPos {
		return m.statusBarEditorPosSection(text, palette), true
	}
	return statusBarSection{
		text:  text,
		style: statusBarContextStyle(seg.key, palette),
	}, true
}

func (m Model) statusBarModeSection(seg statusBarSeg) statusBarSection {
	mode := seg.val
	valueStyle := statusBarModeInlineStyle(m.theme.StatusBarValue)
	runs := []styledRun{{text: mode, style: valueStyle}}
	if icon := statusBarModeIcon(mode); icon != "" {
		runs = []styledRun{
			{text: icon, style: statusBarModeInlineStyle(m.theme.StatusBarKey)},
			{text: " " + mode, style: valueStyle},
		}
	}
	return statusBarSection{
		text:       styledRunsText(runs),
		runs:       runs,
		valueStyle: valueStyle,
	}
}

func (m Model) statusBarEditorPosSection(
	text string,
	palette theme.StatusBarPalette,
) statusBarSection {
	if editor := palette.Editor; theme.ColorDefined(editor.Foreground) ||
		theme.ColorDefined(editor.Background) {
		return statusBarSection{text: text, style: editor}
	}
	style := statusBarModeInlineStyle(m.theme.StatusBarValue).Faint(true)
	return statusBarSection{
		text:       text,
		runs:       []styledRun{{text: text, style: style}},
		valueStyle: style,
	}
}

func statusBarModeInlineStyle(style lipgloss.Style) lipgloss.Style {
	return style.UnsetBackground()
}

func statusBarStatusStyle(
	level statusLevel,
	palette theme.StatusBarPalette,
) theme.StatusBarSegmentStyle {
	switch level {
	case statusWarn:
		return palette.Warn
	case statusError:
		return palette.Error
	case statusSuccess:
		return palette.Success
	default:
		return palette.Info
	}
}

func statusBarContextStyle(
	key statusBarSegmentKind,
	palette theme.StatusBarPalette,
) theme.StatusBarSegmentStyle {
	switch key {
	case statusBarSegmentFile:
		return palette.File
	case statusBarSegmentFocus:
		return palette.Focus
	case statusBarSegmentMode:
		return palette.Mode
	case statusBarSegmentZoom:
		return palette.Zoom
	default:
		return palette.Focus
	}
}

func statusBarLeftReserve(segs []statusBarSection, width int) int {
	if width <= 0 {
		return 0
	}
	fullWidth := statusBarSectionsWidth(compactStatusBarSections(segs))
	if fullWidth == 0 {
		return 0
	}

	reserve := width * 2 / 3
	if reserve < statusBarMinLeftWidth {
		reserve = statusBarMinLeftWidth
	}
	if reserve > width {
		return width
	}
	if fullWidth <= reserve {
		return fullWidth
	}
	return reserve
}

func fitStatusBarSections(
	segs []statusBarSection,
	width int,
) []statusBarSection {
	if width <= 0 {
		return nil
	}
	out := compactStatusBarSections(segs)
	for len(out) > 0 {
		if statusBarSectionsWidth(out) <= width {
			return out
		}
		before := statusBarSectionsWidth(out)
		truncateLastStatusBarSection(out, width)
		if statusBarSectionsWidth(out) <= width {
			return out
		}
		if len(out) == 1 {
			return nil
		}
		if statusBarSectionsWidth(out) == before {
			out = out[:len(out)-1]
			continue
		}
		out = out[:len(out)-1]
	}
	return nil
}

func compactStatusBarSections(
	segs []statusBarSection,
) []statusBarSection {
	out := make([]statusBarSection, 0, len(segs))
	for _, seg := range segs {
		seg.text = strings.TrimSpace(seg.text)
		if seg.text == "" {
			continue
		}
		out = append(out, seg)
	}
	return out
}

func truncateLastStatusBarSection(
	segs []statusBarSection,
	width int,
) {
	if len(segs) == 0 {
		return
	}
	over := statusBarSectionsWidth(segs) - width
	if over <= 0 {
		return
	}
	last := &segs[len(segs)-1]
	target := lipgloss.Width(last.text) - over
	if target < 1 {
		target = 1
	}
	last.text = truncateToWidth(last.text, target)
	if len(last.runs) > 0 {
		// A mid-token cut can't preserve per-token colours; collapse to one run.
		last.runs = []styledRun{{text: last.text, style: last.valueStyle}}
	}
}

func statusBarSectionsWidth(segs []statusBarSection) int {
	width := 0
	for _, seg := range segs {
		text := strings.TrimSpace(seg.text)
		if text == "" {
			continue
		}
		width += lipgloss.Width(text) + statusBarSectionPad*2
	}
	return width
}

func renderStatusBarSections(segs []statusBarSection) string {
	if len(segs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, seg := range segs {
		b.WriteString(renderStatusBarSection(seg))
	}
	return b.String()
}

func renderStatusBarSection(seg statusBarSection) string {
	if len(seg.runs) > 0 {
		return renderStatusBarStyledSection(seg)
	}
	return lipgloss.NewStyle().
		Foreground(seg.style.Foreground).
		Background(seg.style.Background).
		Render(statusBarSectionContent(seg.text))
}

func renderStatusBarStyledSection(seg statusBarSection) string {
	pad := seg.valueStyle.Render(strings.Repeat(" ", statusBarSectionPad))
	var b strings.Builder
	b.WriteString(pad)
	for _, run := range seg.runs {
		b.WriteString(run.style.Render(run.text))
	}
	b.WriteString(pad)
	return b.String()
}

func statusBarGitForeground(color lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(color).
		Bold(true)
}

func statusBarSectionContent(text string) string {
	pad := strings.Repeat(" ", statusBarSectionPad)
	return pad + strings.TrimSpace(text) + pad
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
	marker := "●"
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if !item.on {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %s", marker, item.label))
	}
	return strings.Join(parts, "  ")
}

func currentStatusIdentity() (string, string) {
	return currentStatusUsername(), currentStatusHost()
}

func currentStatusUsername() string {
	if u, err := user.Current(); err == nil && u != nil {
		if name := cleanStatusUsername(u.Username); name != "" {
			return name
		}
	}

	for _, v := range []string{os.Getenv("USER"), os.Getenv("USERNAME")} {
		if name := cleanStatusUsername(v); name != "" {
			return name
		}
	}
	return ""
}

func currentStatusHost() string {
	if name, err := os.Hostname(); err == nil {
		return cleanStatusHost(name)
	}
	return ""
}

func cleanStatusUsername(s string) string {
	if i := strings.LastIndexAny(s, `\/`); i >= 0 {
		s = s[i+1:]
	}
	return strings.TrimSpace(s)
}

func cleanStatusHost(s string) string {
	s = strings.TrimSpace(s)
	if name, _, ok := strings.Cut(s, "."); ok && name != "" {
		return name
	}
	return s
}

package ui

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/theme"
)

const (
	statusBarVersionIcon  = "◇"
	statusBarHTTPFileIcon = "⇄"
	statusBarEditorIcon   = "▣"
	statusBarViewIcon     = "□"
	statusBarInsertIcon   = "▸"
	statusBarVisualIcon   = "◫"
	statusBarSectionPad   = 1
	statusBarMinLeftWidth = 12
)

type statusBarSeg struct {
	key statusBarSegmentKind
	val string
}

type statusBarSegmentKind string

const (
	statusBarSegmentFile  statusBarSegmentKind = "File"
	statusBarSegmentFocus statusBarSegmentKind = "Focus"
	statusBarSegmentMode  statusBarSegmentKind = "Mode"
	statusBarSegmentZoom  statusBarSegmentKind = "Zoom"
)

type statusBarSection struct {
	text  string
	style theme.StatusBarSegmentStyle
}

func (m Model) renderStatusBar() string {
	status, level := m.statusBarMessage()
	width := max(m.width, 1)
	palette := statusBarPalette(m.theme.StatusBarPalette)
	line := m.renderStatusBarLine(status, level, width, palette)
	return lipgloss.NewStyle().
		Background(palette.Base).
		Width(width).
		Render(line)
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
	return leftView + strings.Repeat(" ", gap) + rightView
}

func statusBarPalette(palette theme.StatusBarPalette) theme.StatusBarPalette {
	defaults := theme.DefaultStatusBarPalette()
	if !theme.ColorDefined(palette.Base) {
		palette.Base = defaults.Base
	}
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
	segs := make([]statusBarSeg, 0, 4)
	if m.currentFile != "" {
		segs = append(segs, statusBarSeg{
			key: statusBarSegmentFile,
			val: filepath.Base(m.currentFile),
		})
	}
	segs = append(segs, statusBarSeg{key: statusBarSegmentFocus, val: m.focusLabel()})
	if m.focus == focusEditor {
		segs = append(segs, statusBarSeg{key: statusBarSegmentMode, val: m.editorModeLabel()})
	}
	if m.zoomActive {
		segs = append(segs, statusBarSeg{
			key: statusBarSegmentZoom,
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

func (m Model) statusBarLeftSections(
	status string,
	level statusLevel,
	palette theme.StatusBarPalette,
) []statusBarSection {
	segs := []statusBarSection{{
		text:  strings.TrimSpace(status),
		style: statusBarStatusStyle(level, palette),
	}}

	for _, item := range m.statusBarSegments() {
		text := statusBarContextText(item)
		if text == "" {
			continue
		}
		segs = append(segs, statusBarSection{
			text:  text,
			style: statusBarContextStyle(item.key, palette),
		})
	}
	return segs
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
	val := strings.TrimSpace(seg.val)
	key := statusBarSegmentKind(strings.TrimSpace(string(seg.key)))
	if val == "" {
		return ""
	}
	switch key {
	case statusBarSegmentFile:
		return statusBarHTTPFileIcon + " " + val
	case statusBarSegmentFocus:
		if strings.EqualFold(val, "Editor") {
			return statusBarEditorIcon + " " + val
		}
		return val
	case statusBarSegmentMode:
		return statusBarModeText(val)
	case "", statusBarSegmentZoom:
		return val
	default:
		return string(key) + ": " + val
	}
}

func statusBarModeText(mode string) string {
	switch strings.ToUpper(mode) {
	case "INSERT":
		return statusBarInsertIcon + " " + mode
	case "VIEW":
		return statusBarViewIcon + " " + mode
	case "VISUAL", "VISUAL LINE":
		return statusBarVisualIcon + " " + mode
	default:
		return mode
	}
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
	return lipgloss.NewStyle().
		Foreground(seg.style.Foreground).
		Background(seg.style.Background).
		Render(statusBarSectionContent(seg.text))
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

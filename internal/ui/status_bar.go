package ui

import (
	"fmt"
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
)

const (
	statusWarnLightColor  = "#d97706"
	statusWarnDarkColor   = "#FACC15"
	statusErrorLightColor = "#dc2626"
	statusErrorDarkColor  = "#FF6E6E"
)

type statusBarSeg struct {
	key string
	val string
}

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

	right := fitStatusBarSections(
		m.statusBarRightSections(palette),
		width,
	)
	rightWidth := statusBarSectionsWidth(right)
	left := fitStatusBarSections(
		m.statusBarLeftSections(status, level, palette),
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
	key := strings.TrimSpace(seg.key)
	if val == "" {
		return ""
	}
	switch key {
	case "File":
		return statusBarHTTPFileIcon + " " + val
	case "Focus":
		if strings.EqualFold(val, "Editor") {
			return statusBarEditorIcon + " " + val
		}
		return val
	case "Mode":
		return statusBarModeText(val)
	case "", "Zoom":
		return val
	default:
		return key + ": " + val
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
	key string,
	palette theme.StatusBarPalette,
) theme.StatusBarSegmentStyle {
	switch key {
	case "File":
		return palette.File
	case "Focus":
		return palette.Focus
	case "Mode":
		return palette.Mode
	case "Zoom":
		return palette.Zoom
	default:
		return palette.Focus
	}
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

func (m Model) statusBarFg(st lipgloss.Style, light, dark string) lipgloss.Style {
	if fg := st.GetForeground(); theme.ColorDefined(fg) {
		return lipgloss.NewStyle().Foreground(fg)
	}
	return lipgloss.NewStyle().
		Foreground(theme.ColorForAppearance(m.themeRuntime.appearance, light, dark))
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

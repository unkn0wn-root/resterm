package ui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/ui/scroll"
)

type responseSearchState struct {
	query      string
	isRegex    bool
	matches    []searchMatch
	index      int
	active     bool
	tab        responseTab
	snapshotID string
	width      int
	computed   bool
}

func (s *responseSearchState) invalidate() {
	s.matches = nil
	s.index = -1
	s.active = false
	s.snapshotID = ""
	s.width = 0
	s.computed = false
}

func (s *responseSearchState) markStale() {
	s.computed = false
}

func (s *responseSearchState) clear() bool {
	hadState := s.hasQuery() || len(s.matches) > 0 || s.active
	s.query = ""
	s.isRegex = false
	s.matches = nil
	s.index = -1
	s.active = false
	s.tab = 0
	s.snapshotID = ""
	s.width = 0
	s.computed = false
	return hadState
}

func (s *responseSearchState) hasQuery() bool {
	return strings.TrimSpace(s.query) != ""
}

func (s *responseSearchState) prepare(
	query string,
	isRegex bool,
	tab responseTab,
	snapshotID string,
	width int,
) {
	s.query = query
	s.isRegex = isRegex
	s.tab = tab
	s.snapshotID = snapshotID
	s.width = width
	s.matches = nil
	s.index = -1
	s.active = false
	s.computed = false
}

func (s *responseSearchState) needsRefresh(snapshotID string, tab responseTab, width int) bool {
	if !s.hasQuery() {
		return false
	}
	if s.snapshotID != snapshotID || s.tab != tab || s.width != width {
		return true
	}
	return !s.computed
}

func (s *responseSearchState) computeMatches(content string) error {
	if !s.hasQuery() {
		s.matches = nil
		s.index = -1
		s.active = false
		s.computed = false
		return nil
	}

	query := s.query
	if s.isRegex {
		rx, err := regexp.Compile(query)
		if err != nil {
			return err
		}
		s.matches = regexMatches(content, rx)
	} else {
		s.matches = literalMatches(content, query)
	}
	if len(s.matches) == 0 {
		s.index = -1
		s.active = false
	} else {
		s.index = 0
		s.active = true
	}
	s.computed = true
	return nil
}

func decorateResponseContent(
	content string,
	matches []searchMatch,
	highlight lipgloss.Style,
	active lipgloss.Style,
	current int,
) string {
	if len(matches) == 0 {
		return content
	}
	highlight = ensureResponseHighlight(highlight, false)
	active = ensureResponseHighlight(active, true)

	runes := []rune(content)
	if len(runes) == 0 {
		return content
	}

	var builder strings.Builder
	builder.Grow(len(content) + len(matches)*8)
	last := 0
	for idx, match := range matches {
		start := clamp(match.start, 0, len(runes))
		end := clamp(match.end, 0, len(runes))
		if end <= start {
			continue
		}
		if start > last {
			builder.WriteString(string(runes[last:start]))
		}
		style := highlight
		if current >= 0 && idx == current {
			style = active
		}
		builder.WriteString(style.Render(string(runes[start:end])))
		last = end
	}
	if last < len(runes) {
		builder.WriteString(string(runes[last:]))
	}
	return builder.String()
}

func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func ensureResponseMatchVisible(v *viewport.Model, content string, match searchMatch) {
	if v == nil {
		return
	}
	runes := []rune(content)
	if len(runes) == 0 {
		return
	}
	start := clamp(match.start, 0, len(runes))
	prefix := string(runes[:start])
	line := strings.Count(prefix, "\n")
	if line > 0 {
		line--
	}
	h := v.Height
	if h <= 0 {
		h = v.VisibleLineCount()
	}
	total := v.TotalLineCount()
	if total == 0 {
		total = strings.Count(content, "\n") + 1
	}
	target := scroll.Align(line, v.YOffset, h, total)
	v.SetYOffset(target)
}

func ensureResponseHighlight(style lipgloss.Style, active bool) lipgloss.Style {
	if !responseHighlightUnset(style) {
		return style
	}
	if active {
		return lipgloss.NewStyle().
			Background(lipgloss.Color("#FFD46A")).
			Foreground(lipgloss.Color("#1A1020")).
			Bold(true)
	}
	return lipgloss.NewStyle().
		Background(lipgloss.Color("#2C1E3A")).
		Foreground(lipgloss.Color("#E9E6FF"))
}

func responseHighlightUnset(style lipgloss.Style) bool {
	_, fgNoColor := style.GetForeground().(lipgloss.NoColor)
	_, bgNoColor := style.GetBackground().(lipgloss.NoColor)
	return fgNoColor && bgNoColor &&
		!style.GetBold() &&
		!style.GetUnderline() &&
		!style.GetReverse() &&
		!style.GetFaint() &&
		!style.GetItalic() &&
		!style.GetBlink() &&
		!style.GetStrikethrough()
}

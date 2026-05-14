package ui

import (
	"strings"
	"unicode/utf8"

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
	content    responseSearchContentIndex
}

type responseSearchBoundary struct {
	raw  int
	sgr  string
	line int
}

type responseSearchContentIndex struct {
	raw        string
	visible    string
	bounds     []responseSearchBoundary
	totalLines int
	valid      bool
}

func (s *responseSearchState) invalidate() {
	s.matches = nil
	s.index = -1
	s.active = false
	s.snapshotID = ""
	s.width = 0
	s.computed = false
	s.content = responseSearchContentIndex{}
}

func (s *responseSearchState) markStale() {
	s.computed = false
	s.content = responseSearchContentIndex{}
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
	s.content = responseSearchContentIndex{}
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
	if s.tab != tab || s.snapshotID != snapshotID || s.width != width {
		s.content = responseSearchContentIndex{}
	}
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
	return s.computeMatchesFromIndex(s.contentIndexFor(content))
}

func (s *responseSearchState) computeMatchesFromIndex(index *responseSearchContentIndex) error {
	if !s.hasQuery() {
		s.matches = nil
		s.index = -1
		s.active = false
		s.computed = false
		return nil
	}
	if index == nil {
		index = &responseSearchContentIndex{}
	}

	matches, err := buildSearchMatches(index.visible, s.query, s.isRegex)
	if err != nil {
		return err
	}
	s.matches = matches
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

func (s *responseSearchState) contentIndexFor(content string) *responseSearchContentIndex {
	if s.content.valid && s.content.raw == content {
		return &s.content
	}
	s.content = buildResponseSearchContentIndex(content)
	return &s.content
}

// decorateResponseContent applies match highlights by visible-rune offsets while
// preserving the original ANSI stream. Search matches must never split escape
// sequences, and highlight resets must restore the SGR state that was active at
// the end of the match.
func decorateResponseContent(
	content string,
	index *responseSearchContentIndex,
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

	if index == nil || !index.valid || index.raw != content {
		built := buildResponseSearchContentIndex(content)
		index = &built
	}
	bounds := index.bounds
	visibleLen := len(bounds) - 1
	if visibleLen <= 0 {
		return content
	}

	highlightPrefix, highlightSuffix := styleSGR(highlight)
	activePrefix, activeSuffix := styleSGR(active)

	var builder strings.Builder
	builder.Grow(len(content) + len(matches)*8)
	lastRaw := 0
	lastVisible := 0
	for idx, match := range matches {
		start := clamp(match.start, 0, visibleLen)
		end := clamp(match.end, 0, visibleLen)
		if end <= start {
			continue
		}
		if end <= lastVisible {
			continue
		}
		if start < lastVisible {
			start = lastVisible
		}

		rawStart := bounds[start].raw
		rawEnd := bounds[end].raw
		if rawEnd <= rawStart {
			continue
		}
		if rawStart > lastRaw {
			builder.WriteString(content[lastRaw:rawStart])
		}
		prefix := highlightPrefix
		suffix := highlightSuffix
		if current >= 0 && idx == current {
			prefix = activePrefix
			suffix = activeSuffix
		}
		builder.WriteString(applyANSIAwareWrap(content[rawStart:rawEnd], prefix, suffix, bounds[end].sgr))
		lastRaw = rawEnd
		lastVisible = end
	}
	if lastRaw < len(content) {
		builder.WriteString(content[lastRaw:])
	}
	return builder.String()
}

// responseSearchBoundaries maps visible-rune boundaries to raw byte offsets and
// active SGR prefixes. bounds[0] is always the boundary before the first visible
// rune, so len(bounds) is visible rune count plus one.
func responseSearchBoundaries(content string) []responseSearchBoundary {
	return buildResponseSearchContentIndex(content).bounds
}

func buildResponseSearchContentIndex(content string) responseSearchContentIndex {
	index := responseSearchContentIndex{
		raw:    content,
		bounds: []responseSearchBoundary{{raw: 0}},
		valid:  true,
	}
	if content == "" {
		return index
	}

	var sgr sgrState
	var visible strings.Builder
	visible.Grow(len(content))
	restore := ""
	line := 0
	for i := 0; i < len(content); {
		if content[i] == '\x1b' {
			if seq, size := ansiSequenceAt(content, i); size > 0 {
				if sgr.apply(seq) {
					restore = sgr.String()
				}
				i += size
				continue
			}
		}

		r, size := utf8.DecodeRuneInString(content[i:])
		if size <= 0 {
			size = 1
			r = rune(content[i])
		}
		visible.WriteRune(r)
		if r == '\n' {
			line++
		}
		i += size
		index.bounds = append(index.bounds, responseSearchBoundary{
			raw:  i,
			sgr:  restore,
			line: line,
		})
	}
	index.visible = visible.String()
	if index.visible != "" {
		index.totalLines = line + 1
	}
	return index
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

func ensureResponseMatchVisible(
	v *viewport.Model,
	index *responseSearchContentIndex,
	match searchMatch,
) {
	if v == nil {
		return
	}
	if index == nil || !index.valid || len(index.bounds) <= 1 {
		return
	}
	start := clamp(match.start, 0, len(index.bounds)-1)
	line := index.bounds[start].line
	h := v.Height
	if h <= 0 {
		h = v.VisibleLineCount()
	}
	total := v.TotalLineCount()
	if total == 0 {
		total = index.totalLines
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

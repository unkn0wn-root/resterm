package ui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/ui/scroll"
)

type respSel struct {
	on   bool
	a    int
	c    int
	tab  responseTab
	sid  string
	hdr  headersViewMode
	mode rawViewMode
}

func (s *respSel) clear() {
	*s = respSel{}
}

func (s respSel) rng() (int, int) {
	if !s.on {
		return 0, -1
	}
	if s.a <= s.c {
		return s.a, s.c
	}
	return s.c, s.a
}

func respTabSel(tab responseTab) bool {
	switch tab {
	case responseTabPretty, responseTabRaw, responseTabHeaders:
		return true
	default:
		return false
	}
}

func (m *Model) handleResponseSelectionKey(
	msg tea.KeyMsg,
	p *responsePaneState,
) (tea.Cmd, bool) {
	if p == nil {
		return nil, false
	}
	tab := p.activeTab
	if p.sel.on && !m.selValid(p, tab) {
		p.sel.clear()
	}
	key := msg.String()

	if !respTabSel(tab) {
		if key == "esc" && p.sel.on {
			cmd := m.clearRespSel(p)
			return cmd, true
		}
		return nil, false
	}

	switch key {
	case "v", "V":
		cmd := m.startRespSel(p)
		return cmd, true
	case "esc":
		if p.sel.on {
			cmd := m.clearRespSel(p)
			return cmd, true
		}
		return nil, false
	case "y", "c":
		if !p.sel.on {
			return statusCmd(statusInfo, "No selection to copy"), true
		}
		cmd := m.copyRespSel(p)
		return batchCommands(cmd, m.syncResponsePane(m.responsePaneFocus)), true
	}

	if !p.sel.on {
		return nil, false
	}

	switch key {
	case "down", "j", "shift+j", "J":
		return m.moveRespSel(p, 1), true
	case "up", "k", "shift+k", "K":
		return m.moveRespSel(p, -1), true
	case "pgdown":
		return m.moveRespSelWrap(p, 1), true
	case "pgup":
		return m.moveRespSelWrap(p, -1), true
	}

	return nil, false
}

func (m *Model) selValid(p *responsePaneState, tab responseTab) bool {
	if p == nil || !p.sel.on {
		return false
	}
	if !respTabSel(tab) {
		return false
	}
	if p.snapshot == nil || !p.snapshot.ready {
		return false
	}
	if p.sel.tab != tab || p.sel.sid != p.snapshot.id {
		return false
	}
	if tab == responseTabHeaders && p.sel.hdr != p.headersView {
		return false
	}
	if tab == responseTabRaw && p.sel.mode != p.snapshot.rawMode {
		return false
	}
	return true
}

func (m *Model) selCache(p *responsePaneState, tab responseTab) (cachedWrap, bool) {
	if p == nil {
		return cachedWrap{}, false
	}
	if tab == responseTabRaw {
		if p.snapshot == nil {
			return cachedWrap{}, false
		}
		if p.rawWrapCache == nil {
			return cachedWrap{}, false
		}
		cache, ok := p.rawWrapCache[p.snapshot.rawMode]
		if !ok || !cache.valid {
			return cachedWrap{}, false
		}
		return cache, true
	}
	cache, ok := p.wrapCache[tab]
	if !ok || !cache.valid {
		return cachedWrap{}, false
	}
	return cache, true
}

func (m *Model) selLineTop(p *responsePaneState, tab responseTab) (int, bool) {
	cache, ok := m.selCache(p, tab)
	if !ok || len(cache.rev) == 0 {
		return 0, false
	}
	off := p.viewport.YOffset
	if off < 0 {
		off = 0
	}
	if off >= len(cache.rev) {
		off = len(cache.rev) - 1
	}
	return cache.rev[off], true
}

func (m *Model) startRespSel(p *responsePaneState) tea.Cmd {
	if p == nil {
		return nil
	}
	tab := p.activeTab
	if !respTabSel(tab) {
		return nil
	}
	if p.snapshot == nil || !p.snapshot.ready {
		return statusCmd(statusWarn, "No response available")
	}
	line, ok := m.selLineTop(p, tab)
	if !ok {
		return statusCmd(statusWarn, "Selection unavailable")
	}
	p.sel = respSel{
		on:   true,
		a:    line,
		c:    line,
		tab:  tab,
		sid:  p.snapshot.id,
		hdr:  p.headersView,
		mode: p.snapshot.rawMode,
	}
	return m.syncResponsePane(m.responsePaneFocus)
}

func (m *Model) clearRespSel(p *responsePaneState) tea.Cmd {
	if p == nil || !p.sel.on {
		return nil
	}
	p.sel.clear()
	return m.syncResponsePane(m.responsePaneFocus)
}

func (m *Model) moveRespSel(p *responsePaneState, delta int) tea.Cmd {
	if p == nil || !m.selValid(p, p.activeTab) {
		return nil
	}
	cache, ok := m.selCache(p, p.activeTab)
	if !ok || len(cache.spans) == 0 {
		return nil
	}
	max := len(cache.spans) - 1
	line := p.sel.c + delta
	if line < 0 {
		line = 0
	}
	if line > max {
		line = max
	}
	return m.setRespSelLine(p, line, cache)
}

func (m *Model) moveRespSelWrap(p *responsePaneState, dir int) tea.Cmd {
	if p == nil || !m.selValid(p, p.activeTab) {
		return nil
	}
	cache, ok := m.selCache(p, p.activeTab)
	if !ok || len(cache.rev) == 0 || len(cache.spans) == 0 {
		return nil
	}
	step := p.viewport.Height
	if step < 1 {
		step = 1
	}
	cur := p.sel.c
	if cur < 0 {
		cur = 0
	}
	if cur >= len(cache.spans) {
		cur = len(cache.spans) - 1
	}
	span := cache.spans[cur]
	pos := span.start + (step * dir)
	if pos < 0 {
		pos = 0
	}
	if pos >= len(cache.rev) {
		pos = len(cache.rev) - 1
	}
	line := cache.rev[pos]
	return m.setRespSelLine(p, line, cache)
}

func (m *Model) setRespSelLine(
	p *responsePaneState,
	line int,
	cache cachedWrap,
) tea.Cmd {
	if p == nil || !p.sel.on {
		return nil
	}
	if len(cache.spans) == 0 {
		return nil
	}
	if line < 0 {
		line = 0
	}
	if line >= len(cache.spans) {
		line = len(cache.spans) - 1
	}
	if line == p.sel.c {
		return nil
	}
	p.sel.c = line
	span := cache.spans[line]
	total := len(cache.rev)
	off := p.viewport.YOffset
	h := p.viewport.Height
	p.viewport.SetYOffset(scroll.Reveal(span.start, span.end, off, h, total))
	p.setCurrPosition()
	return m.syncResponsePane(m.responsePaneFocus)
}

func (m *Model) copyRespSel(p *responsePaneState) tea.Cmd {
	if p == nil || !m.selValid(p, p.activeTab) {
		if p != nil {
			p.sel.clear()
		}
		return statusCmd(statusInfo, "No selection to copy")
	}
	text, ok := m.respSelText(p)
	if !ok {
		p.sel.clear()
		return statusCmd(statusInfo, "No selection to copy")
	}
	size := formatByteSize(int64(len(text)))
	msg := fmt.Sprintf("Copied selection (%s)", size)
	p.sel.clear()
	return (&m.editor).copyToClipboard(text, msg)
}

func (m *Model) respSelText(p *responsePaneState) (string, bool) {
	if p == nil || !m.selValid(p, p.activeTab) {
		return "", false
	}
	labelTab := p.activeTab
	content, _ := m.paneContentForTab(m.responsePaneFocus, labelTab)
	plain := stripANSIEscape(content)
	base := ensureTrailingNewline(plain)
	lines := strings.Split(base, "\n")
	start, end := p.sel.rng()
	if start < 0 {
		start = 0
	}
	if end >= len(lines) {
		end = len(lines) - 1
	}
	if start > end || start < 0 || end < 0 {
		return "", false
	}
	text := strings.Join(lines[start:end+1], "\n")
	return ensureTrailingNewline(text), true
}

func (m *Model) decorateResponseSelection(
	p *responsePaneState,
	tab responseTab,
	content string,
) string {
	if p == nil || !p.sel.on || !respTabSel(tab) || content == "" {
		return content
	}
	if !m.selValid(p, tab) {
		p.sel.clear()
		return content
	}
	cache, ok := m.selCache(p, tab)
	if !ok || len(cache.spans) == 0 {
		return content
	}
	start, end := p.sel.rng()
	if start < 0 {
		start = 0
	}
	if end >= len(cache.spans) {
		end = len(cache.spans) - 1
	}
	if start > end {
		return content
	}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return content
	}

	highlight := make([]bool, len(lines))
	maxLine := len(lines) - 1
	for i := start; i <= end; i++ {
		span := cache.spans[i]
		if span.end < span.start {
			continue
		}
		if span.start > maxLine {
			break
		}
		if span.end > maxLine {
			span.end = maxLine
		}
		for j := span.start; j <= span.end; j++ {
			highlight[j] = true
		}
	}

	var builder strings.Builder
	builder.Grow(len(content))
	style := m.respSelStyle(tab)
	prefix, suffix := styleSGR(style)
	if prefix == "" {
		return content
	}
	basePrefix, _ := styleSGR(m.respBaseStyle(tab))
	if basePrefix != "" {
		if suffix == "" {
			suffix = "\x1b[0m" + basePrefix
		} else {
			suffix += basePrefix
		}
	}
	for i, line := range lines {
		if highlight[i] {
			builder.WriteString(applySelectionToLine(line, prefix, suffix))
		} else {
			builder.WriteString(line)
		}
		if i < len(lines)-1 {
			builder.WriteByte('\n')
		}
	}
	return builder.String()
}

func (m *Model) respSelStyle(tab responseTab) lipgloss.Style {
	base := m.respBaseStyle(tab)
	return m.theme.ResponseSelection.Inherit(base)
}

func (m *Model) respBaseStyle(tab responseTab) lipgloss.Style {
	base := m.theme.ResponseContent
	switch tab {
	case responseTabRaw:
		base = m.theme.ResponseContentRaw.Inherit(base)
	case responseTabHeaders:
		base = m.theme.ResponseContentHeaders.Inherit(base)
	}
	return base
}

func styleSGR(style lipgloss.Style) (string, string) {
	profile := lipgloss.DefaultRenderer().ColorProfile()
	st := profile.String()

	if fg := toTermenvColor(profile, style.GetForeground()); fg != nil {
		st = st.Foreground(fg)
	}
	if bg := toTermenvColor(profile, style.GetBackground()); bg != nil {
		st = st.Background(bg)
	}
	if style.GetBold() {
		st = st.Bold()
	}
	if style.GetItalic() {
		st = st.Italic()
	}
	if style.GetUnderline() {
		st = st.Underline()
	}
	if style.GetFaint() {
		st = st.Faint()
	}
	if style.GetStrikethrough() {
		st = st.CrossOut()
	}
	if style.GetReverse() {
		st = st.Reverse()
	}
	if style.GetBlink() {
		st = st.Blink()
	}

	const sentinel = "X"
	styled := st.Styled(sentinel)
	if styled == sentinel {
		return "", ""
	}
	idx := strings.Index(styled, sentinel)
	if idx == -1 {
		return "", ""
	}
	return styled[:idx], styled[idx+len(sentinel):]
}

func toTermenvColor(profile termenv.Profile, c lipgloss.TerminalColor) termenv.Color {
	if c == nil {
		return nil
	}
	switch v := c.(type) {
	case lipgloss.NoColor:
		return nil
	case lipgloss.Color:
		return profile.Color(string(v))
	case lipgloss.ANSIColor:
		return profile.Color(strconv.FormatUint(uint64(v), 10))
	default:
		return nil
	}
}

func applySelectionToLine(line, prefix, suffix string) string {
	if prefix == "" {
		return line
	}
	if line == "" {
		return prefix + suffix
	}
	if !ansiSequenceRegex.MatchString(line) {
		return prefix + line + suffix
	}
	indices := ansiSequenceRegex.FindAllStringIndex(line, -1)
	if len(indices) == 0 {
		return prefix + line + suffix
	}
	var builder strings.Builder
	builder.Grow(len(line) + len(prefix)*(len(indices)+1) + len(suffix))
	builder.WriteString(prefix)
	last := 0
	for _, idx := range indices {
		if idx[0] > last {
			builder.WriteString(line[last:idx[0]])
		}
		seq := line[idx[0]:idx[1]]
		builder.WriteString(seq)
		if isSGR(seq) {
			builder.WriteString(prefix)
		}
		last = idx[1]
	}
	if last < len(line) {
		builder.WriteString(line[last:])
	}
	builder.WriteString(suffix)
	return builder.String()
}

func isSGR(seq string) bool {
	if len(seq) == 0 {
		return false
	}
	if seq[len(seq)-1] != 'm' {
		return false
	}
	return strings.HasPrefix(seq, "\x1b[")
}

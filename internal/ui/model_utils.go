package ui

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/ui/textarea"
)

func (m *Model) filterEditorMessage(msg tea.Msg) tea.Msg {
	if km, ok := msg.(tea.KeyMsg); ok {
		if m.editorInsertMode {
			if km.Type == tea.KeyTab {
				km.Type = tea.KeyRunes
				km.Runes = []rune{'\t'}
				return km
			}
			return msg
		}

		if km.Type == tea.KeyRunes && len(km.Runes) > 0 {
			km.Runes = nil
			return km
		}
		switch km.String() {
		case "enter", "ctrl+m", "ctrl+j", "backspace", "ctrl+h", "delete":
			km.Type = tea.KeyRunes
			km.Runes = nil
			return km
		}
	}
	return msg
}

var ansiSequenceRegex = regexp.MustCompile(
	"\x1b\\[[0-9;?]*[ -/]*[@-~]|\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)",
)

func stripANSIEscape(s string) string {
	return ansiSequenceRegex.ReplaceAllString(s, "")
}

func formatTestSummary(results []scripts.TestResult, scriptErr error) string {
	if len(results) == 0 && scriptErr == nil {
		return ""
	}

	builder := strings.Builder{}
	builder.WriteString(statsHeadingStyle.Render("Tests:") + "\n")
	if scriptErr != nil {
		errorLabel := statsWarnStyle.Render("[ERROR]")
		builder.WriteString(
			"  " + errorLabel + " " + statsMessageStyle.Render(scriptErr.Error()) + "\n",
		)
	}
	for _, result := range results {
		statusStyle := statsSuccessStyle
		statusLabel := "[PASS]"
		if !result.Passed {
			statusStyle = statsWarnStyle
			statusLabel = "[FAIL]"
		}
		line := strings.Builder{}
		line.WriteString("  ")
		line.WriteString(statusStyle.Render(statusLabel))
		if strings.TrimSpace(result.Name) != "" {
			line.WriteString(" ")
			line.WriteString(statsValueStyle.Render(result.Name))
		}
		if strings.TrimSpace(result.Message) != "" {
			line.WriteString(" – ")
			line.WriteString(statsMessageStyle.Render(result.Message))
		}
		if result.Elapsed > 0 {
			dur := result.Elapsed.Truncate(time.Millisecond)
			line.WriteString(" ")
			line.WriteString(statsDurationStyle.Render(fmt.Sprintf("(%s)", dur)))
		}
		builder.WriteString(line.String() + "\n")
	}
	return strings.TrimRight(builder.String(), "\n")
}

func buildRespSum(resp *httpclient.Response, tests []scripts.TestResult, scriptErr error) string {
	return buildRespSumWithLength(resp, tests, scriptErr, renderContentLengthLine)
}

func buildRespSumPretty(
	resp *httpclient.Response,
	tests []scripts.TestResult,
	scriptErr error,
) string {
	return buildRespSumWithLength(resp, tests, scriptErr, renderContentLengthLinePretty)
}

func buildRespSumWithLength(
	resp *httpclient.Response,
	tests []scripts.TestResult,
	scriptErr error,
	lengthFn func(*httpclient.Response) string,
) string {
	if resp == nil {
		return ""
	}

	if lengthFn == nil {
		lengthFn = renderContentLengthLine
	}

	var lines []string
	statusLine := renderStatusLine(resp.Status, resp.StatusCode)
	if statusLine != "" {
		lines = append(lines, statusLine)
	}

	if lengthLine := lengthFn(resp); lengthLine != "" {
		lines = append(lines, lengthLine)
	}

	if trimmedURL := strings.TrimSpace(resp.EffectiveURL); trimmedURL != "" {
		lines = append(lines, renderLabelValue("URL", trimmedURL, statsLabelStyle, statsValueStyle))
	}

	if resp.Headers != nil {
		if streamType := strings.TrimSpace(resp.Headers.Get(streamHeaderType)); streamType != "" {
			lines = append(
				lines,
				renderLabelValue("Stream", streamType, statsLabelStyle, statsValueStyle),
			)
		}

		if summary := strings.TrimSpace(resp.Headers.Get(streamHeaderSummary)); summary != "" {
			lines = append(
				lines,
				renderLabelValue("Stream summary", summary, statsLabelStyle, statsMessageStyle),
			)
		}
	}

	if resp.Duration > 0 {
		dur := resp.Duration.Round(time.Millisecond)
		if dur <= 0 {
			dur = resp.Duration
		}
		lines = append(
			lines,
			renderLabelValue("Duration", dur.String(), statsLabelStyle, statsDurationStyle),
		)
	}

	summary := strings.Join(lines, "\n")
	if testSummary := formatTestSummary(tests, scriptErr); testSummary != "" {
		summary = joinSections(summary, testSummary)
	}
	return summary
}

func renderStatusLine(status string, code int) string {
	trimmed := strings.TrimSpace(status)
	if trimmed == "" {
		return ""
	}
	style := selectStatusStyle(code)
	return renderLabelValue("Status", trimmed, statsLabelStyle, style)
}

type contentLen struct {
	n       int64
	raw     string
	has     bool
	numeric bool
}

func contentLength(resp *httpclient.Response) contentLen {
	if resp == nil {
		return contentLen{}
	}
	if resp.Headers != nil {
		if v := strings.TrimSpace(resp.Headers.Get("Content-Length")); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
				return contentLen{n: n, has: true, numeric: true}
			}
			return contentLen{raw: v, has: true}
		}
	}
	n := int64(len(resp.Body))
	return contentLen{n: n, has: true, numeric: true}
}

func renderContentLengthLine(resp *httpclient.Response) string {
	cl := contentLength(resp)
	if !cl.has {
		return ""
	}

	value := cl.raw
	if cl.numeric {
		value = formatByteQuantity(cl.n)
	}

	return renderLabelValue("Content-Length", value, statsLabelStyle, statsValueStyle)
}

func renderContentLengthLinePretty(resp *httpclient.Response) string {
	cl := contentLength(resp)
	if !cl.has {
		return ""
	}

	value := cl.raw
	if cl.numeric {
		value = formatByteSize(cl.n)
	}

	return renderLabelValue("Content-Length", value, statsLabelStyle, statsValueStyle)
}

func formatByteQuantity(n int64) string {
	if n == 1 {
		return "1 byte"
	}
	return fmt.Sprintf("%d bytes", n)
}

func formatByteSize(n int64) string {
	if n < 0 {
		n = 0
	}

	units := []string{"B", "KiB", "MiB", "GiB"}
	f := float64(n)
	i := 0
	for i < len(units)-1 && f >= 1024 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d %s", n, units[i])
	}

	s := fmt.Sprintf("%.1f", f)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return s + " " + units[i]
}

func selectStatusStyle(code int) lipgloss.Style {
	switch {
	case code >= 500 && code <= 599:
		return statsWarnStyle
	case code >= 400 && code <= 499:
		return statsWarnStyle
	case code >= 300 && code <= 399:
		return statsNeutralStyle
	case code > 0:
		return statsSuccessStyle
	default:
		return statsValueStyle
	}
}

func joinSections(sections ...string) string {
	var parts []string
	for _, section := range sections {
		trimmed := trimSection(section)
		if strings.TrimSpace(trimmed) == "" {
			continue
		}
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, "\n\n")
}

func trimSection(section string) string {
	if section == "" {
		return ""
	}
	return strings.Trim(section, "\r\n")
}

func makeReadOnlyKeyMap(base textarea.KeyMap) textarea.KeyMap {
	read := base
	read.DeleteAfterCursor = key.Binding{}
	read.DeleteBeforeCursor = key.Binding{}
	read.DeleteCharacterBackward = key.Binding{}
	read.DeleteCharacterForward = key.Binding{}
	read.DeleteWordBackward = key.Binding{}
	read.DeleteWordForward = key.Binding{}
	read.InsertNewline = key.Binding{}
	read.Paste = key.Binding{}
	read.LowercaseWordForward = key.Binding{}
	read.UppercaseWordForward = key.Binding{}
	read.CapitalizeWordForward = key.Binding{}
	read.TransposeCharacterBackward = key.Binding{}
	return read
}

func trimTrailingNewline(content string) string {
	if content == "" {
		return content
	}
	end := len(content)
	if end == 0 || content[end-1] != '\n' {
		return content
	}
	end--
	if end > 0 && content[end-1] == '\r' {
		end--
	}
	return content[:end]
}

func trimSyntheticNewline(content string, syn bool) string {
	if !syn {
		return content
	}
	return trimTrailingNewline(content)
}

func wrapToWidth(content string, width int) string {
	out, _ := wrapToWidthCtx(context.Background(), content, width)
	return out
}

func wrapToWidthCtx(ctx context.Context, content string, width int) (string, bool) {
	if width <= 0 {
		return content, true
	}
	if ctxDone(ctx) {
		return "", false
	}

	lines := strings.Split(content, "\n")
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		if ctxDone(ctx) {
			return "", false
		}
		segments, ok := wrapLineSegmentsCtx(ctx, line, width)
		if !ok {
			return "", false
		}
		wrapped = append(wrapped, segments...)
	}
	return strings.Join(wrapped, "\n"), true
}

func wrapContentForTab(tab responseTab, content string, width int) string {
	out, _ := wrapContentForTabCtx(context.Background(), tab, content, width)
	return out
}

func wrapContentForTabCtx(
	ctx context.Context,
	tab responseTab,
	content string,
	width int,
) (string, bool) {
	switch tab {
	case responseTabRaw:
		return wrapPreformattedContentCtx(ctx, content, width)
	case responseTabDiff:
		return wrapDiffContentCtx(ctx, content, width)
	case responseTabPretty:
		return wrapStructuredContentCtx(ctx, content, width)
	default:
		return wrapToWidthCtx(ctx, content, width)
	}
}

func wrapContentForTabMap(
	tab responseTab,
	content string,
	width int,
) (string, []lineSpan, []int) {
	out, spans, rev, _ := wrapContentForTabMapCtx(context.Background(), tab, content, width)
	return out, spans, rev
}

func wrapContentForTabMapCtx(
	ctx context.Context,
	tab responseTab,
	content string,
	width int,
) (string, []lineSpan, []int, bool) {
	if ctxDone(ctx) {
		return "", nil, nil, false
	}
	lines := strings.Split(content, "\n")
	wrapped := make([]string, 0, len(lines))
	spans := make([]lineSpan, len(lines))
	rev := make([]int, 0, len(lines))

	for i, line := range lines {
		if ctxDone(ctx) {
			return "", nil, nil, false
		}
		segments, ok := wrapLineForTabCtx(ctx, tab, line, width)
		if !ok {
			return "", nil, nil, false
		}
		if len(segments) == 0 {
			segments = []string{""}
		}
		start := len(wrapped)
		for _, seg := range segments {
			if ctxDone(ctx) {
				return "", nil, nil, false
			}
			wrapped = append(wrapped, seg)
			rev = append(rev, i)
		}
		spans[i] = lineSpan{start: start, end: len(wrapped) - 1}
	}

	return strings.Join(wrapped, "\n"), spans, rev, true
}

func wrapLineForTabCtx(
	ctx context.Context,
	tab responseTab,
	line string,
	width int,
) ([]string, bool) {
	switch tab {
	case responseTabRaw:
		return wrapPreformattedLineCtx(ctx, line, width)
	case responseTabPretty:
		return wrapStructuredLineCtx(ctx, line, width)
	case responseTabDiff:
		return wrapDiffLineCtx(ctx, line, width)
	default:
		return wrapLineSegmentsCtx(ctx, line, width)
	}
}

func wrapCache(tab responseTab, content string, width int) cachedWrap {
	cache, _ := wrapCacheCtx(context.Background(), tab, content, width)
	return cache
}

func wrapCacheCtx(
	ctx context.Context,
	tab responseTab,
	content string,
	width int,
) (cachedWrap, bool) {
	if ctxDone(ctx) {
		return cachedWrap{}, false
	}
	if !respTabSel(tab) {
		wrapped, ok := wrapContentForTabCtx(ctx, tab, content, width)
		if !ok {
			return cachedWrap{}, false
		}
		return cachedWrap{
			width:   width,
			content: wrapped,
			valid:   true,
		}, true
	}
	wrapped, spans, rev, ok := wrapContentForTabMapCtx(ctx, tab, content, width)
	if !ok {
		return cachedWrap{}, false
	}
	return cachedWrap{
		width:   width,
		content: wrapped,
		valid:   true,
		spans:   spans,
		rev:     rev,
	}, true
}

func wrapPreformattedContent(content string, width int) string {
	out, _ := wrapPreformattedContentCtx(context.Background(), content, width)
	return out
}

func wrapPreformattedContentCtx(ctx context.Context, content string, width int) (string, bool) {
	if width <= 0 {
		return content, true
	}
	if ctxDone(ctx) {
		return "", false
	}

	lines := strings.Split(content, "\n")
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		if ctxDone(ctx) {
			return "", false
		}
		segments, ok := wrapPreformattedLineCtx(ctx, line, width)
		if !ok {
			return "", false
		}
		wrapped = append(wrapped, segments...)
	}
	return strings.Join(wrapped, "\n"), true
}

func wrapPreformattedLineCtx(ctx context.Context, line string, width int) ([]string, bool) {
	if ctxDone(ctx) {
		return nil, false
	}
	if width <= 0 {
		return []string{line}, true
	}
	if line == "" {
		return []string{""}, true
	}
	if visibleWidth(line) <= width {
		return []string{line}, true
	}

	indent := leadingIndent(line)
	if indent == "" {
		return wrapLineSegmentsCtx(ctx, line, width)
	}

	indentWidth := visibleWidth(indent)
	available := width - indentWidth
	if available <= 0 {
		return wrapLineSegmentsCtx(ctx, line, width)
	}

	body := line[len(indent):]
	if body == "" {
		return []string{indent}, true
	}

	segments := make([]string, 0, (len(line)/width)+1)
	remaining := body
	for len(remaining) > 0 {
		if ctxDone(ctx) {
			return nil, false
		}
		segment, rest, ok := splitSegmentCtx(ctx, remaining, available)
		if !ok {
			return nil, false
		}
		segments = append(segments, indent+segment)
		if rest == "" || rest == remaining {
			if rest == "" {
				break
			}
			fallback, ok := wrapLineSegmentsCtx(ctx, rest, width)
			if !ok {
				return nil, false
			}
			segments = append(segments, fallback...)
			break
		}
		remaining = rest
	}
	if len(segments) == 0 {
		return []string{""}, true
	}
	return segments, true
}

func leadingIndent(line string) string {
	if line == "" {
		return ""
	}

	var builder strings.Builder
	for _, r := range line {
		if r == ' ' || r == '\t' {
			builder.WriteRune(r)
			continue
		}
		break
	}
	return builder.String()
}

func wrapLineSegments(line string, width int) []string {
	segments, _ := wrapLineSegmentsCtx(context.Background(), line, width)
	return segments
}

func wrapLineSegmentsCtx(ctx context.Context, line string, width int) ([]string, bool) {
	if ctxDone(ctx) {
		return nil, false
	}
	if width <= 0 {
		return []string{line}, true
	}
	if line == "" {
		return []string{""}, true
	}
	if visibleWidth(line) <= width {
		return []string{line}, true
	}

	tokens, ok := tokenizeLineCtx(ctx, line)
	if !ok {
		return nil, false
	}
	if len(tokens) == 0 {
		return []string{""}, true
	}

	var current strings.Builder
	segments := make([]string, 0, len(tokens))
	currentWidth := 0
	lineHasNonSpace := false

	appendSegment := func(segment string) {
		if segment == "" {
			return
		}
		trimmed := strings.TrimRight(segment, " ")
		if trimmed != "" {
			segment = trimmed
		}
		segments = append(segments, segment)
	}

	flush := func() {
		if current.Len() == 0 {
			return
		}
		appendSegment(current.String())
		current.Reset()
		currentWidth = 0
	}

	for _, tok := range tokens {
		if ctxDone(ctx) {
			return nil, false
		}
		text := tok.text
		tokWidth := tok.width
		if text == "" {
			continue
		}

		if tokWidth == 0 {
			current.WriteString(text)
			continue
		}

		if tokWidth > width {
			if currentWidth > 0 {
				remaining := width - currentWidth
				if remaining <= 0 {
					flush()
				} else {
					segment, rest, ok := splitSegmentCtx(ctx, text, remaining)
					if !ok {
						return nil, false
					}
					if segment != "" {
						current.WriteString(segment)
						currentWidth += visibleWidth(segment)
						if !tok.isSpace {
							lineHasNonSpace = true
						}
					}

					flush()
					if rest == "" || rest == text {
						continue
					}

					text = rest
					tokWidth = visibleWidth(text)
					if tokWidth == 0 {
						continue
					}
				}
			}
			if tokWidth > width {
				parts, ok := splitLongTokenCtx(ctx, text, width)
				if !ok {
					return nil, false
				}
				if !tok.isSpace {
					lineHasNonSpace = true
				}
				for _, part := range parts {
					if part == "" {
						continue
					}
					appendSegment(part)
				}
				continue
			}
		}

		if currentWidth > 0 && currentWidth+tokWidth > width {
			flush()
			if tok.isSpace && lineHasNonSpace {
				continue
			}
		}

		if currentWidth == 0 && tok.isSpace && lineHasNonSpace {
			continue
		}

		current.WriteString(text)
		currentWidth += tokWidth
		if !tok.isSpace {
			lineHasNonSpace = true
		}
	}

	if currentWidth > 0 || current.Len() > 0 {
		flush()
	}

	if len(segments) == 0 {
		return []string{""}, true
	}
	return segments, true
}

func splitSegmentCtx(ctx context.Context, s string, width int) (string, string, bool) {
	if ctxDone(ctx) {
		return "", "", false
	}
	if width <= 0 || visibleWidth(s) <= width {
		return s, "", true
	}

	var builder strings.Builder
	currentWidth := 0
	index := 0
	for index < len(s) {
		if ctxDone(ctx) {
			return "", "", false
		}
		if loc := ansiSequenceRegex.FindStringIndex(s[index:]); loc != nil && loc[0] == 0 {
			seq := s[index : index+loc[1]]
			builder.WriteString(seq)
			index += loc[1]
			continue
		}

		r, size := utf8.DecodeRuneInString(s[index:])
		if size <= 0 {
			size = 1
		}

		runeWidth := runewidth.RuneWidth(r)
		if runeWidth <= 0 {
			runeWidth = 1
		}
		if currentWidth+runeWidth > width {
			break
		}

		builder.WriteString(s[index : index+size])
		currentWidth += runeWidth
		index += size
	}

	segment := builder.String()
	rest := s[index:]
	if segment == "" && rest != "" {
		if loc := ansiSequenceRegex.FindStringIndex(rest); loc != nil && loc[0] == 0 {
			segment = rest[:loc[1]]
			rest = rest[loc[1]:]
		} else {
			_, size := utf8.DecodeRuneInString(rest)
			if size <= 0 {
				size = 1
			}
			segment = rest[:size]
			rest = rest[size:]
		}
	}
	return segment, rest, true
}

func splitLongTokenCtx(ctx context.Context, token string, width int) ([]string, bool) {
	if ctxDone(ctx) {
		return nil, false
	}
	if width <= 0 {
		return []string{token}, true
	}

	remaining := token
	parts := make([]string, 0, (len(token)/width)+1)
	for len(remaining) > 0 {
		if ctxDone(ctx) {
			return nil, false
		}
		segment, rest, ok := splitSegmentCtx(ctx, remaining, width)
		if !ok {
			return nil, false
		}
		if segment == "" && rest == "" {
			break
		}

		parts = append(parts, segment)
		if rest == "" || rest == remaining {
			break
		}
		remaining = rest
	}
	if len(parts) == 0 {
		return []string{""}, true
	}
	return parts, true
}

type textToken struct {
	text    string
	width   int
	isSpace bool
}

func tokenizeLineCtx(ctx context.Context, line string) ([]textToken, bool) {
	if ctxDone(ctx) {
		return nil, false
	}
	if line == "" {
		return nil, true
	}

	var tokens []textToken
	var builder strings.Builder
	width := 0
	currentIsSpace := false
	haveToken := false

	flush := func() {
		if builder.Len() == 0 {
			return
		}
		tokens = append(tokens, textToken{
			text:    builder.String(),
			width:   width,
			isSpace: currentIsSpace,
		})
		builder.Reset()
		width = 0
		haveToken = false
	}

	index := 0
	for index < len(line) {
		if ctxDone(ctx) {
			return nil, false
		}
		if loc := ansiSequenceRegex.FindStringIndex(line[index:]); loc != nil && loc[0] == 0 {
			seq := line[index : index+loc[1]]
			builder.WriteString(seq)
			index += loc[1]
			continue
		}

		r, size := utf8.DecodeRuneInString(line[index:])
		if size <= 0 {
			size = 1
		}

		isSpace := unicode.IsSpace(r)
		if !haveToken {
			currentIsSpace = isSpace
			haveToken = true
		} else if currentIsSpace != isSpace {
			flush()
			currentIsSpace = isSpace
			haveToken = true
		}

		builder.WriteString(line[index : index+size])
		width += runewidth.RuneWidth(r)
		index += size
	}

	flush()
	return tokens, true
}

func centerContent(content string, width, height int) string {
	if content == "" {
		return ""
	}

	lines := strings.Split(content, "\n")
	trimmed := make([]string, len(lines))
	maxWidth := 0
	for i, line := range lines {
		trimmedLine := strings.TrimRight(line, " ")
		trimmed[i] = trimmedLine
		if w := visibleWidth(trimmedLine); w > maxWidth {
			maxWidth = w
		}
	}

	if width <= 0 {
		width = maxWidth
	}

	padded := make([]string, len(trimmed))
	for i, line := range trimmed {
		lineWidth := visibleWidth(line)
		if width <= lineWidth {
			padded[i] = line
			continue
		}

		padding := (width - lineWidth) / 2
		if padding < 0 {
			padding = 0
		}
		padded[i] = strings.Repeat(" ", padding) + line
	}

	if height > len(padded) {
		topPadding := (height - len(padded)) / 2
		if topPadding > 0 {
			blank := make([]string, topPadding)
			padded = append(blank, padded...)
		}
	}

	return strings.Join(padded, "\n")
}

func visibleWidth(s string) int {
	if s == "" {
		return 0
	}
	clean := ansiSequenceRegex.ReplaceAllString(s, "")
	return runewidth.StringWidth(clean)
}

func formatHistorySnippet(snippet string, width int) string {
	trimmed := strings.TrimSpace(snippet)
	if trimmed == "" {
		return ""
	}

	content := trimmed
	if isLikelyHTML(content) {
		stripped := stripHTMLTags(content)
		if strings.TrimSpace(stripped) == "" {
			content = historySnippetPlaceholder
		} else {
			content = stripped
		}
	}

	if width <= 0 {
		width = 80
	}

	wrapped := wrapToWidth(content, width)
	lines := strings.Split(wrapped, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmedLine := strings.TrimRight(line, " ")
		trimmedLine = strings.TrimSpace(trimmedLine)
		if trimmedLine != "" {
			cleaned = append(cleaned, trimmedLine)
		}
	}
	if len(cleaned) == 0 {
		return content
	}
	if len(cleaned) > historySnippetMaxLines {
		cleaned = append(cleaned[:historySnippetMaxLines], "… (truncated)")
	}
	return strings.Join(cleaned, "\n")
}

func isLikelyHTML(s string) bool {
	return strings.Contains(s, "<") && strings.Contains(s, ">")
}

var blockLevelHTMLTags = map[string]struct{}{
	"article": {},
	"aside":   {},
	"body":    {},
	"div":     {},
	"footer":  {},
	"header":  {},
	"li":      {},
	"main":    {},
	"nav":     {},
	"p":       {},
	"section": {},
	"table":   {},
	"tr":      {},
	"td":      {},
	"th":      {},
	"ul":      {},
	"ol":      {},
	"h1":      {},
	"h2":      {},
	"h3":      {},
	"h4":      {},
	"h5":      {},
	"h6":      {},
}

var htmlEntityReplacer = strings.NewReplacer(
	"&nbsp;", " ",
	"&amp;", "&",
	"&lt;", "<",
	"&gt;", ">",
	"&quot;", "\"",
	"&#39;", "'",
)

func stripHTMLTags(input string) string {
	if input == "" {
		return ""
	}

	var out strings.Builder
	var tag strings.Builder
	inTag := false
	skipDepth := 0

	for i := 0; i < len(input); i++ {
		ch := input[i]
		if ch == '<' {
			inTag = true
			tag.Reset()
			continue
		}
		if inTag {
			if ch == '>' {
				raw := strings.TrimSpace(tag.String())
				closing := false
				if strings.HasPrefix(raw, "/") {
					closing = true
					raw = strings.TrimSpace(raw[1:])
				}
				if idx := strings.IndexAny(raw, " \t\r\n/"); idx != -1 {
					raw = raw[:idx]
				}
				raw = strings.ToLower(raw)
				if raw != "" {
					switch raw {
					case "style", "script":
						if closing {
							if skipDepth > 0 {
								skipDepth--
							}
						} else {
							skipDepth++
						}
					case "br":
						if !closing && skipDepth == 0 {
							out.WriteString("\n")
						}
					default:
						if closing && skipDepth == 0 {
							if _, ok := blockLevelHTMLTags[raw]; ok {
								out.WriteString("\n")
							}
						}
					}
				}
				inTag = false
				continue
			}
			tag.WriteByte(ch)
			continue
		}
		if skipDepth > 0 {
			continue
		}
		out.WriteByte(ch)
	}

	text := htmlEntityReplacer.Replace(out.String())
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	lines := strings.Split(text, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return strings.Join(cleaned, "\n")
}

func currentCursorLine(ed requestEditor) int {
	return ed.Line() + 1
}

func requestAtLine(doc *restfile.Document, line int) (*restfile.Request, int) {
	if doc == nil || line < 1 {
		return nil, -1
	}

	for idx, req := range doc.Requests {
		if line >= req.LineRange.Start && line <= req.LineRange.End {
			return req, idx
		}
	}

	// parser anchors LineRange.Start at the first non header line of a request.
	// treat the preceding line (should always be "###" separator) as part of the request
	// so the cursor on the header selects the correct request in the UI.
	headerLine := line + 1
	for idx, req := range doc.Requests {
		if headerLine == req.LineRange.Start {
			return req, idx
		}
	}

	return nil, -1
}

func requestIdentifier(req *restfile.Request) string {
	if req == nil {
		return ""
	}
	if req.Metadata.Name != "" {
		return req.Metadata.Name
	}
	return strings.TrimSpace(req.URL)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

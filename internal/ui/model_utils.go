package ui

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/ui/textarea"
	"github.com/unkn0wn-root/resterm/internal/wrap"
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
	res, ok := wrap.Wrap(ctx, content, width, wrap.Plain, false)
	if !ok {
		return "", false
	}
	return res.S, true
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
	case responseTabDiff:
		return wrapDiffContentCtx(ctx, content, width)
	case responseTabRaw:
		res, ok := wrap.Wrap(ctx, content, width, wrap.Pre, false)
		if !ok {
			return "", false
		}
		return res.S, true
	case responseTabPretty:
		res, ok := wrap.Wrap(ctx, content, width, wrap.Structured, false)
		if !ok {
			return "", false
		}
		return res.S, true
	default:
		res, ok := wrap.Wrap(ctx, content, width, wrap.Plain, false)
		if !ok {
			return "", false
		}
		return res.S, true
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
	mode := wrap.Plain
	switch tab {
	case responseTabRaw:
		mode = wrap.Pre
	case responseTabPretty:
		mode = wrap.Structured
	}
	res, ok := wrap.Wrap(ctx, content, width, mode, true)
	if !ok {
		return "", nil, nil, false
	}
	spans := make([]lineSpan, len(res.Sp))
	for i, sp := range res.Sp {
		spans[i] = lineSpan{start: sp.S, end: sp.E}
	}
	return res.S, spans, res.Rv, true
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
	res, ok := wrap.Wrap(ctx, content, width, wrap.Pre, false)
	if !ok {
		return "", false
	}
	return res.S, true
}

func wrapPreformattedLineCtx(ctx context.Context, line string, width int) ([]string, bool) {
	return wrap.Line(ctx, line, width, wrap.Pre)
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
	return wrap.Line(ctx, line, width, wrap.Plain)
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

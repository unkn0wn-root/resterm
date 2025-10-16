package ui

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
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

var ansiSequenceRegex = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]|\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)")

func stripANSIEscape(s string) string {
	return ansiSequenceRegex.ReplaceAllString(s, "")
}

func formatTestSummary(results []scripts.TestResult, scriptErr error) string {
	if len(results) == 0 && scriptErr == nil {
		return ""
	}

	builder := strings.Builder{}
	builder.WriteString("Tests:\n")
	if scriptErr != nil {
		builder.WriteString(fmt.Sprintf("  [error] %v\n", scriptErr))
	}
	for _, result := range results {
		status := "PASS"
		if !result.Passed {
			status = "FAIL"
		}
		line := fmt.Sprintf("  [%s] %s", status, result.Name)
		if result.Message != "" {
			line += fmt.Sprintf(" – %s", result.Message)
		}
		if result.Elapsed > 0 {
			line += fmt.Sprintf(" (%s)", result.Elapsed.Truncate(time.Millisecond))
		}
		builder.WriteString(line + "\n")
	}
	return strings.TrimRight(builder.String(), "\n")
}

func buildResponseSummary(resp *httpclient.Response, tests []scripts.TestResult, scriptErr error) string {
	if resp == nil {
		return ""
	}

	parts := []string{
		fmt.Sprintf("Status: %s", resp.Status),
		fmt.Sprintf("URL: %s", resp.EffectiveURL),
	}

	if resp.Duration > 0 {
		parts = append(parts, fmt.Sprintf("Duration: %s", resp.Duration.Round(time.Millisecond)))
	}

	summary := strings.Join(parts, "\n")
	if testSummary := formatTestSummary(tests, scriptErr); testSummary != "" {
		summary = joinSections(summary, testSummary)
	}
	return summary
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

func wrapToWidth(content string, width int) string {
	if width <= 0 {
		return content
	}

	lines := strings.Split(content, "\n")
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		segments := wrapLineSegments(line, width)
		wrapped = append(wrapped, segments...)
	}
	return strings.Join(wrapped, "\n")
}

func wrapContentForTab(tab responseTab, content string, width int) string {
	switch tab {
	case responseTabRaw:
		return wrapPreformattedContent(content, width)
	case responseTabDiff:
		return wrapDiffContent(content, width)
	default:
		return wrapToWidth(content, width)
	}
}

func wrapPreformattedContent(content string, width int) string {
	if width <= 0 {
		return content
	}

	lines := strings.Split(content, "\n")
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		segments := wrapPreformattedLine(line, width)
		wrapped = append(wrapped, segments...)
	}
	return strings.Join(wrapped, "\n")
}

func wrapPreformattedLine(line string, width int) []string {
	if width <= 0 {
		return []string{line}
	}
	if line == "" {
		return []string{""}
	}
	if visibleWidth(line) <= width {
		return []string{line}
	}

	indent := leadingIndent(line)
	if indent == "" {
		return wrapLineSegments(line, width)
	}

	indentWidth := visibleWidth(indent)
	available := width - indentWidth
	if available <= 0 {
		return wrapLineSegments(line, width)
	}

	body := line[len(indent):]
	if body == "" {
		return []string{indent}
	}

	segments := make([]string, 0, (len(line)/width)+1)
	remaining := body
	for len(remaining) > 0 {
		segment, rest := splitSegment(remaining, available)
		segments = append(segments, indent+segment)
		if rest == "" || rest == remaining {
			if rest == "" {
				break
			}
			fallback := wrapLineSegments(rest, width)
			segments = append(segments, fallback...)
			break
		}
		remaining = rest
	}
	if len(segments) == 0 {
		return []string{""}
	}
	return segments
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
	if width <= 0 {
		return []string{line}
	}
	if line == "" {
		return []string{""}
	}
	if visibleWidth(line) <= width {
		return []string{line}
	}

	tokens := tokenizeLine(line)
	if len(tokens) == 0 {
		return []string{""}
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
					segment, rest := splitSegment(text, remaining)
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
				parts := splitLongToken(text, width)
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
		return []string{""}
	}
	return segments
}

func splitSegment(s string, width int) (string, string) {
	if width <= 0 || visibleWidth(s) <= width {
		return s, ""
	}

	var builder strings.Builder
	currentWidth := 0
	index := 0
	for index < len(s) {
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
	return segment, rest
}

func splitLongToken(token string, width int) []string {
	if width <= 0 {
		return []string{token}
	}

	remaining := token
	parts := make([]string, 0, (len(token)/width)+1)
	for len(remaining) > 0 {
		segment, rest := splitSegment(remaining, width)
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
		return []string{""}
	}
	return parts
}

type textToken struct {
	text    string
	width   int
	isSpace bool
}

func tokenizeLine(line string) []textToken {
	if line == "" {
		return nil
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
	return tokens
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

func findRequestAtLine(doc *restfile.Document, line int) *restfile.Request {
	if doc == nil {
		return nil
	}

	for _, req := range doc.Requests {
		if line >= req.LineRange.Start && line <= req.LineRange.End {
			return req
		}
	}
	if len(doc.Requests) > 0 {
		return doc.Requests[len(doc.Requests)-1]
	}
	return nil
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

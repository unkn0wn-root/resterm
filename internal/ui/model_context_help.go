package ui

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/helpdoc"
)

func (m *Model) showContextHelp() tea.Cmd {
	topic, ok := m.contextHelpTopic()
	if !ok {
		return statusCmd(
			statusWarn,
			"No help topic for this context; press ? or run :help",
		)
	}
	m.openHelpTopic(topic)
	return nil
}

func (m *Model) contextHelpTopic() (helpdoc.Topic, bool) {
	pos := m.editor.caretPosition()
	line := m.editor.LineRunes(pos.Line)
	if name, ok := contextDirective(string(line)); ok {
		return helpdoc.Directive(name)
	}
	if cursorInTemplate(line, pos.Column) {
		return helpdoc.Lookup("variables")
	}
	return helpdoc.Lookup(contextWord(line, pos.Column))
}

func contextDirective(line string) (directive.Name, bool) {
	text := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(text, "//"), strings.HasPrefix(text, "--"):
		text = strings.TrimSpace(text[2:])
	case strings.HasPrefix(text, "#"):
		text = strings.TrimSpace(text[1:])
	case strings.HasPrefix(text, "/*"):
		text = strings.TrimSpace(strings.TrimPrefix(text, "/*"))
		if idx := strings.Index(text, "*/"); idx >= 0 {
			text = strings.TrimSpace(text[:idx])
		}
	case strings.HasPrefix(text, "*"):
		text = strings.TrimSpace(strings.TrimPrefix(text, "*"))
	default:
		return "", false
	}

	call, ok := directive.Parse(text)
	if !ok {
		return "", false
	}
	spec, ok := directive.Lookup(call.Spelling)
	if !ok {
		return "", false
	}
	return spec.Name, true
}

func cursorInTemplate(line []rune, col int) bool {
	col = clamp(col, 0, len(line))
	left := strings.LastIndex(string(line[:col]), "{{")
	if left < 0 {
		return false
	}
	return strings.Contains(string(line[col:]), "}}")
}

func contextWord(line []rune, col int) string {
	if len(line) == 0 {
		return ""
	}
	col = clamp(col, 0, len(line))
	if col == len(line) || !isHelpWordRune(line[col]) {
		col--
	}
	if col < 0 || !isHelpWordRune(line[col]) {
		return ""
	}
	start := col
	for start > 0 && isHelpWordRune(line[start-1]) {
		start--
	}
	end := col + 1
	for end < len(line) && isHelpWordRune(line[end]) {
		end++
	}
	return string(line[start:end])
}

func isHelpWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_'
}

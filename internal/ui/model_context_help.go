package ui

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/helpdoc"
	"github.com/unkn0wn-root/resterm/internal/parser"
)

func (m *Model) showContextHelp() tea.Cmd {
	if m.diagnosticsActive() {
		return m.requestDiagnostics(diagnosticHelp)
	}
	return m.showContextDocumentation()
}

func (m *Model) showContextDocumentation() tea.Cmd {
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
	syntax := m.editor.sourceLine(pos.Line)
	if syntax.Directive.Known() {
		return helpdoc.Directive(syntax.Directive)
	}
	switch syntax.Kind {
	case parser.SourceLineDirective, parser.SourceLineDirectiveValue:
		// An unknown directive must not resolve to an unrelated keyword topic.
		return helpdoc.Topic{}, false
	}
	// Literal bodies and scripts can still contain templates or help keywords.
	line := m.editor.LineRunes(pos.Line)
	if cursorInTemplate(line, pos.Column) {
		return helpdoc.Lookup("variables")
	}
	return helpdoc.Lookup(contextWord(line, pos.Column))
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
	// Directive names require source context, not ordinary word lookup.
	if start > 0 && line[start-1] == '@' {
		return ""
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

package ui

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/prompt"
)

type pathArgKind uint8

const (
	pathArgNone pathArgKind = iota
	pathArgWorkspace
	pathArgRequests
)

type pathArg struct {
	kind  pathArgKind
	value string
	edit  prompt.Edit
}

func (c exCatalog) pathArg(input string, cursor int) (pathArg, bool) {
	at, ok := newArgCursor(input, cursor)
	if !ok {
		return pathArg{}, false
	}

	def, ok := c.Lookup(commandName(at.line.Tokens[0].Value))
	if !ok {
		return pathArg{}, false
	}
	switch def.kind {
	case exCommandEdit:
		return at.editPath()
	case exCommandMock:
		return at.mockSourcePath()
	default:
		return pathArg{}, false
	}
}

type argCursor struct {
	line   prompt.Line
	cursor int
	token  prompt.Token
	index  int
	inside bool
}

func newArgCursor(input string, cursor int) (argCursor, bool) {
	line := prompt.Lex(input)
	if len(line.Tokens) == 0 {
		return argCursor{}, false
	}

	token, index, inside := line.TokenAt(cursor)
	if inside {
		return argCursor{line: line, cursor: cursor, token: token, index: index, inside: true}, true
	}

	last := len(line.Tokens) - 1
	if !line.SpaceBetween(line.Tokens[last].End, cursor) {
		return argCursor{}, false
	}
	return argCursor{line: line, cursor: cursor, index: last + 1}, true
}

func (a argCursor) edit() prompt.Edit {
	if !a.inside {
		return prompt.Edit{Start: a.cursor, End: a.cursor}
	}
	return prompt.Edit{Start: a.token.Start, End: a.token.End}
}

func (a argCursor) value() string {
	if !a.inside {
		return ""
	}
	return a.line.ValueAt(a.token, a.cursor)
}

func (a argCursor) previous() string {
	if a.index <= 0 {
		return ""
	}
	return a.line.Tokens[a.index-1].Value
}

func (a argCursor) editPath() (pathArg, bool) {
	if a.index != 1 {
		return pathArg{}, false
	}
	return pathArg{kind: pathArgWorkspace, value: a.value(), edit: a.edit()}, true
}

func (a argCursor) mockSourcePath() (pathArg, bool) {
	if len(a.line.Tokens) < 2 {
		return pathArg{}, false
	}
	switch strings.ToLower(a.line.Tokens[1].Value) {
	case "start", "restart":
	default:
		return pathArg{}, false
	}

	if a.inside {
		if arg, ok := a.sourceEqualsPath(); ok {
			return arg, true
		}
	}
	if !isSourceFlag(a.previous()) {
		return pathArg{}, false
	}
	return pathArg{kind: pathArgRequests, value: a.value(), edit: a.edit()}, true
}

func (a argCursor) sourceEqualsPath() (pathArg, bool) {
	raw := a.line.Text(a.token.Start, a.token.End)
	for _, prefix := range []string{"--" + mockSourceFlagName + "=", "-" + mockSourceFlagAlias + "="} {
		if !strings.HasPrefix(raw, prefix) {
			continue
		}
		start := a.token.Start + len([]rune(prefix))
		if a.cursor < start {
			return pathArg{}, false
		}
		value, ok := strings.CutPrefix(a.value(), prefix)
		if !ok {
			return pathArg{}, false
		}
		return pathArg{
			kind:  pathArgRequests,
			value: value,
			edit:  prompt.Edit{Start: start, End: a.token.End},
		}, true
	}
	return pathArg{}, false
}

func isSourceFlag(value string) bool {
	return value == "--"+mockSourceFlagName || value == "-"+mockSourceFlagAlias
}

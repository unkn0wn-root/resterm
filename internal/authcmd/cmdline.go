package authcmd

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
)

type argBuilder struct {
	args []string
	cur  strings.Builder
	open bool
}

func (b *argBuilder) write(s string) {
	b.cur.WriteString(s)
	b.open = true
}

func (b *argBuilder) end() {
	if !b.open {
		return
	}
	b.args = append(b.args, b.cur.String())
	b.cur.Reset()
	b.open = false
}

// splitCommand splits quoted words and templates into arguments without
// running a shell. Templates expand after splitting, so spaces in a value
// stay in one argument.
func splitCommand(line string) ([]string, error) {
	var b argBuilder
	var quote byte
	for i := 0; i < len(line); i++ {
		if n := templateLen(line[i:]); n > 0 {
			b.write(line[i : i+n])
			i += n - 1
			continue
		}
		c := line[i]
		switch {
		case quote != 0 && c == quote:
			quote = 0
		case quote == '\'':
			b.write(line[i : i+1])
		case c == '\\' && i+1 < len(line) && escapes(quote, line[i+1]):
			i++
			b.write(line[i : i+1])
		case quote == '"':
			b.write(line[i : i+1])
		case c == '\'' || c == '"':
			quote = c
			b.write("")
		case isSpace(c):
			b.end()
		default:
			b.write(line[i : i+1])
		}
	}
	if quote != 0 {
		return nil, diag.Newf(diag.ClassAuth, "cmd has an unterminated %c quote", quote)
	}
	b.end()
	return b.args, nil
}

func escapes(quote, next byte) bool {
	if quote == '"' {
		return next == '"' || next == '\\'
	}
	return next == '"' || next == '\'' || next == '\\' || isSpace(next)
}

func templateLen(s string) int {
	if !strings.HasPrefix(s, "{{") {
		return 0
	}
	end := strings.Index(s[2:], "}}")
	if end < 0 {
		return 0
	}
	return end + 4
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

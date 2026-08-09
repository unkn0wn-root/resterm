package parser

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/directive"
)

func (b *documentBuilder) handleComment(no, baseCol int, text string) {
	call, ok := directive.Parse(text)
	if !ok {
		return
	}
	d := directiveLine{Call: call, no: no}
	if baseCol > 0 {
		d.argCol = baseCol + call.ArgOffset
	}
	if b.redeclared(d) {
		return
	}
	if b.routeDirective(d) == directiveApplied {
		b.markDeclared(d)
	}
}

func (b *documentBuilder) handleBlockComment(ln line) bool {
	if b.inBlock {
		content, closed := parseBlockCommentLine(ln.text)
		if content != "" {
			b.handleComment(ln.no, 0, content)
		}
		b.appendLine(ln.raw)
		if closed {
			b.inBlock = false
		}
		return true
	}

	if ln.isBlockCommentStart() {
		content, closed := cutBlockCommentStart(ln.text)
		if content != "" {
			b.handleComment(ln.no, 0, content)
		}
		b.appendLine(ln.raw)
		if !closed {
			b.inBlock = true
		}
		return true
	}
	return false
}

func (b *documentBuilder) handleCommentLine(ln line) bool {
	if commentText, col, ok := stripComment(ln.text); ok {
		// col counts from the start of the trimmed text. Adding the offset of
		// the trimmed text inside the raw line turns it into a source column.
		base := strings.Index(ln.raw, ln.text) + col
		b.handleComment(ln.no, base, commentText)
		b.appendLine(ln.raw)
		return true
	}
	return false
}

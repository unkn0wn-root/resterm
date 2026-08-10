package parser

func (b *documentBuilder) handleComment(no, baseCol int, text string) {
	d, ok := b.readDirective(no, baseCol, text)
	if !ok {
		return
	}
	if !b.redeclared(d) && b.routeDirective(d) == directiveApplied {
		b.markDeclared(d)
	}
	b.flushOpenLines()
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
	text, col, ok := ln.comment()
	if !ok {
		return false
	}
	b.handleComment(ln.no, col, text)
	b.appendLine(ln.raw)
	return true
}

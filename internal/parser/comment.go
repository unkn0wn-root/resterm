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
	opening := !b.inBlock
	if opening && !ln.isBlockCommentStart() {
		return false
	}

	c, closed := ln.blockComment(opening)
	if c.text != "" {
		b.handleComment(ln.no, 0, c.text)
	}
	b.appendLine(ln.raw)
	b.inBlock = !closed
	return true
}

func (b *documentBuilder) handleCommentLine(ln line) bool {
	c, ok := ln.comment()
	if !ok {
		return false
	}
	b.handleComment(ln.no, c.col(), c.text)
	b.appendLine(ln.raw)
	return true
}

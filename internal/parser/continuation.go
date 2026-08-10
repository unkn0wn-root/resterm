package parser

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/capture"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

type openDirective struct {
	d      parsedDirective
	closer string
}

func (b *documentBuilder) readDirective(no, col int, text string) (parsedDirective, bool) {
	call, ok := directive.Parse(text)
	if b.open != nil {
		// A known directive ends the current one instead of becoming its argument.
		if !ok || !call.Name.Known() {
			return b.growDirective(no, col, text)
		}
		b.failOpenDirective()
	}
	if !ok {
		return parsedDirective{}, false
	}

	d := parsedDirective{Call: call, lines: restfile.LineRange{Start: no, End: no}}
	if col > 0 {
		d.argCol = col + call.ArgOffset
	}
	if closer := openCloser(d.Name, d.Args); closer != "" {
		b.open = &openDirective{d: d, closer: closer}
		return parsedDirective{}, false
	}
	return d, true
}

func (b *documentBuilder) growDirective(no, col int, text string) (parsedDirective, bool) {
	o := b.open
	o.d.lines.End = no
	// Preserve newlines and indentation so diagnostics keep their source positions.
	padding := max(col-1, 0)
	if padding == 0 {
		switch o.d.Name.Continuation() {
		case directive.ContinueExpr, directive.ContinueCapture:
			// The writer replaces one leading padding byte with its "#" marker.
			// These grammars ignore that whitespace, so keep their model stable.
			padding = 1
		}
	}
	o.d.Args += "\n" + strings.Repeat(" ", padding) + text

	if closer := openCloser(o.d.Name, o.d.Args); closer != "" {
		o.closer = closer
		return parsedDirective{}, false
	}
	b.open = nil
	return o.d, true
}

func openCloser(name directive.Name, args string) string {
	switch name.Continuation() {
	case directive.ContinueOptions:
		return closerText(directive.OptionsOpen(args))
	case directive.ContinueExpr:
		return closerText(rts.OpenGroup(argExpr(name, args)))
	case directive.ContinueCapture:
		return captureCloser(argExpr(name, args))
	}
	return ""
}

func closerText(closer rune) string {
	if closer == 0 {
		return ""
	}
	return string(closer)
}

// Template captures follow markers. Captures without markers use RTS groups.
func captureCloser(expr string) string {
	if closer := capture.OpenMarker(expr); closer != "" {
		return closer
	}
	if capture.HasUnquotedTemplateMarker(expr) {
		return ""
	}
	return closerText(rts.OpenGroup(expr))
}

// argExpr strips names, messages, and options before checking delimiters.
// Its split helpers accept incomplete arguments because collection is not done yet.
func argExpr(name directive.Name, args string) string {
	switch name {
	case directive.Assert:
		expr, _ := splitAssert(args)
		return expr
	case directive.Capture:
		_, _, expr := cutCapture(args)
		return expr
	case directive.Patch:
		_, _, expr := cutPatch(args)
		return expr
	case directive.Apply:
		return applyExpr(args)
	case directive.ForEach:
		expr, _, _ := cutForEach(args)
		return expr
	case directive.If, directive.Elif, directive.Case:
		expr, _ := cutBranch(args)
		return expr
	default:
		return args
	}
}

func (b *documentBuilder) closeOpenDirective(ln line) {
	if b.open == nil || b.carriesArgument(ln) {
		return
	}
	b.failOpenDirective()
}

func (b *documentBuilder) carriesArgument(ln line) bool {
	if b.inBlock {
		return true
	}
	return !ln.isSeparator() && ln.isComment()
}

func (b *documentBuilder) flushOpenLines() {
	held := b.openLines
	b.openLines = nil
	if b.inRequest {
		b.request.originalLines = append(b.request.originalLines, held...)
	}
}

func (b *documentBuilder) failOpenDirective() {
	o := b.open
	b.open = nil
	b.openLines = nil
	err := &directive.UnclosedError{Directive: o.d.Spelling, Closer: o.closer}
	if b.mock != nil {
		b.addMockError(o.d.lines.Start, err.Error())
		return
	}
	b.addError(o.d.lines.Start, err.Error())
}

package parser

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/capture"
	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

type openDirective struct {
	d      parsedDirective
	closer string
	args   strings.Builder
	state  continuationState
}

func (o *openDirective) write(text string) {
	o.args.WriteString(text)
	o.state.feed(text)
}

func (o *openDirective) writeOpen(text string) int {
	o.args.WriteString(text)
	return o.state.feedOpen(text)
}

func (o *openDirective) add(no int, c commentText) int {
	padding := max(c.argCol()-1, 0)
	if padding == 0 {
		switch o.d.Name.Continuation() {
		case directive.ContinueExpr, directive.ContinueCapture:
			// Expression columns include one byte for the missing comment marker.
			padding = 1
		}
	}
	o.write("\n")
	if padding > 0 {
		o.write(strings.Repeat(" ", padding))
	}
	o.d.argParts = append(o.d.argParts, argumentPart{
		start: o.args.Len(), end: o.args.Len() + len(c.text),
		pos: diag.Pos{Line: no, Col: c.col()},
	})
	return o.writeOpen(c.text)
}

func (o *openDirective) collect() string {
	o.d.Args = o.args.String()
	return o.d.Args
}

// Parsing and editor highlighting share this reader for multiline directives.
type directiveReader struct {
	open *openDirective
}

type directiveReadKind uint8

const (
	directiveReadNone directiveReadKind = iota
	directiveReadMark                   // Bare @ typed before a directive name.
	directiveReadStarted
	directiveReadContinued
	directiveReadCompleted
	directiveReadContinuationCompleted
)

type directiveReadResult struct {
	kind      directiveReadKind
	directive parsedDirective
	owner     directive.Name
	cut       *openDirective
	// Bytes used by the option value on this line.
	optionValueLen int
}

func (r directiveReadResult) completed() (parsedDirective, bool) {
	switch r.kind {
	case directiveReadCompleted, directiveReadContinuationCompleted:
		return r.directive, true
	default:
		return parsedDirective{}, false
	}
}

func (r *directiveReader) pending() (directive.Name, bool) {
	if r.open == nil {
		return "", false
	}
	return r.open.d.Name, true
}

func (r *directiveReader) abandon() *openDirective {
	o := r.open
	if o == nil {
		return nil
	}
	r.open = nil
	if closer := openCloser(o.d.Name, o.collect()); closer != "" {
		o.closer = closer
	}
	return o
}

func (r *directiveReader) read(no int, c commentText) directiveReadResult {
	call, parsed := directive.Parse(c.text)
	if r.open != nil {
		// A known directive ends the unfinished directive.
		if !parsed || !call.Name.Known() {
			return r.grow(no, c)
		}
		cut := r.abandon()
		return r.readNew(no, c, call, cut)
	}
	if !parsed {
		if c.text == "@" {
			return directiveReadResult{kind: directiveReadMark}
		}
		return directiveReadResult{}
	}
	return r.readNew(no, c, call, nil)
}

func (r *directiveReader) readNew(no int, c commentText, call directive.Call, cut *openDirective) directiveReadResult {
	d := newParsedDirective(call, restfile.LineRange{Start: no, End: no})
	name := strings.TrimRightFunc(c.text[:call.ArgOffset], directive.IsArgSep)
	d.nameSpan = diag.Span{
		Start: diag.Pos{Line: no, Col: c.col()},
		End:   diag.Pos{Line: no, Col: c.col() + len(name)},
	}
	d.argParts = []argumentPart{{end: len(call.Args), pos: diag.Pos{Line: no, Col: c.col() + call.ArgOffset}}}
	if col := c.argCol(); col > 0 {
		d.argCol = col + call.ArgOffset
	}
	if closer := openCloser(d.Name, d.Args); closer != "" {
		o := &openDirective{d: d, closer: closer}
		o.args.WriteString(d.Args)
		o.state = newContinuationState(d.Name, d.Args)
		r.open = o
		return directiveReadResult{
			kind:  directiveReadStarted,
			owner: d.Name,
			cut:   cut,
		}
	}
	return directiveReadResult{
		kind:      directiveReadCompleted,
		directive: d,
		owner:     d.Name,
		cut:       cut,
	}
}

func (r *directiveReader) grow(no int, c commentText) directiveReadResult {
	o := r.open
	o.d.lines.End = no
	valueLen := o.add(no, c)
	res := directiveReadResult{
		kind:           directiveReadContinued,
		owner:          o.d.Name,
		optionValueLen: valueLen,
	}

	if !o.state.mayComplete() {
		return res
	}
	if closer := openCloser(o.d.Name, o.collect()); closer != "" {
		o.closer = closer
		// The full parse found more input to collect, so rebuild the scan state.
		o.state = newContinuationState(o.d.Name, o.d.Args)
		return res
	}

	r.open = nil
	res.kind = directiveReadContinuationCompleted
	res.directive = o.d
	return res
}

// Only comments can continue an argument. A separator always ends it.
func (r *directiveReader) close(ln line, inBlock bool) *openDirective {
	if r.open == nil || inBlock || (!ln.isSeparator() && ln.isComment()) {
		return nil
	}
	return r.abandon()
}

func (b *documentBuilder) readDirective(no int, c commentText) (parsedDirective, bool) {
	result := b.reader.read(no, c)
	if result.cut != nil {
		b.failOpenDirective(result.cut)
	}
	return result.completed()
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
		expr, _, _ := splitAssert(args)
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
	case directive.Poll:
		_, expr, _ := cutPollUntil(args)
		return expr
	case directive.If, directive.Elif, directive.Case:
		expr, _ := cutBranch(args)
		return expr
	default:
		return args
	}
}

func (b *documentBuilder) closeOpenDirective(ln line) {
	if cut := b.reader.close(ln, b.inBlock); cut != nil {
		b.failOpenDirective(cut)
	}
}

func (b *documentBuilder) flushOpenLines() {
	held := b.openLines
	b.openLines = nil
	if b.inRequest {
		b.request.originalLines = append(b.request.originalLines, held...)
	}
}

func (b *documentBuilder) failOpenDirective(o *openDirective) {
	b.openLines = nil
	err := &directive.UnclosedError{Directive: o.d.Spelling, Closer: o.closer}
	item := o.d.diagnostic(err.Error(), err)
	item.Mock = b.mock != nil
	b.pushError(item)
}

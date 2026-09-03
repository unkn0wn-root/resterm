package parser

import (
	"unicode/utf8"

	"github.com/unkn0wn-root/resterm/internal/capture"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

type expressionCutMode uint8

const (
	expressionCutNone expressionCutMode = iota
	expressionCutAssert
	expressionCutBranch
	expressionCutForEach
)

type expressionCutAction uint8

const (
	expressionCutContinue expressionCutAction = iota
	expressionCutRestart
	expressionCutStop
)

type optionStartScanner struct {
	key       int
	invalid   bool
	skipLogic byte
}

func (s *optionStartScanner) feed(ch, next byte) bool {
	if s.skipLogic != 0 {
		if ch == s.skipLogic {
			s.skipLogic = 0
			return false
		}
		s.skipLogic = 0
	}

	if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
		s.key = 0
		s.invalid = false
		return false
	}
	if ch == '&' || ch == '|' {
		if next == ch {
			s.invalid = true
			s.skipLogic = ch
			return false
		}
		// Mask treats a lone ampersand or pipe as a field boundary.
		s.key = 0
		s.invalid = false
		return false
	}
	if s.invalid {
		return false
	}
	if directive.IsKeyRune(rune(ch)) {
		s.key++
		return false
	}
	if ch == '=' && s.key > 0 {
		if next == '=' {
			s.invalid = true
			return false
		}
		return true
	}
	s.invalid = true
	return false
}

type expressionCutScanner struct {
	mode    expressionCutMode
	stopped bool
	inSeen  bool
	last    byte
	window  [4]byte
	windowN int
	option  optionStartScanner
}

func (s *expressionCutScanner) feed(ch, next byte) expressionCutAction {
	if s.stopped {
		return expressionCutStop
	}

	switch s.mode {
	case expressionCutAssert:
		cut := s.last == '=' && ch == '>'
		s.last = ch
		if cut {
			s.stopped = true
			return expressionCutStop
		}
	case expressionCutBranch:
		if s.option.feed(ch, next) {
			s.stopped = true
			return expressionCutStop
		}
	case expressionCutForEach:
		s.push(ch)
		switch {
		case s.hasSuffix(" as "):
			s.stopped = true
			return expressionCutStop
		case !s.inSeen && s.hasSuffix(" in "):
			s.inSeen = true
			s.windowN = 0
			return expressionCutRestart
		}
	}
	return expressionCutContinue
}

func (s *expressionCutScanner) push(ch byte) {
	if s.windowN < len(s.window) {
		s.window[s.windowN] = ch
		s.windowN++
		return
	}
	copy(s.window[:], s.window[1:])
	s.window[len(s.window)-1] = ch
}

func (s *expressionCutScanner) hasSuffix(text string) bool {
	if s.windowN < len(text) {
		return false
	}
	start := s.windowN - len(text)
	return string(s.window[start:s.windowN]) == text
}

type expressionContinuation struct {
	groups rts.GroupScanner
	cut    expressionCutScanner
}

func newExpressionContinuation(name directive.Name, args string) expressionContinuation {
	var scan expressionContinuation
	switch name {
	case directive.Assert:
		scan.cut.mode = expressionCutAssert
		expr, _, cut := splitAssert(args)
		scan.start(expr, cut)
	case directive.If, directive.Elif, directive.Case:
		scan.cut.mode = expressionCutBranch
		expr, opts := cutBranch(args)
		scan.start(expr, opts != "")
	case directive.ForEach:
		scan.cut.mode = expressionCutForEach
		expr, _, form := cutForEach(args)
		scan.cut.inSeen = form == forEachIn
		scan.start(expr, form == forEachAs)
	default:
		scan.groups.Feed(argExpr(name, args))
	}
	return scan
}

func (s *expressionContinuation) start(expr string, cut bool) {
	if cut {
		s.groups.Feed(expr)
		s.cut.stopped = true
		return
	}
	s.feed(expr)
}

func (s *expressionContinuation) feed(src string) {
	for i := range len(src) {
		var next byte
		if i+1 < len(src) {
			next = src[i+1]
		}
		top, ok := s.groups.ScanByte(src[i])
		if !ok {
			continue
		}
		switch s.cut.feed(top, next) {
		case expressionCutRestart:
			s.groups = rts.GroupScanner{}
		case expressionCutStop:
			return
		}
	}
}

type captureContinuation struct {
	groups    rts.GroupScanner
	templates capture.TemplateScanner
}

func (s *captureContinuation) feed(src string) {
	s.groups.Feed(src)
	s.templates.Feed(src)
}

func (s *captureContinuation) mayComplete() bool {
	switch s.templates.State() {
	case capture.TemplateOpen:
		return false
	case capture.TemplateClosed:
		return true
	default:
		return s.groups.Closer() == 0
	}
}

type continuationState struct {
	mode    directive.Continuation
	options directive.OptionsScanner
	expr    expressionContinuation
	capture captureContinuation
}

func newContinuationState(name directive.Name, args string) continuationState {
	state := continuationState{mode: name.Continuation()}
	switch state.mode {
	case directive.ContinueOptions:
		state.options.Feed(args)
	case directive.ContinueExpr:
		state.expr = newExpressionContinuation(name, args)
	case directive.ContinueCapture:
		state.capture.feed(argExpr(name, args))
	}
	return state
}

func (s *continuationState) feed(src string) {
	switch s.mode {
	case directive.ContinueOptions:
		s.options.Feed(src)
	case directive.ContinueExpr:
		s.expr.feed(src)
	case directive.ContinueCapture:
		s.capture.feed(src)
	}
}

func (s *continuationState) feedLine(src string) int {
	if s.mode != directive.ContinueOptions || !s.options.ValueOpen() {
		s.feed(src)
		return 0
	}

	for i := range src {
		_, size := utf8.DecodeRuneInString(src[i:])
		end := i + size
		s.options.Feed(src[i:end])
		if s.options.Closer() == 0 {
			s.options.Feed(src[end:])
			return end
		}
	}
	return len(src)
}

func (s *continuationState) mayComplete() bool {
	switch s.mode {
	case directive.ContinueOptions:
		return s.options.Closer() == 0
	case directive.ContinueExpr:
		return s.expr.groups.Closer() == 0
	case directive.ContinueCapture:
		return s.capture.mayComplete()
	default:
		return true
	}
}

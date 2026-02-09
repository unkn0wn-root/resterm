package captureutil

import (
	"regexp"
	"strings"
)

const (
	templateOpen  = "{{"
	templateClose = "}}"

	templateOpenLen  = len(templateOpen)
	templateCloseLen = len(templateClose)
)

var mixedTemplateCallPattern = regexp.MustCompile(
	`^\s*[A-Za-z_][A-Za-z0-9_.]*\s*\(`,
)

var strictKeys = []string{
	"capture.strict",
	"capture-strict",
	"capture_strict",
}

var jsonDoubleDotPrefixes = []string{
	"response.json..",
	"last.json..",
}

type strictAliasState struct {
	set      bool
	val      bool
	conflict bool
}

type exprScanner struct {
	s   string
	i   int
	q   byte
	esc bool
}

func newExprScanner(s string) *exprScanner {
	return &exprScanner{s: s}
}

func (sc *exprScanner) done() bool {
	return sc == nil || sc.i >= len(sc.s)
}

func (sc *exprScanner) ch() byte {
	return sc.s[sc.i]
}

func (sc *exprScanner) advance(n int) {
	sc.i += n
}

func (sc *exprScanner) inQuoted(ch byte) bool {
	if sc == nil || sc.q == 0 {
		return false
	}
	if sc.esc {
		sc.esc = false
		return true
	}
	if ch == '\\' {
		sc.esc = true
		return true
	}
	if ch == sc.q {
		sc.q = 0
	}
	return true
}

func (sc *exprScanner) openQuote(ch byte) bool {
	if sc == nil || !isQuote(ch) {
		return false
	}
	sc.q = ch
	return true
}

func (sc *exprScanner) templateEnd() (int, bool) {
	if sc == nil || !hasTemplateOpenAt(sc.s, sc.i) {
		return 0, false
	}
	return nextTemplateEnd(sc.s, sc.i)
}

func HasUnquotedTemplateMarker(ex string) bool {
	_, has := stripUnquotedTemplateSegments(ex)
	return has
}

func MixedTemplateRTSCall(ex string) bool {
	rem, has := stripUnquotedTemplateSegments(ex)
	if !has {
		return false
	}
	return mixedTemplateCallPattern.MatchString(strings.TrimSpace(rem))
}

func stripUnquotedTemplateSegments(ex string) (string, bool) {
	s := strings.TrimSpace(ex)
	if s == "" {
		return "", false
	}
	sc := newExprScanner(s)
	var b strings.Builder
	b.Grow(len(s))
	has := false

	for !sc.done() {
		ch := sc.ch()
		if sc.inQuoted(ch) {
			b.WriteByte(ch)
			sc.advance(1)
			continue
		}
		if sc.openQuote(ch) {
			b.WriteByte(ch)
			sc.advance(1)
			continue
		}
		if end, ok := sc.templateEnd(); ok {
			has = true
			sc.i = end
			continue
		}
		b.WriteByte(ch)
		sc.advance(1)
	}
	return b.String(), has
}

func StrictEnabled(ss ...map[string]string) bool {
	v, ok := strictValue(ss...)
	return ok && v
}

func strictValue(ss ...map[string]string) (bool, bool) {
	set := false
	val := false
	for _, s := range ss {
		v, ok := strictFromMap(s)
		if !ok {
			continue
		}
		set = true
		val = v
	}
	return val, set
}

func strictFromMap(s map[string]string) (bool, bool) {
	if len(s) == 0 {
		return false, false
	}
	states := [3]strictAliasState{}
	for k, raw := range s {
		idx := strictKeyIdx(k)
		if idx < 0 {
			continue
		}
		b, ok := parseBool(raw)
		if !ok {
			continue
		}
		state := &states[idx]
		if !state.set {
			state.set = true
			state.val = b
			continue
		}
		if state.val != b {
			state.conflict = true
			state.val = false
		}
	}
	for i := range strictKeys {
		state := states[i]
		if !state.set {
			continue
		}
		if state.conflict {
			// Conflicting canonicalized declarations resolve to safe default.
			return false, true
		}
		return state.val, true
	}
	return false, false
}

func strictKeyIdx(k string) int {
	nk := strings.ToLower(strings.TrimSpace(k))
	for i := range strictKeys {
		if nk == strictKeys[i] {
			return i
		}
	}
	return -1
}

func parseBool(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "t", "1", "yes", "on":
		return true, true
	case "false", "f", "0", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func HasJSONPathDoubleDot(ex string) bool {
	s := strings.TrimSpace(ex)
	if s == "" {
		return false
	}
	sc := newExprScanner(s)
	for !sc.done() {
		ch := sc.ch()
		if sc.inQuoted(ch) {
			sc.advance(1)
			continue
		}
		if sc.openQuote(ch) {
			sc.advance(1)
			continue
		}
		if hasJSONDoubleDotPrefix(s, sc.i) {
			return true
		}
		sc.advance(1)
	}
	return false
}

func hasJSONDoubleDotPrefix(s string, i int) bool {
	for _, p := range jsonDoubleDotPrefixes {
		if !prefixFold(s, i, p) {
			continue
		}
		if i > 0 {
			c := s[i-1]
			if ident(c) || c == '.' {
				continue
			}
		}
		return true
	}
	return false
}

func prefixFold(s string, i int, p string) bool {
	n := len(p)
	if i+n > len(s) {
		return false
	}
	return strings.EqualFold(s[i:i+n], p)
}

func ident(b byte) bool {
	if b >= 'a' && b <= 'z' {
		return true
	}
	if b >= 'A' && b <= 'Z' {
		return true
	}
	if b >= '0' && b <= '9' {
		return true
	}
	return b == '_'
}

func isQuote(ch byte) bool {
	return ch == '"' || ch == '\''
}

func hasTemplateOpenAt(s string, i int) bool {
	return i+templateOpenLen <= len(s) && s[i:i+templateOpenLen] == templateOpen
}

func nextTemplateEnd(s string, start int) (int, bool) {
	if !hasTemplateOpenAt(s, start) {
		return 0, false
	}
	bodyStart := start + templateOpenLen
	closeRel := strings.Index(s[bodyStart:], templateClose)
	if closeRel < 0 {
		return 0, false
	}
	return bodyStart + closeRel + templateCloseLen, true
}

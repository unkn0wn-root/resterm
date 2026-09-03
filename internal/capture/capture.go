package capture

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

type quoteScan struct {
	q   byte
	esc bool
}

func (sc *quoteScan) inQuoted(ch byte) bool {
	if sc.q == 0 {
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

func (sc *quoteScan) openQuote(ch byte) bool {
	if !isQuote(ch) {
		return false
	}
	sc.q = ch
	return true
}

type exprScanner struct {
	quoteScan
	s string
	i int
}

func newExprScanner(s string) *exprScanner {
	return &exprScanner{s: s}
}

func (sc *exprScanner) done() bool {
	return sc.i >= len(sc.s)
}

func (sc *exprScanner) ch() byte {
	return sc.s[sc.i]
}

func (sc *exprScanner) advance(n int) {
	sc.i += n
}

func HasUnquotedTemplateMarker(ex string) bool {
	return scanTemplates(ex).has
}

// OpenMarker returns "}}" when ex contains an unclosed template marker.
func OpenMarker(ex string) string {
	if !scanTemplates(ex).open {
		return ""
	}
	return templateClose
}

// MixedTemplateRTSCall reports template markers mixed with RTS call syntax.
// It only checks call syntax to avoid flagging ordinary template prefixes.
func MixedTemplateRTSCall(ex string) bool {
	scan := scanTemplates(ex)
	if !scan.has {
		return false
	}
	return mixedTemplateCallPattern.MatchString(strings.TrimSpace(scan.rem))
}

type templateScan struct {
	rem  string // the text left once the closed markers are cut out
	has  bool   // a marker closed
	open bool   // the last marker never closed
}

type TemplateState uint8

const (
	TemplateNone TemplateState = iota
	TemplateClosed
	TemplateOpen
)

type TemplateScanner struct {
	quoteScan
	pendingOpen  bool
	pendingClose bool
	open         bool
	closed       bool
}

func (s *TemplateScanner) Feed(src string) {
	for i := range len(src) {
		s.feedByte(src[i])
	}
}

func (s *TemplateScanner) State() TemplateState {
	if s.open {
		return TemplateOpen
	}
	if s.closed {
		return TemplateClosed
	}
	return TemplateNone
}

func (s *TemplateScanner) feedByte(ch byte) {
	if s.open {
		s.feedMarkerByte(ch)
		return
	}
	if s.inQuoted(ch) {
		return
	}
	// Clear a pending "{" before a quote starts.
	if s.pendingOpen {
		s.pendingOpen = false
		if ch == '{' {
			s.open = true
			return
		}
	}
	if s.openQuote(ch) {
		return
	}
	if ch == '{' {
		s.pendingOpen = true
	}
}

func (s *TemplateScanner) feedMarkerByte(ch byte) {
	if s.pendingClose {
		s.pendingClose = false
		if ch == '}' {
			s.open = false
			s.closed = true
			return
		}
	}
	if ch == '}' {
		s.pendingClose = true
	}
}

// scanTemplates ignores markers inside quoted RTS strings.
func scanTemplates(ex string) templateScan {
	s := strings.TrimSpace(ex)
	if s == "" {
		return templateScan{}
	}
	sc := newExprScanner(s)
	var scan templateScan
	var b strings.Builder
	b.Grow(len(s))

	for !sc.done() {
		ch := sc.ch()
		if sc.inQuoted(ch) || sc.openQuote(ch) || !strings.HasPrefix(s[sc.i:], templateOpen) {
			b.WriteByte(ch)
			sc.advance(1)
			continue
		}
		body := sc.i + templateOpenLen
		end := strings.Index(s[body:], templateClose)
		if end < 0 {
			scan.rem, scan.open = b.String(), true
			return scan
		}
		scan.has = true
		sc.i = body + end + templateCloseLen
	}
	scan.rem = b.String()
	return scan
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

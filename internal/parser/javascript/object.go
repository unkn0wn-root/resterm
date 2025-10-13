package js

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type node interface {
	write(*strings.Builder, int)
}

type object struct {
	props []prop
}

type prop struct {
	key key
	val node
}

type keyKind int

type key struct {
	name string
	kind keyKind
}

type array struct {
	items []node
}

type literalKind int

type literal struct {
	kind literalKind
	text string
}

const (
	keyIdentifier keyKind = iota
	keyString
	keyNumber
)

const (
	literalString literalKind = iota
	literalNumber
	literalIdentifier
)

func (n *object) write(buf *strings.Builder, indent int) {
	if len(n.props) == 0 {
		buf.WriteString("{}")
		return
	}
	props := n.props
	if len(props) > 1 {
		sorted := make([]prop, len(props))
		copy(sorted, props)
		sort.SliceStable(sorted, func(i, j int) bool {
			pi, pj := sorted[i], sorted[j]
			if pi.key.name == pj.key.name {
				return pi.key.kind < pj.key.kind
			}
			return pi.key.name < pj.key.name
		})
		props = sorted
	}
	buf.WriteString("{\n")
	for i, prop := range props {
		writeIndent(buf, indent+1)
		prop.key.write(buf)
		buf.WriteString(": ")
		prop.val.write(buf, indent+1)
		if i < len(props)-1 {
			buf.WriteString(",")
		}
		buf.WriteByte('\n')
	}
	writeIndent(buf, indent)
	buf.WriteString("}")
}

func (k key) write(buf *strings.Builder) {
	switch k.kind {
	case keyIdentifier, keyNumber:
		buf.WriteString(k.name)
	case keyString:
		if isJSIdentifier(k.name) {
			buf.WriteString(k.name)
		} else {
			buf.WriteString(strconv.Quote(k.name))
		}
	default:
		buf.WriteString(strconv.Quote(k.name))
	}
}

func (n *array) write(buf *strings.Builder, indent int) {
	if len(n.items) == 0 {
		buf.WriteString("[]")
		return
	}

	buf.WriteString("[\n")
	for i, item := range n.items {
		writeIndent(buf, indent+1)
		item.write(buf, indent+1)
		if i < len(n.items)-1 {
			buf.WriteString(",")
		}
		buf.WriteByte('\n')
	}
	writeIndent(buf, indent)
	buf.WriteString("]")
}

func (l *literal) write(buf *strings.Builder, indent int) {
	switch l.kind {
	case literalString:
		if formatted, ok := FormatInlineValue(l.text, indent); ok {
			buf.WriteString(formatted)
			return
		}
		buf.WriteString(strconv.Quote(l.text))
	case literalNumber:
		buf.WriteString(formatNumberLiteral(l.text))
	case literalIdentifier:
		buf.WriteString(l.text)
	}
}

func formatNumberLiteral(text string) string {
	if text == "" {
		return text
	}
	sign := ""
	if text[0] == '+' || text[0] == '-' {
		sign = text[:1]
		text = text[1:]
		if text == "" {
			return sign
		}
	}
	if len(text) >= 2 && text[0] == '0' {
		switch text[1] {
		case 'b', 'B':
			digits := strings.ReplaceAll(text[2:], "_", "")
			if len(digits) <= 4 {
				return sign + text[:2] + digits
			}
			var buf strings.Builder
			buf.Grow(2 + len(digits) + len(digits)/4)
			buf.WriteString(text[:2])
			firstGroup := len(digits) % 4
			if firstGroup == 0 {
				firstGroup = 4
			}
			buf.WriteString(digits[:firstGroup])
			for i := firstGroup; i < len(digits); i += 4 {
				buf.WriteByte('_')
				end := i + 4
				if end > len(digits) {
					end = len(digits)
				}
				buf.WriteString(digits[i:end])
			}
			return sign + buf.String()
		case 'x', 'X', 'o', 'O':
			digits := strings.ReplaceAll(text[2:], "_", "")
			return sign + text[:2] + digits
		}
	}
	return sign + text
}

func writeIndent(buf *strings.Builder, count int) {
	buf.WriteString(strings.Repeat("  ", count))
}

func looksStructured(s string) bool {
	if s == "" {
		return false
	}
	switch s[0] {
	case '{', '[':
		return true
	default:
		return false
	}
}

func FormatValue(src string) (string, error) {
	node, err := parseRelaxed(strings.TrimSpace(src))
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	node.write(&buf, 0)
	return buf.String(), nil
}

func FormatInlineValue(src string, indent int) (string, bool) {
	trimmed := strings.TrimSpace(src)
	if !looksStructured(trimmed) {
		return "", false
	}
	node, err := parseRelaxed(trimmed)
	if err != nil {
		return "", false
	}
	var buf strings.Builder
	node.write(&buf, indent)
	return buf.String(), true
}

type tokenType int

const (
	tokenEOF tokenType = iota
	tokenError
	tokenLBrace
	tokenRBrace
	tokenLBracket
	tokenRBracket
	tokenColon
	tokenComma
	tokenString
	tokenNumber
	tokenIdentifier
	tokenTrue
	tokenFalse
	tokenNull
	tokenNaN
	tokenInfinity
	tokenUndefined
)

type token struct {
	typ  tokenType
	pos  int
	text string
}

type parser struct {
	lx  *lexer
	cur token
	err error
}

func newParser(src string) *parser {
	lx := newLexer(src)
	p := &parser{lx: lx}
	p.advance()
	return p
}

func parseRelaxed(src string) (node, error) {
	p := newParser(src)
	if p.err != nil {
		return nil, p.err
	}
	value, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	if p.cur.typ != tokenEOF {
		return nil, fmt.Errorf("unexpected trailing input at position %d", p.cur.pos)
	}
	return value, nil
}

func (p *parser) advance() {
	if p.err != nil {
		return
	}
	tok := p.lx.nextToken()
	if tok.typ == tokenError {
		p.err = errors.New(tok.text)
	}
	p.cur = tok
}

func (p *parser) parseValue() (node, error) {
	switch p.cur.typ {
	case tokenLBrace:
		return p.parseObject()
	case tokenLBracket:
		return p.parseArray()
	case tokenString:
		lit := &literal{kind: literalString, text: p.cur.text}
		p.advance()
		return lit, nil
	case tokenNumber:
		lit := &literal{kind: literalNumber, text: p.cur.text}
		p.advance()
		return lit, nil
	case tokenTrue:
		p.advance()
		return &literal{kind: literalIdentifier, text: "true"}, nil
	case tokenFalse:
		p.advance()
		return &literal{kind: literalIdentifier, text: "false"}, nil
	case tokenNull:
		p.advance()
		return &literal{kind: literalIdentifier, text: "null"}, nil
	case tokenNaN, tokenInfinity, tokenIdentifier:
		text := p.cur.text
		p.advance()
		return &literal{kind: literalIdentifier, text: text}, nil
	case tokenUndefined:
		p.advance()
		return &literal{kind: literalIdentifier, text: "undefined"}, nil
	default:
		return nil, fmt.Errorf("unexpected token at position %d", p.cur.pos)
	}
}

func (p *parser) parseObject() (node, error) {
	start := p.cur
	p.advance()
	obj := &object{}
	if p.cur.typ == tokenRBrace {
		p.advance()
		return obj, nil
	}
	for {
		key, err := p.parseKey()
		if err != nil {
			return nil, err
		}
		if p.cur.typ != tokenColon {
			return nil, fmt.Errorf("expected ':' after object key at position %d", p.cur.pos)
		}

		p.advance()
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}

		obj.props = append(obj.props, prop{key: key, val: val})
		if p.cur.typ == tokenComma {
			p.advance()
			if p.cur.typ == tokenRBrace {
				break
			}
			continue
		}
		break
	}

	if p.cur.typ != tokenRBrace {
		return nil, fmt.Errorf("expected '}' to close object starting at position %d", start.pos)
	}
	p.advance()
	return obj, nil
}

func (p *parser) parseArray() (node, error) {
	p.advance()
	arr := &array{}
	if p.cur.typ == tokenRBracket {
		p.advance()
		return arr, nil
	}
	for {
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}

		arr.items = append(arr.items, val)
		if p.cur.typ == tokenComma {
			p.advance()
			if p.cur.typ == tokenRBracket {
				break
			}
			continue
		}
		break
	}

	if p.cur.typ != tokenRBracket {
		return nil, fmt.Errorf("expected ']' to close array")
	}
	p.advance()
	return arr, nil
}

func (p *parser) parseKey() (key, error) {
	tok := p.cur
	switch tok.typ {
	case tokenString:
		p.advance()
		return key{name: tok.text, kind: keyString}, nil
	case tokenIdentifier:
		p.advance()
		return key{name: tok.text, kind: keyIdentifier}, nil
	case tokenNumber:
		p.advance()
		return key{name: tok.text, kind: keyNumber}, nil
	case tokenTrue:
		p.advance()
		return key{name: "true", kind: keyString}, nil
	case tokenFalse:
		p.advance()
		return key{name: "false", kind: keyString}, nil
	case tokenNull:
		p.advance()
		return key{name: "null", kind: keyString}, nil
	case tokenNaN, tokenInfinity, tokenUndefined:
		p.advance()
		return key{name: tok.text, kind: keyString}, nil
	default:
		return key{}, fmt.Errorf("unexpected token for object key at position %d", tok.pos)
	}
}

type lexer struct {
	src   string
	pos   int
	width int
}

func newLexer(src string) *lexer {
	return &lexer{src: src}
}

func (l *lexer) nextToken() token {
	if err := l.skipSpace(); err != nil {
		return token{typ: tokenError, pos: l.pos, text: err.Error()}
	}
	start := l.pos
	r := l.peek()
	switch r {
	case eofRune:
		return token{typ: tokenEOF, pos: start}
	case '{':
		l.next()
		return token{typ: tokenLBrace, pos: start}
	case '}':
		l.next()
		return token{typ: tokenRBrace, pos: start}
	case '[':
		l.next()
		return token{typ: tokenLBracket, pos: start}
	case ']':
		l.next()
		return token{typ: tokenRBracket, pos: start}
	case ':':
		l.next()
		return token{typ: tokenColon, pos: start}
	case ',':
		l.next()
		return token{typ: tokenComma, pos: start}
	case '"', '\'', '`':
		return l.scanString()
	case '+', '-':
		return l.scanSign()
	case '.':
		if isDigit(l.peekNext()) {
			return l.scanNumber()
		}
	}
	if isDigit(r) {
		return l.scanNumber()
	}
	if isIdentifierStart(r) {
		return l.scanIdentifier()
	}
	return token{typ: tokenError, pos: start, text: fmt.Sprintf("unexpected character %q", r)}
}

const eofRune = rune(0)

func (l *lexer) skipSpace() error {
	for {
		r := l.peek()
		switch {
		case r == eofRune:
			return nil
		case unicode.IsSpace(r):
			l.next()
		case r == '/':
			consumed, err := l.skipComment()
			if err != nil {
				return err
			}
			if consumed {
				continue
			}
			return nil
		default:
			return nil
		}
	}
}

func (l *lexer) skipComment() (bool, error) {
	l.next()
	switch l.peek() {
	case '/':
		l.next()
		for {
			r := l.next()
			if r == '\n' || r == eofRune {
				break
			}
		}
		return true, nil
	case '*':
		l.next()
		for {
			r := l.next()
			if r == eofRune {
				return false, fmt.Errorf("unterminated block comment")
			}
			if r == '*' && l.peek() == '/' {
				l.next()
				return true, nil
			}
		}
	default:
		l.backup()
		return false, nil
	}
}

func (l *lexer) scanString() token {
	start := l.pos
	quote := l.next()
	var buf strings.Builder
	for {
		r := l.next()
		switch r {
		case eofRune:
			return token{typ: tokenError, pos: start, text: "unterminated string"}
		case '\n':
			if quote != '`' {
				return token{typ: tokenError, pos: start, text: "newline in string"}
			}
			buf.WriteRune(r)
		case '\\':
			esc := l.next()
			if esc == eofRune {
				return token{typ: tokenError, pos: start, text: "unterminated escape sequence"}
			}
			if esc == 'u' {
				val, ok := l.readUnicodeEscape()
				if !ok {
					return token{typ: tokenError, pos: start, text: "invalid unicode escape"}
				}
				buf.WriteRune(val)
				continue
			}
			if esc == 'x' {
				val, ok := l.readHexEscape()
				if !ok {
					return token{typ: tokenError, pos: start, text: "invalid hex escape"}
				}
				buf.WriteRune(val)
				continue
			}
			if val, ok := decodeSimpleEscape(esc); ok {
				buf.WriteRune(val)
				continue
			}
			return token{typ: tokenError, pos: start, text: fmt.Sprintf("invalid escape \\%c", esc)}
		default:
			if r == quote {
				return token{typ: tokenString, pos: start, text: buf.String()}
			}
			buf.WriteRune(r)
		}
	}
}

func decodeSimpleEscape(r rune) (rune, bool) {
	switch r {
	case 'b':
		return '\b', true
	case 'f':
		return '\f', true
	case 'n':
		return '\n', true
	case 'r':
		return '\r', true
	case 't':
		return '\t', true
	case '\\':
		return '\\', true
	case '\'':
		return '\'', true
	case '"':
		return '"', true
	case '/':
		return '/', true
	default:
		return 0, false
	}
}

func (l *lexer) readUnicodeEscape() (rune, bool) {
	if l.peek() == '{' {
		l.next()
		var hex strings.Builder
		for {
			r := l.next()
			if r == '}' {
				if hex.Len() == 0 {
					return 0, false
				}

				val, err := strconv.ParseInt(hex.String(), 16, 32)
				return rune(val), err == nil
			}

			if !isHexDigit(r) {
				return 0, false
			}
			hex.WriteRune(r)
		}
	}

	digits := make([]rune, 4)
	for i := 0; i < 4; i++ {
		r := l.next()
		if !isHexDigit(r) {
			return 0, false
		}
		digits[i] = r
	}

	val, err := strconv.ParseInt(string(digits), 16, 32)
	return rune(val), err == nil
}

func (l *lexer) readHexEscape() (rune, bool) {
	digits := make([]rune, 2)
	for i := 0; i < 2; i++ {
		r := l.next()
		if !isHexDigit(r) {
			return 0, false
		}
		digits[i] = r
	}

	val, err := strconv.ParseInt(string(digits), 16, 32)
	return rune(val), err == nil
}

func (l *lexer) scanSign() token {
	start := l.pos
	sign := l.next()
	next := l.peek()
	if next == eofRune {
		return token{typ: tokenError, pos: start, text: "unexpected end after sign"}
	}

	if next == 'I' {
		if !l.consumeWord("Infinity") {
			return token{typ: tokenError, pos: start, text: "invalid signed literal"}
		}
		if isIdentifierPart(l.peek()) {
			return token{typ: tokenError, pos: start, text: "invalid signed literal"}
		}
		text := string(sign) + "Infinity"
		return token{typ: tokenInfinity, pos: start, text: text}
	}

	if next == 'N' {
		if !l.consumeWord("NaN") {
			return token{typ: tokenError, pos: start, text: "invalid signed literal"}
		}
		if isIdentifierPart(l.peek()) {
			return token{typ: tokenError, pos: start, text: "invalid signed literal"}
		}
		text := string(sign) + "NaN"
		return token{typ: tokenNaN, pos: start, text: text}
	}

	if isDigit(next) || next == '.' {
		l.backup()
		return l.scanNumber()
	}
	return token{typ: tokenError, pos: start, text: "unexpected sign"}
}

func (l *lexer) scanNumber() token {
	start := l.pos
	var buf strings.Builder
	if ch := l.peek(); ch == '+' || ch == '-' {
		buf.WriteRune(l.next())
	}

	if l.peek() == '0' {
		buf.WriteRune(l.next())
		if p := l.peek(); p == 'x' || p == 'X' {
			buf.WriteRune(l.next())
			digits, err := l.scanDigits(&buf, isHexDigit)
			if err != nil {
				return token{typ: tokenError, pos: start, text: err.Error()}
			}
			if digits == 0 {
				return token{typ: tokenError, pos: start, text: "invalid hex literal"}
			}
			return token{typ: tokenNumber, pos: start, text: buf.String()}
		}

		if p := l.peek(); p == 'b' || p == 'B' {
			buf.WriteRune(l.next())
			digits, err := l.scanDigits(&buf, isBinaryDigit)
			if err != nil {
				return token{typ: tokenError, pos: start, text: err.Error()}
			}
			if digits == 0 {
				return token{typ: tokenError, pos: start, text: "invalid binary literal"}
			}
			return token{typ: tokenNumber, pos: start, text: buf.String()}
		}

		if p := l.peek(); p == 'o' || p == 'O' {
			buf.WriteRune(l.next())
			digits, err := l.scanDigits(&buf, isOctalDigit)
			if err != nil {
				return token{typ: tokenError, pos: start, text: err.Error()}
			}
			if digits == 0 {
				return token{typ: tokenError, pos: start, text: "invalid octal literal"}
			}
			return token{typ: tokenNumber, pos: start, text: buf.String()}
		}
	}

	digits, err := l.scanDigits(&buf, isDigit)
	if err != nil {
		return token{typ: tokenError, pos: start, text: err.Error()}
	}
	if digits == 0 && l.peek() != '.' {
		return token{typ: tokenError, pos: start, text: "invalid number"}
	}

	if l.peek() == '.' {
		buf.WriteRune(l.next())
		if _, err := l.scanDigits(&buf, isDigit); err != nil {
			return token{typ: tokenError, pos: start, text: err.Error()}
		}
	}

	if p := l.peek(); p == 'e' || p == 'E' {
		buf.WriteRune(l.next())
		if s := l.peek(); s == '+' || s == '-' {
			buf.WriteRune(l.next())
		}
		digits, err := l.scanDigits(&buf, isDigit)
		if err != nil {
			return token{typ: tokenError, pos: start, text: err.Error()}
		}
		if digits == 0 {
			return token{typ: tokenError, pos: start, text: "invalid exponent"}
		}
	}
	return token{typ: tokenNumber, pos: start, text: buf.String()}
}

func (l *lexer) scanIdentifier() token {
	start := l.pos
	var buf strings.Builder
	buf.WriteRune(l.next())
	for {
		r := l.peek()
		if !isIdentifierPart(r) {
			break
		}
		buf.WriteRune(l.next())
	}
	text := buf.String()
	switch text {
	case "true":
		return token{typ: tokenTrue, pos: start}
	case "false":
		return token{typ: tokenFalse, pos: start}
	case "null":
		return token{typ: tokenNull, pos: start}
	case "NaN":
		return token{typ: tokenNaN, pos: start, text: text}
	case "Infinity":
		return token{typ: tokenInfinity, pos: start, text: text}
	case "undefined":
		return token{typ: tokenUndefined, pos: start}
	default:
		return token{typ: tokenIdentifier, pos: start, text: text}
	}
}

func (l *lexer) consumeWord(word string) bool {
	for _, r := range word {
		if l.peek() != r {
			return false
		}
		l.next()
	}
	return true
}

func (l *lexer) scanDigits(buf *strings.Builder, valid func(rune) bool) (int, error) {
	count := 0
	lastWasDigit := false
	for {
		r := l.peek()
		switch {
		case valid(r):
			buf.WriteRune(l.next())
			count++
			lastWasDigit = true
		case r == '_':
			if !lastWasDigit {
				return count, fmt.Errorf("invalid numeric separator placement")
			}
			l.next()
			next := l.peek()
			if !valid(next) {
				return count, fmt.Errorf("invalid numeric separator placement")
			}
			lastWasDigit = false
		default:
			return count, nil
		}
	}
}

func (l *lexer) next() rune {
	if l.pos >= len(l.src) {
		l.width = 0
		return eofRune
	}
	r, w := utf8.DecodeRuneInString(l.src[l.pos:])
	l.pos += w
	l.width = w
	return r
}

func (l *lexer) peek() rune {
	r := l.next()
	l.backup()
	return r
}

func (l *lexer) peekNext() rune {
	l.next()
	r := l.next()
	l.backup()
	l.backup()
	return r
}

func (l *lexer) backup() {
	l.pos -= l.width
	if l.pos < 0 {
		l.pos = 0
	}
}

func isIdentifierStart(r rune) bool {
	return r == '$' || r == '_' || unicode.IsLetter(r)
}

func isIdentifierPart(r rune) bool {
	return isIdentifierStart(r) || unicode.IsDigit(r)
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func isHexDigit(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func isBinaryDigit(r rune) bool {
	return r == '0' || r == '1'
}

func isOctalDigit(r rune) bool {
	return r >= '0' && r <= '7'
}

func isJSIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if !isIdentifierStart(r) {
				return false
			}
			continue
		}
		if !isIdentifierPart(r) {
			return false
		}
	}
	return true
}

package rts

import (
	"fmt"
	"strconv"
)

type controlKind uint8

const (
	controlSwitch controlKind = iota
	controlLoop
)

// controlTarget is one lexically enclosing for or switch. label is empty for
// unlabeled statements and pos points at the label that opened the target.
type controlTarget struct {
	kind  controlKind
	label string
	pos   Pos
}

// stmtLabel is the optional name in front of a for or switch. The zero value
// means the statement carries no label.
type stmtLabel struct {
	name string
	pos  Pos
}

// parser builds the syntax tree of RTS code from the tokens of a lexer.
type parser struct {
	lx    *lexer
	cur   token
	peek  token
	ahead []token

	// controls holds every for and switch that encloses the statement being
	// parsed, innermost last, so break and continue can be checked against them.
	controls []controlTarget
}

// newParserAt returns a parser for src, a snippet that starts at the line and
// column of at. Its positions are in at's file.
func newParserAt(src []byte, at Pos) *parser {
	lx := newLexerAt(src, at)
	p := &parser{lx: lx}
	p.cur = lx.scan()
	p.peek = lx.scan()
	return p
}

// ParseModule parses the whole text of f.
func ParseModule(f *File) (*Mod, error) {
	return parseModuleAt(f.src, f.Start())
}

// parseModuleAt parses src as a snippet that starts at the line and column of
// at. Its positions are in at's file.
func parseModuleAt(src []byte, at Pos) (m *Mod, err error) {
	defer func() {
		if r := recover(); r != nil {
			if pe, ok := r.(*ParseError); ok {
				err = pe
				m = nil
				return
			}
			panic(r)
		}
	}()
	p := newParserAt(src, at)
	m = p.parseMod(at)
	return m, err
}

// ParseExpr parses src as an expression that starts at the line and column of
// at. Its positions are in at's file.
func ParseExpr(src string, at Pos) (ex Expr, err error) {
	defer func() {
		if r := recover(); r != nil {
			if pe, ok := r.(*ParseError); ok {
				err = pe
				ex = nil
				return
			}
			panic(r)
		}
	}()
	p := newParserAt([]byte(src), at)
	ex = p.parseExprOnly()
	return ex, err
}

func (p *parser) parseMod(start Pos) *Mod {
	m := &Mod{Start: start}
	for p.cur.K != tokEOF {
		if p.isSemi(p.cur.K) {
			p.next()
			continue
		}
		if p.cur.K == tokModule {
			if m.Name != "" || len(m.Stmts) > 0 {
				p.fail(p.cur.P, "module must appear before statements")
			}
			nm, pos := p.parseModDecl()
			m.Name = nm
			m.NamePos = pos
			p.skipSemi()
			continue
		}
		st := p.parseStmt()
		m.Stmts = append(m.Stmts, st)
		p.skipSemi()
	}
	return m
}

func (p *parser) parseModDecl() (string, Pos) {
	pos := p.expect(tokModule).P
	// the module name becomes the binding when @use omits an alias
	if p.cur.K.isKeyword() {
		p.fail(p.cur.P, fmt.Sprintf("module name cannot be the reserved word %s", p.cur.K))
	}
	if p.cur.K != tokIdent {
		p.fail(p.cur.P, "module requires a name")
	}
	nm := p.cur.Lit
	p.next()
	return nm, pos
}

func (p *parser) parseExprOnly() Expr {
	ex := p.parseExpr()
	for p.isSemi(p.cur.K) {
		p.next()
	}
	if p.cur.K != tokEOF {
		p.failCur(fmt.Sprintf("unexpected %s", p.cur.K))
	}
	return ex
}

func (p *parser) parseStmt() Stmt {
	switch p.cur.K {
	case tokExport:
		return p.parseExport()
	case tokLet:
		return p.parseLet(false, false)
	case tokConst:
		return p.parseLet(false, true)
	case tokFn:
		return p.parseFn(false)
	case tokIf:
		return p.parseIf()
	case tokSwitch:
		return p.parseSwitch(stmtLabel{})
	case tokCase:
		p.fail(p.cur.P, "case outside switch body")
	case tokDefault:
		p.fail(p.cur.P, "default outside switch body")
	case tokFor:
		return p.parseFor(stmtLabel{})
	case tokBreak:
		return p.parseBreak()
	case tokContinue:
		return p.parseContinue()
	case tokReturn:
		return p.parseReturn()
	case tokIdent:
		if p.peek.K == tokAssign {
			return p.parseAssign()
		}
		if p.peek.K == tokColon {
			return p.parseLabeled()
		}
	}
	return p.parseExprStmt()
}

func (p *parser) parseExport() Stmt {
	p.expect(tokExport)
	switch p.cur.K {
	case tokLet:
		return p.parseLet(true, false)
	case tokConst:
		return p.parseLet(true, true)
	case tokFn:
		return p.parseFn(true)
	default:
		p.fail(p.cur.P, "export must be followed by let, const, or fn")
		return nil
	}
}

func (p *parser) parseLet(exp bool, isConst bool) Stmt {
	var pos Pos
	if isConst {
		pos = p.expect(tokConst).P
	} else {
		pos = p.expect(tokLet).P
	}

	name := p.expect(tokIdent).Lit
	p.expect(tokAssign)
	val := p.parseExpr()
	return &LetStmt{P: pos, Exported: exp, Const: isConst, Name: name, Val: val}
}

func (p *parser) parseAssign() Stmt {
	tok := p.expect(tokIdent)
	pos := tok.P
	name := tok.Lit
	p.expect(tokAssign)
	val := p.parseExpr()
	return &AssignStmt{P: pos, Name: name, Val: val}
}

func (p *parser) parseReturn() Stmt {
	pos := p.expect(tokReturn).P
	if p.cur.K == tokEOF || p.cur.K == tokRbrace || p.isSemi(p.cur.K) {
		return &ReturnStmt{P: pos}
	}
	val := p.parseExpr()
	return &ReturnStmt{P: pos, Val: val}
}

func (p *parser) parseExprStmt() Stmt {
	pos := p.cur.P
	ex := p.parseExpr()
	return &ExprStmt{P: pos, Exp: ex}
}

func (p *parser) parseFn(exp bool) Stmt {
	pos := p.expect(tokFn).P
	name := p.expect(tokIdent).Lit
	p.expect(tokLparen)
	params := p.parseParams()
	p.expect(tokRparen)
	saved := p.controls
	p.controls = nil
	body := p.parseBlock()
	p.controls = saved
	return &FnDef{P: pos, Exported: exp, Name: name, Params: params, Body: body}
}

func (p *parser) parseParams() []string {
	if p.cur.K == tokRparen {
		return nil
	}
	var out []string
	seen := map[string]struct{}{}
	for {
		if p.cur.K != tokIdent {
			p.fail(p.cur.P, "expected parameter name")
		}

		name := p.cur.Lit
		if _, ok := seen[name]; ok {
			p.fail(p.cur.P, fmt.Sprintf("duplicate parameter %q", name))
		}
		seen[name] = struct{}{}
		out = append(out, name)
		p.next()
		if p.cur.K == tokComma {
			p.next()
			if p.cur.K == tokRparen {
				break
			}
			continue
		}
		break
	}
	return out
}

func (p *parser) parseIf() Stmt {
	pos := p.expect(tokIf).P
	cond := p.parseExpr()
	then := p.parseBlock()
	var elifs []Elif
	for p.cur.K == tokElif {
		p.next()
		c := p.parseExpr()
		b := p.parseBlock()
		elifs = append(elifs, Elif{Cond: c, Body: b})
	}

	var els *Block
	if p.cur.K == tokElse {
		p.next()
		els = p.parseBlock()
	}
	return &IfStmt{P: pos, Cond: cond, Then: then, Elifs: elifs, Else: els}
}

// parseLabeled reads "name:" in front of a for or switch. Labels live in their
// own namespace and decorate nothing else, so there is no general labeled
// statement and no goto.
func (p *parser) parseLabeled() Stmt {
	tok := p.cur
	if tok.Lit == "_" {
		p.fail(tok.P, fmt.Sprintf("invalid label %q", tok.Lit))
	}
	if t := p.findControl(tok.Lit); t != nil {
		p.fail(
			tok.P,
			fmt.Sprintf("duplicate label %q, active since %d:%d", tok.Lit, t.pos.Line, t.pos.Col),
		)
	}
	p.next()
	p.next()

	lb := stmtLabel{name: tok.Lit, pos: tok.P}
	switch p.cur.K {
	case tokFor:
		return p.parseFor(lb)
	case tokSwitch:
		return p.parseSwitch(lb)
	default:
		p.failCur(fmt.Sprintf("label must be followed by for or switch, got %s", p.cur.K))
		return nil
	}
}

func (p *parser) pushControl(kind controlKind, lb stmtLabel) {
	p.controls = append(p.controls, controlTarget{kind: kind, label: lb.name, pos: lb.pos})
}

func (p *parser) popControl() {
	p.controls = p.controls[:len(p.controls)-1]
}

// findControl looks up an active label. It is never called with an empty name,
// which would match the unlabeled targets.
func (p *parser) findControl(label string) *controlTarget {
	for i := len(p.controls) - 1; i >= 0; i-- {
		if p.controls[i].label == label {
			return &p.controls[i]
		}
	}
	return nil
}

func (p *parser) inLoop() bool {
	for _, c := range p.controls {
		if c.kind == controlLoop {
			return true
		}
	}
	return false
}

// labelRef reads the optional target of a break or continue. The lexer inserts
// a semicolon after both keywords, so a label only parses on the same line.
func (p *parser) labelRef() (token, bool) {
	if p.cur.K != tokIdent {
		return token{}, false
	}
	tok := p.cur
	p.next()
	return tok, true
}

// parseSwitch reads both switch forms. A brace right after the keyword means
// the tagless form, mirroring Go: a dict literal tag has to be parenthesized.
func (p *parser) parseSwitch(lb stmtLabel) Stmt {
	pos := p.expect(tokSwitch).P
	var tag Expr
	if p.cur.K != tokLbrace {
		tag = p.parseExpr()
	}
	p.expect(tokLbrace)
	p.pushControl(controlSwitch, lb)
	clauses := p.parseCaseClauses()
	p.popControl()
	p.expect(tokRbrace)
	return &SwitchStmt{P: pos, Label: lb.name, LabelPos: lb.pos, Tag: tag, Clauses: clauses}
}

func (p *parser) parseCaseClauses() []CaseClause {
	var out []CaseClause
	seenDefault := false
	for {
		p.skipSemi()
		switch p.cur.K {
		case tokRbrace:
			return out
		case tokEOF:
			p.fail(p.cur.P, "unterminated switch")
		case tokCase:
			out = append(out, p.parseCase())
		case tokDefault:
			if seenDefault {
				p.fail(p.cur.P, "duplicate default in switch")
			}
			seenDefault = true
			out = append(out, p.parseDefault())
		default:
			p.failCur(fmt.Sprintf("expected case or default, got %s", p.cur.K))
		}
	}
}

func (p *parser) parseCase() CaseClause {
	pos := p.expect(tokCase).P
	exprs := p.parseCaseExprs()
	colon := p.expect(tokColon).P
	return CaseClause{P: pos, Exprs: exprs, Body: p.parseClauseBody(colon)}
}

func (p *parser) parseCaseExprs() []Expr {
	var out []Expr
	for {
		if p.cur.K == tokColon {
			p.fail(p.cur.P, "case requires an expression")
		}
		out = append(out, p.parseExpr())
		if p.cur.K != tokComma {
			return out
		}
		p.next()
	}
}

func (p *parser) parseDefault() CaseClause {
	pos := p.expect(tokDefault).P
	colon := p.expect(tokColon).P
	return CaseClause{P: pos, Body: p.parseClauseBody(colon)}
}

func (p *parser) parseClauseBody(pos Pos) *Block {
	var out []Stmt
	for {
		p.skipSemi()
		if p.cur.K == tokRbrace || p.cur.K == tokCase || p.cur.K == tokDefault {
			return &Block{P: pos, Stmts: out}
		}
		if p.cur.K == tokEOF {
			p.fail(p.cur.P, "unterminated switch")
		}
		out = append(out, p.parseStmt())
	}
}

func (p *parser) parseFor(lb stmtLabel) Stmt {
	pos := p.expect(tokFor).P
	if p.cur.K == tokLbrace {
		return p.parseForBody(pos, lb, nil, nil, nil, nil)
	}
	if p.isForRangeStart() {
		return p.parseForRange(pos, lb)
	}

	if p.isSemi(p.cur.K) {
		p.next()
		cond := p.parseForCond()
		p.expectSemi()
		post := p.parseForPost()
		return p.parseForBody(pos, lb, nil, cond, post, nil)
	}

	init := p.parseForInit()
	if p.isSemi(p.cur.K) {
		p.next()
		cond := p.parseForCond()
		p.expectSemi()
		post := p.parseForPost()
		return p.parseForBody(pos, lb, init, cond, post, nil)
	}

	exprStmt, ok := init.(*ExprStmt)
	if !ok {
		p.fail(init.Pos(), "for condition must be expression")
	}
	return p.parseForBody(pos, lb, nil, exprStmt.Exp, nil, nil)
}

func (p *parser) isForRangeStart() bool {
	tok := p.cur
	idx := 0
	if tok.K == tokLet {
		tok = p.peekN(1)
		if tok.K != tokIdent {
			return false
		}
		idx = 1
	} else if tok.K != tokIdent {
		return false
	}

	next := p.peekN(idx + 1)
	if next.K == tokComma {
		if p.peekN(idx+2).K != tokIdent {
			return false
		}
		next = p.peekN(idx + 3)
	}
	return next.K == tokRange
}

func (p *parser) parseForRange(pos Pos, lb stmtLabel) Stmt {
	decl := false
	if p.cur.K == tokLet {
		decl = true
		p.next()
	}

	keyTok := p.expect(tokIdent)
	key := keyTok.Lit
	val := ""
	if p.cur.K == tokComma {
		p.next()
		valTok := p.expect(tokIdent)
		val = valTok.Lit
	}

	if val != "" && key == val && key != "_" {
		p.fail(keyTok.P, "range variables must be distinct")
	}

	p.expect(tokRange)
	src := p.parseExpr()
	rng := &ForRange{Key: key, Val: val, Expr: src, Declare: decl}
	return p.parseForBody(pos, lb, nil, nil, nil, rng)
}

func (p *parser) parseForCond() Expr {
	if p.isSemi(p.cur.K) {
		return nil
	}
	return p.parseExpr()
}

func (p *parser) parseForInit() Stmt {
	return p.parseForClause(true, "init")
}

func (p *parser) parseForPost() Stmt {
	if p.cur.K == tokLbrace {
		return nil
	}
	return p.parseForClause(false, "post")
}

func (p *parser) parseForClause(allowLet bool, label string) Stmt {
	switch p.cur.K {
	case tokLet:
		if !allowLet {
			p.fail(p.cur.P, fmt.Sprintf("for %s clause cannot use let", label))
		}
		return p.parseLet(false, false)
	case tokConst:
		p.fail(p.cur.P, fmt.Sprintf("for %s clause cannot use const", label))
	case tokExport, tokFn, tokIf, tokSwitch, tokFor, tokReturn, tokBreak, tokContinue, tokRange:
		p.fail(p.cur.P, fmt.Sprintf("invalid for %s clause", label))
	case tokIdent:
		if p.peek.K == tokAssign {
			return p.parseAssign()
		}
	}
	return p.parseExprStmt()
}

func (p *parser) parseForBody(
	pos Pos,
	lb stmtLabel,
	init Stmt,
	cond Expr,
	post Stmt,
	rng *ForRange,
) Stmt {
	p.pushControl(controlLoop, lb)
	body := p.parseBlock()
	p.popControl()
	return &ForStmt{
		P:        pos,
		Label:    lb.name,
		LabelPos: lb.pos,
		Init:     init,
		Cond:     cond,
		Post:     post,
		Range:    rng,
		Body:     body,
	}
}

func (p *parser) parseBreak() Stmt {
	pos := p.expect(tokBreak).P
	tok, ok := p.labelRef()
	if !ok {
		if len(p.controls) == 0 {
			p.fail(pos, "break outside loop or switch")
		}
		return &BreakStmt{P: pos}
	}
	if p.findControl(tok.Lit) == nil {
		p.fail(tok.P, fmt.Sprintf("undefined label %q", tok.Lit))
	}
	return &BreakStmt{P: pos, Label: tok.Lit}
}

func (p *parser) parseContinue() Stmt {
	pos := p.expect(tokContinue).P
	tok, ok := p.labelRef()
	if !ok {
		if !p.inLoop() {
			p.fail(pos, "continue outside loop")
		}
		return &ContinueStmt{P: pos}
	}

	t := p.findControl(tok.Lit)
	if t == nil {
		p.fail(tok.P, fmt.Sprintf("undefined label %q", tok.Lit))
	}
	if t.kind != controlLoop {
		p.fail(tok.P, fmt.Sprintf("continue label %q is not a loop", tok.Lit))
	}
	return &ContinueStmt{P: pos, Label: tok.Lit}
}

func (p *parser) parseBlock() *Block {
	pos := p.expect(tokLbrace).P
	var out []Stmt
	for p.cur.K != tokRbrace {
		if p.cur.K == tokEOF {
			p.fail(p.cur.P, "unterminated block")
		}

		if p.isSemi(p.cur.K) {
			p.next()
			continue
		}

		st := p.parseStmt()
		out = append(out, st)
		p.skipSemi()
	}
	p.expect(tokRbrace)
	return &Block{P: pos, Stmts: out}
}

func (p *parser) parseExpr() Expr {
	return p.parseTernary()
}

type binOpMatcher func(tokKind) (BinOp, bool)

func (p *parser) parseBinary(next func() Expr, opOf binOpMatcher) Expr {
	left := next()
	for {
		op, ok := opOf(p.cur.K)
		if !ok {
			return left
		}
		pos := p.cur.P
		p.next()
		right := next()
		left = &Binary{P: pos, Op: op, Left: left, Right: right}
	}
}

func (p *parser) parseTernary() Expr {
	cond := p.parseCoalesce()
	if p.cur.K != tokQuestion {
		return cond
	}

	pos := p.cur.P
	p.next()
	then := p.parseExpr()
	p.expect(tokColon)
	els := p.parseExpr()
	return &Ternary{P: pos, Cond: cond, Then: then, Else: els}
}

func (p *parser) parseCoalesce() Expr {
	return p.parseBinary(p.parseOr, coalesceOp)
}

func (p *parser) parseOr() Expr {
	return p.parseBinary(p.parseAnd, orOp)
}

func (p *parser) parseAnd() Expr {
	return p.parseBinary(p.parseEq, andOp)
}

func (p *parser) parseEq() Expr {
	return p.parseBinary(p.parseCmp, eqOp)
}

func (p *parser) parseCmp() Expr {
	left := p.parseAdd()
	for {
		pos := p.cur.P
		op, ok := cmpOp(p.cur.K)
		membership := false
		switch {
		case ok:
			p.next()
		case p.cur.K == tokIdent && p.cur.Lit == "in":
			op = OpIn
			membership = true
			p.next()
		case p.cur.K == tokNot && p.peek.K == tokIdent && p.peek.Lit == "in":
			op = OpNotIn
			membership = true
			p.next()
			p.next()
		default:
			return left
		}

		// in stays an identifier in the lexer, so it receives an automatic
		// semicolon at a line boundary. Once the parser has identified it as
		// an operator, that semicolon is continuation rather than punctuation.
		if membership && p.cur.K == tokAutoSemi {
			p.next()
		}
		right := p.parseAdd()
		left = &Binary{P: pos, Op: op, Left: left, Right: right}
	}
}

func (p *parser) parseAdd() Expr {
	return p.parseBinary(p.parseMul, addOp)
}

func (p *parser) parseMul() Expr {
	return p.parseBinary(p.parseUnary, mulOp)
}

func coalesceOp(k tokKind) (BinOp, bool) {
	if k == tokCoalesce {
		return OpCoalesce, true
	}
	return 0, false
}

func orOp(k tokKind) (BinOp, bool) {
	switch k {
	case tokOr, tokOrOr:
		return OpOr, true
	default:
		return 0, false
	}
}

func andOp(k tokKind) (BinOp, bool) {
	switch k {
	case tokAnd, tokAndAnd:
		return OpAnd, true
	default:
		return 0, false
	}
}

func eqOp(k tokKind) (BinOp, bool) {
	switch k {
	case tokEq:
		return OpEq, true
	case tokNe:
		return OpNe, true
	default:
		return 0, false
	}
}

func cmpOp(k tokKind) (BinOp, bool) {
	switch k {
	case tokLt:
		return OpLt, true
	case tokLe:
		return OpLe, true
	case tokGt:
		return OpGt, true
	case tokGe:
		return OpGe, true
	default:
		return 0, false
	}
}

func addOp(k tokKind) (BinOp, bool) {
	switch k {
	case tokPlus:
		return OpAdd, true
	case tokMinus:
		return OpSub, true
	default:
		return 0, false
	}
}

func mulOp(k tokKind) (BinOp, bool) {
	switch k {
	case tokStar:
		return OpMul, true
	case tokSlash:
		return OpDiv, true
	case tokPercent:
		return OpMod, true
	default:
		return 0, false
	}
}

// unaryOp maps the word and symbol forms onto the same operator, so not x and
// !x parse to one node.
func unaryOp(k tokKind) (UnOp, bool) {
	switch k {
	case tokNot, tokBang:
		return UnNot, true
	case tokMinus:
		return UnNeg, true
	default:
		return 0, false
	}
}

func (p *parser) parseUnary() Expr {
	if p.cur.K == tokTry {
		pos := p.cur.P
		p.next()
		x := p.parseUnary()
		return &TryExpr{P: pos, X: x}
	}

	if op, ok := unaryOp(p.cur.K); ok {
		pos := p.cur.P
		p.next()
		x := p.parseUnary()
		return &Unary{P: pos, Op: op, X: x}
	}
	return p.parsePostfix()
}

func (p *parser) parsePostfix() Expr {
	left := p.parsePrimary()
	for {
		switch p.cur.K {
		case tokLparen:
			pos := p.cur.P
			p.next()
			args := p.parseArgs()
			p.expect(tokRparen)
			left = &Call{P: pos, Callee: left, Args: args}
		case tokLbrack:
			pos := p.cur.P
			p.next()
			idx := p.parseExpr()
			p.expect(tokRbrack)
			left = &Index{P: pos, X: left, Idx: idx}
		case tokDot:
			pos := p.cur.P
			p.next()
			// member access has no quoted form, so the fix here is an index
			if p.cur.K.isKeyword() {
				p.fail(p.cur.P, fmt.Sprintf(
					"field name cannot be the reserved word %s, use [%q] instead", p.cur.K, p.cur.K,
				))
			}
			name := p.expect(tokIdent).Lit
			left = &Member{P: pos, X: left, Name: name}
		default:
			return left
		}
	}
}

func (p *parser) parseArgs() []Expr {
	if p.cur.K == tokRparen {
		return nil
	}

	var out []Expr
	for {
		out = append(out, p.parseExpr())
		if p.cur.K == tokComma {
			p.next()
			if p.cur.K == tokRparen {
				break
			}
			continue
		}
		break
	}
	return out
}

func (p *parser) parsePrimary() Expr {
	switch p.cur.K {
	case tokIdent:
		pos := p.cur.P
		name := p.cur.Lit
		p.next()
		return &Ident{P: pos, Name: name}
	case tokNumber:
		pos := p.cur.P
		lit := p.cur.Lit
		p.next()
		n, err := strconv.ParseFloat(lit, 64)
		if err != nil {
			p.fail(pos, "invalid number")
		}
		return &Literal{P: pos, Kind: LitNum, N: n}
	case tokString:
		pos := p.cur.P
		lit := p.cur.Lit
		p.next()
		return &Literal{P: pos, Kind: LitStr, S: lit}
	case tokTrue:
		pos := p.cur.P
		p.next()
		return &Literal{P: pos, Kind: LitBool, B: true}
	case tokFalse:
		pos := p.cur.P
		p.next()
		return &Literal{P: pos, Kind: LitBool, B: false}
	case tokNull:
		pos := p.cur.P
		p.next()
		return &Literal{P: pos, Kind: LitNull}
	case tokLparen:
		p.next()
		ex := p.parseExpr()
		p.expect(tokRparen)
		return ex
	case tokLbrack:
		return p.parseList()
	case tokLbrace:
		return p.parseDict()
	}
	p.failCur(fmt.Sprintf("unexpected %s", p.cur.K))
	return nil
}

func (p *parser) parseList() Expr {
	pos := p.expect(tokLbrack).P
	p.skipSemi()
	if p.cur.K == tokRbrack {
		p.next()
		return &ListLit{P: pos}
	}

	var elems []Expr
	for {
		elems = append(elems, p.parseExpr())
		p.skipSemi()
		if p.cur.K == tokComma {
			p.next()
			p.skipSemi()
			if p.cur.K == tokRbrack {
				break
			}
			continue
		}
		if p.cur.K == tokRbrack {
			break
		}
		break
	}
	p.expect(tokRbrack)
	return &ListLit{P: pos, Elems: elems}
}

func (p *parser) parseDict() Expr {
	pos := p.expect(tokLbrace).P
	p.skipSemi()
	if p.cur.K == tokRbrace {
		p.next()
		return &DictLit{P: pos}
	}

	var entries []DictEntry
	for {
		var key string
		kp := p.cur.P
		switch p.cur.K {
		case tokString:
			key = p.cur.Lit
			p.next()
		case tokIdent:
			key = p.cur.Lit
			p.next()
		default:
			if p.cur.K.isKeyword() {
				p.fail(p.cur.P, fmt.Sprintf(
					"dict key cannot be the reserved word %s, quote it as %q", p.cur.K, p.cur.K,
				))
			}
			p.fail(p.cur.P, "dict key must be string or ident")
		}
		p.expect(tokColon)
		val := p.parseExpr()
		entries = append(entries, DictEntry{P: kp, Key: key, Val: val})
		p.skipSemi()
		if p.cur.K == tokComma {
			p.next()
			p.skipSemi()
			if p.cur.K == tokRbrace {
				break
			}
			continue
		}
		if p.cur.K == tokRbrace {
			break
		}
		break
	}
	p.expect(tokRbrace)
	return &DictLit{P: pos, Entries: entries}
}

func (p *parser) skipSemi() {
	for p.isSemi(p.cur.K) {
		p.next()
	}
}

func (p *parser) isSemi(k tokKind) bool {
	return k == tokSemi || k == tokAutoSemi
}

func (p *parser) expectSemi() {
	if !p.isSemi(p.cur.K) {
		p.failCur(fmt.Sprintf("expected %s, got %s", tokSemi, p.cur.K))
	}
	p.next()
}

func (p *parser) next() {
	p.cur = p.peek
	if len(p.ahead) > 0 {
		p.peek = p.ahead[0]
		p.ahead = p.ahead[1:]
		return
	}
	p.peek = p.lx.scan()
}

func (p *parser) peekN(n int) token {
	if n <= 0 {
		return p.cur
	}
	if n == 1 {
		return p.peek
	}
	for len(p.ahead) < n-1 {
		p.ahead = append(p.ahead, p.lx.scan())
	}
	return p.ahead[n-2]
}

func (p *parser) expect(k tokKind) token {
	if p.cur.K != k {
		p.failCur(fmt.Sprintf("expected %s, got %s", k, p.cur.K))
	}
	t := p.cur
	p.next()
	return t
}

func (p *parser) fail(pos Pos, msg string) {
	panic(&ParseError{Pos: pos, Msg: msg})
}

// failCur reports a parse error at the current token. An illegal token already
// carries the lexer's diagnostic, such as unexpected '&'.
func (p *parser) failCur(fallback string) {
	msg := fallback
	if p.cur.K == tokIllegal {
		msg = p.cur.Lit
	}
	p.fail(p.cur.P, msg)
}

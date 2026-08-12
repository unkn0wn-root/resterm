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
// unlabeled statements and pos points at the label that opened the target
type controlTarget struct {
	kind  controlKind
	label string
	pos   Pos
}

// stmtLabel is the optional name in front of a for or switch. The zero value
// means the statement carries no label
type stmtLabel struct {
	name string
	pos  Pos
}

// controls holds every for and switch enclosing the statement being parsed, so
// break and continue can be checked against real targets instead of a depth
type Parser struct {
	lx       *Lexer
	cur      Tok
	peek     Tok
	ahead    []Tok
	controls []controlTarget
}

func NewParserAt(path string, src []byte, pos Pos) *Parser {
	lx := NewLexerAt(path, src, pos)
	p := &Parser{lx: lx}
	p.cur = lx.Next()
	p.peek = lx.Next()
	return p
}

func ParseModule(path string, src []byte) (m *Mod, err error) {
	return ParseModuleAt(path, src, Pos{Line: 1, Col: 1})
}

func ParseModuleAt(path string, src []byte, pos Pos) (m *Mod, err error) {
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
	p := NewParserAt(path, src, pos)
	m = p.parseMod()
	return m, err
}

func ParseExpr(path string, line, col int, src string) (ex Expr, err error) {
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
	p := NewParserAt(path, []byte(src), Pos{Line: line, Col: col})
	ex = p.parseExprOnly()
	return ex, err
}

func (p *Parser) parseMod() *Mod {
	m := &Mod{Path: p.lx.path}
	for p.cur.K != EOF {
		if p.isSemi(p.cur.K) {
			p.next()
			continue
		}
		if p.cur.K == KW_MODULE {
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

func (p *Parser) parseModDecl() (string, Pos) {
	pos := p.expect(KW_MODULE).P
	// the module name becomes the binding when @use omits an alias
	if p.cur.K.isKeyword() {
		p.fail(p.cur.P, fmt.Sprintf("module name cannot be the reserved word %s", p.cur.K))
	}
	if p.cur.K != IDENT {
		p.fail(p.cur.P, "module requires a name")
	}
	nm := p.cur.Lit
	p.next()
	return nm, pos
}

func (p *Parser) parseExprOnly() Expr {
	ex := p.parseExpr()
	for p.isSemi(p.cur.K) {
		p.next()
	}
	if p.cur.K != EOF {
		p.failCur(fmt.Sprintf("unexpected %s", p.cur.K))
	}
	return ex
}

func (p *Parser) parseStmt() Stmt {
	switch p.cur.K {
	case KW_EXPORT:
		return p.parseExport()
	case KW_LET:
		return p.parseLet(false, false)
	case KW_CONST:
		return p.parseLet(false, true)
	case KW_FN:
		return p.parseFn(false)
	case KW_IF:
		return p.parseIf()
	case KW_SWITCH:
		return p.parseSwitch(stmtLabel{})
	case KW_CASE:
		p.fail(p.cur.P, "case outside switch body")
	case KW_DEFAULT:
		p.fail(p.cur.P, "default outside switch body")
	case KW_FOR:
		return p.parseFor(stmtLabel{})
	case KW_BREAK:
		return p.parseBreak()
	case KW_CONTINUE:
		return p.parseContinue()
	case KW_RETURN:
		return p.parseReturn()
	case IDENT:
		if p.peek.K == ASSIGN {
			return p.parseAssign()
		}
		if p.peek.K == COLON {
			return p.parseLabeled()
		}
	}
	return p.parseExprStmt()
}

func (p *Parser) parseExport() Stmt {
	p.expect(KW_EXPORT)
	switch p.cur.K {
	case KW_LET:
		return p.parseLet(true, false)
	case KW_CONST:
		return p.parseLet(true, true)
	case KW_FN:
		return p.parseFn(true)
	default:
		p.fail(p.cur.P, "export must be followed by let, const, or fn")
		return nil
	}
}

func (p *Parser) parseLet(exp bool, isConst bool) Stmt {
	var pos Pos
	if isConst {
		pos = p.expect(KW_CONST).P
	} else {
		pos = p.expect(KW_LET).P
	}

	name := p.expect(IDENT).Lit
	p.expect(ASSIGN)
	val := p.parseExpr()
	return &LetStmt{P: pos, Exported: exp, Const: isConst, Name: name, Val: val}
}

func (p *Parser) parseAssign() Stmt {
	tok := p.expect(IDENT)
	pos := tok.P
	name := tok.Lit
	p.expect(ASSIGN)
	val := p.parseExpr()
	return &AssignStmt{P: pos, Name: name, Val: val}
}

func (p *Parser) parseReturn() Stmt {
	pos := p.expect(KW_RETURN).P
	if p.cur.K == EOF || p.cur.K == RBRACE || p.isSemi(p.cur.K) {
		return &ReturnStmt{P: pos}
	}
	val := p.parseExpr()
	return &ReturnStmt{P: pos, Val: val}
}

func (p *Parser) parseExprStmt() Stmt {
	pos := p.cur.P
	ex := p.parseExpr()
	return &ExprStmt{P: pos, Exp: ex}
}

func (p *Parser) parseFn(exp bool) Stmt {
	pos := p.expect(KW_FN).P
	name := p.expect(IDENT).Lit
	p.expect(LPAREN)
	params := p.parseParams()
	p.expect(RPAREN)
	saved := p.controls
	p.controls = nil
	body := p.parseBlock()
	p.controls = saved
	return &FnDef{P: pos, Exported: exp, Name: name, Params: params, Body: body}
}

func (p *Parser) parseParams() []string {
	if p.cur.K == RPAREN {
		return nil
	}
	var out []string
	seen := map[string]struct{}{}
	for {
		if p.cur.K != IDENT {
			p.fail(p.cur.P, "expected parameter name")
		}

		name := p.cur.Lit
		if _, ok := seen[name]; ok {
			p.fail(p.cur.P, fmt.Sprintf("duplicate parameter %q", name))
		}
		seen[name] = struct{}{}
		out = append(out, name)
		p.next()
		if p.cur.K == COMMA {
			p.next()
			if p.cur.K == RPAREN {
				break
			}
			continue
		}
		break
	}
	return out
}

func (p *Parser) parseIf() Stmt {
	pos := p.expect(KW_IF).P
	cond := p.parseExpr()
	then := p.parseBlock()
	var elifs []Elif
	for p.cur.K == KW_ELIF {
		p.next()
		c := p.parseExpr()
		b := p.parseBlock()
		elifs = append(elifs, Elif{Cond: c, Body: b})
	}

	var els *Block
	if p.cur.K == KW_ELSE {
		p.next()
		els = p.parseBlock()
	}
	return &IfStmt{P: pos, Cond: cond, Then: then, Elifs: elifs, Else: els}
}

// parseLabeled reads "name:" in front of a for or switch. Labels live in their
// own namespace and decorate nothing else, so there is no general labeled
// statement and no goto
func (p *Parser) parseLabeled() Stmt {
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
	case KW_FOR:
		return p.parseFor(lb)
	case KW_SWITCH:
		return p.parseSwitch(lb)
	default:
		p.failCur(fmt.Sprintf("label must be followed by for or switch, got %s", p.cur.K))
		return nil
	}
}

func (p *Parser) pushControl(kind controlKind, lb stmtLabel) {
	p.controls = append(p.controls, controlTarget{kind: kind, label: lb.name, pos: lb.pos})
}

func (p *Parser) popControl() {
	p.controls = p.controls[:len(p.controls)-1]
}

// findControl looks up an active label. It is never called with an empty name,
// which would match the unlabeled targets
func (p *Parser) findControl(label string) *controlTarget {
	for i := len(p.controls) - 1; i >= 0; i-- {
		if p.controls[i].label == label {
			return &p.controls[i]
		}
	}
	return nil
}

func (p *Parser) inLoop() bool {
	for _, c := range p.controls {
		if c.kind == controlLoop {
			return true
		}
	}
	return false
}

// labelRef reads the optional target of a break or continue. The lexer inserts
// a semicolon after both keywords, so a label only parses on the same line
func (p *Parser) labelRef() (Tok, bool) {
	if p.cur.K != IDENT {
		return Tok{}, false
	}
	tok := p.cur
	p.next()
	return tok, true
}

// parseSwitch reads both switch forms. A brace right after the keyword means
// the tagless form, mirroring Go: a dict literal tag has to be parenthesized
func (p *Parser) parseSwitch(lb stmtLabel) Stmt {
	pos := p.expect(KW_SWITCH).P
	var tag Expr
	if p.cur.K != LBRACE {
		tag = p.parseExpr()
	}
	p.expect(LBRACE)
	p.pushControl(controlSwitch, lb)
	clauses := p.parseCaseClauses()
	p.popControl()
	p.expect(RBRACE)
	return &SwitchStmt{P: pos, Label: lb.name, LabelPos: lb.pos, Tag: tag, Clauses: clauses}
}

func (p *Parser) parseCaseClauses() []CaseClause {
	var out []CaseClause
	seenDefault := false
	for {
		p.skipSemi()
		switch p.cur.K {
		case RBRACE:
			return out
		case EOF:
			p.fail(p.cur.P, "unterminated switch")
		case KW_CASE:
			out = append(out, p.parseCase())
		case KW_DEFAULT:
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

func (p *Parser) parseCase() CaseClause {
	pos := p.expect(KW_CASE).P
	exprs := p.parseCaseExprs()
	colon := p.expect(COLON).P
	return CaseClause{P: pos, Exprs: exprs, Body: p.parseClauseBody(colon)}
}

func (p *Parser) parseCaseExprs() []Expr {
	var out []Expr
	for {
		if p.cur.K == COLON {
			p.fail(p.cur.P, "case requires an expression")
		}
		out = append(out, p.parseExpr())
		if p.cur.K != COMMA {
			return out
		}
		p.next()
	}
}

func (p *Parser) parseDefault() CaseClause {
	pos := p.expect(KW_DEFAULT).P
	colon := p.expect(COLON).P
	return CaseClause{P: pos, Body: p.parseClauseBody(colon)}
}

func (p *Parser) parseClauseBody(pos Pos) *Block {
	var out []Stmt
	for {
		p.skipSemi()
		if p.cur.K == RBRACE || p.cur.K == KW_CASE || p.cur.K == KW_DEFAULT {
			return &Block{P: pos, Stmts: out}
		}
		if p.cur.K == EOF {
			p.fail(p.cur.P, "unterminated switch")
		}
		out = append(out, p.parseStmt())
	}
}

func (p *Parser) parseFor(lb stmtLabel) Stmt {
	pos := p.expect(KW_FOR).P
	if p.cur.K == LBRACE {
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

func (p *Parser) isForRangeStart() bool {
	tok := p.cur
	idx := 0
	if tok.K == KW_LET {
		tok = p.peekN(1)
		if tok.K != IDENT {
			return false
		}
		idx = 1
	} else if tok.K != IDENT {
		return false
	}

	next := p.peekN(idx + 1)
	if next.K == COMMA {
		if p.peekN(idx+2).K != IDENT {
			return false
		}
		next = p.peekN(idx + 3)
	}
	return next.K == KW_RANGE
}

func (p *Parser) parseForRange(pos Pos, lb stmtLabel) Stmt {
	decl := false
	if p.cur.K == KW_LET {
		decl = true
		p.next()
	}

	keyTok := p.expect(IDENT)
	key := keyTok.Lit
	val := ""
	if p.cur.K == COMMA {
		p.next()
		valTok := p.expect(IDENT)
		val = valTok.Lit
	}

	if val != "" && key == val && key != "_" {
		p.fail(keyTok.P, "range variables must be distinct")
	}

	p.expect(KW_RANGE)
	src := p.parseExpr()
	rng := &ForRange{Key: key, Val: val, Expr: src, Declare: decl}
	return p.parseForBody(pos, lb, nil, nil, nil, rng)
}

func (p *Parser) parseForCond() Expr {
	if p.isSemi(p.cur.K) {
		return nil
	}
	return p.parseExpr()
}

func (p *Parser) parseForInit() Stmt {
	return p.parseForClause(true, "init")
}

func (p *Parser) parseForPost() Stmt {
	if p.cur.K == LBRACE {
		return nil
	}
	return p.parseForClause(false, "post")
}

func (p *Parser) parseForClause(allowLet bool, label string) Stmt {
	switch p.cur.K {
	case KW_LET:
		if !allowLet {
			p.fail(p.cur.P, fmt.Sprintf("for %s clause cannot use let", label))
		}
		return p.parseLet(false, false)
	case KW_CONST:
		p.fail(p.cur.P, fmt.Sprintf("for %s clause cannot use const", label))
	case KW_EXPORT, KW_FN, KW_IF, KW_SWITCH, KW_FOR, KW_RETURN, KW_BREAK, KW_CONTINUE, KW_RANGE:
		p.fail(p.cur.P, fmt.Sprintf("invalid for %s clause", label))
	case IDENT:
		if p.peek.K == ASSIGN {
			return p.parseAssign()
		}
	}
	return p.parseExprStmt()
}

func (p *Parser) parseForBody(
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

func (p *Parser) parseBreak() Stmt {
	pos := p.expect(KW_BREAK).P
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

func (p *Parser) parseContinue() Stmt {
	pos := p.expect(KW_CONTINUE).P
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

func (p *Parser) parseBlock() *Block {
	pos := p.expect(LBRACE).P
	var out []Stmt
	for p.cur.K != RBRACE {
		if p.cur.K == EOF {
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
	p.expect(RBRACE)
	return &Block{P: pos, Stmts: out}
}

func (p *Parser) parseExpr() Expr {
	return p.parseTernary()
}

type binOpMatcher func(Kind) (BinOp, bool)

func (p *Parser) parseBinary(next func() Expr, opOf binOpMatcher) Expr {
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

func (p *Parser) parseTernary() Expr {
	cond := p.parseCoalesce()
	if p.cur.K != QUESTION {
		return cond
	}

	pos := p.cur.P
	p.next()
	then := p.parseExpr()
	p.expect(COLON)
	els := p.parseExpr()
	return &Ternary{P: pos, Cond: cond, Then: then, Else: els}
}

func (p *Parser) parseCoalesce() Expr {
	return p.parseBinary(p.parseOr, coalesceOp)
}

func (p *Parser) parseOr() Expr {
	return p.parseBinary(p.parseAnd, orOp)
}

func (p *Parser) parseAnd() Expr {
	return p.parseBinary(p.parseEq, andOp)
}

func (p *Parser) parseEq() Expr {
	return p.parseBinary(p.parseCmp, eqOp)
}

func (p *Parser) parseCmp() Expr {
	return p.parseBinary(p.parseAdd, cmpOp)
}

func (p *Parser) parseAdd() Expr {
	return p.parseBinary(p.parseMul, addOp)
}

func (p *Parser) parseMul() Expr {
	return p.parseBinary(p.parseUnary, mulOp)
}

func coalesceOp(k Kind) (BinOp, bool) {
	if k == COALESCE {
		return OpCoalesce, true
	}
	return 0, false
}

func orOp(k Kind) (BinOp, bool) {
	switch k {
	case KW_OR, OROR:
		return OpOr, true
	default:
		return 0, false
	}
}

func andOp(k Kind) (BinOp, bool) {
	switch k {
	case KW_AND, ANDAND:
		return OpAnd, true
	default:
		return 0, false
	}
}

func eqOp(k Kind) (BinOp, bool) {
	switch k {
	case EQ:
		return OpEq, true
	case NE:
		return OpNe, true
	default:
		return 0, false
	}
}

func cmpOp(k Kind) (BinOp, bool) {
	switch k {
	case LT:
		return OpLt, true
	case LE:
		return OpLe, true
	case GT:
		return OpGt, true
	case GE:
		return OpGe, true
	default:
		return 0, false
	}
}

func addOp(k Kind) (BinOp, bool) {
	switch k {
	case PLUS:
		return OpAdd, true
	case MINUS:
		return OpSub, true
	default:
		return 0, false
	}
}

func mulOp(k Kind) (BinOp, bool) {
	switch k {
	case STAR:
		return OpMul, true
	case SLASH:
		return OpDiv, true
	case PERCENT:
		return OpMod, true
	default:
		return 0, false
	}
}

// unaryOp maps the word and symbol forms onto the same operator, so not x and
// !x parse to one node
func unaryOp(k Kind) (UnOp, bool) {
	switch k {
	case KW_NOT, BANG:
		return UnNot, true
	case MINUS:
		return UnNeg, true
	default:
		return 0, false
	}
}

func (p *Parser) parseUnary() Expr {
	if p.cur.K == KW_TRY {
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

func (p *Parser) parsePostfix() Expr {
	left := p.parsePrimary()
	for {
		switch p.cur.K {
		case LPAREN:
			pos := p.cur.P
			p.next()
			args := p.parseArgs()
			p.expect(RPAREN)
			left = &Call{P: pos, Callee: left, Args: args}
		case LBRACK:
			pos := p.cur.P
			p.next()
			idx := p.parseExpr()
			p.expect(RBRACK)
			left = &Index{P: pos, X: left, Idx: idx}
		case DOT:
			pos := p.cur.P
			p.next()
			// member access has no quoted spelling, so the fix here is an index
			if p.cur.K.isKeyword() {
				p.fail(p.cur.P, fmt.Sprintf(
					"field name cannot be the reserved word %s, use [%q] instead", p.cur.K, p.cur.K,
				))
			}
			name := p.expect(IDENT).Lit
			left = &Member{P: pos, X: left, Name: name}
		default:
			return left
		}
	}
}

func (p *Parser) parseArgs() []Expr {
	if p.cur.K == RPAREN {
		return nil
	}

	var out []Expr
	for {
		out = append(out, p.parseExpr())
		if p.cur.K == COMMA {
			p.next()
			if p.cur.K == RPAREN {
				break
			}
			continue
		}
		break
	}
	return out
}

func (p *Parser) parsePrimary() Expr {
	switch p.cur.K {
	case IDENT:
		pos := p.cur.P
		name := p.cur.Lit
		p.next()
		return &Ident{P: pos, Name: name}
	case NUMBER:
		pos := p.cur.P
		lit := p.cur.Lit
		p.next()
		n, err := strconv.ParseFloat(lit, 64)
		if err != nil {
			p.fail(pos, "invalid number")
		}
		return &Literal{P: pos, Kind: LitNum, N: n}
	case STRING:
		pos := p.cur.P
		lit := p.cur.Lit
		p.next()
		return &Literal{P: pos, Kind: LitStr, S: lit}
	case KW_TRUE:
		pos := p.cur.P
		p.next()
		return &Literal{P: pos, Kind: LitBool, B: true}
	case KW_FALSE:
		pos := p.cur.P
		p.next()
		return &Literal{P: pos, Kind: LitBool, B: false}
	case KW_NULL:
		pos := p.cur.P
		p.next()
		return &Literal{P: pos, Kind: LitNull}
	case LPAREN:
		p.next()
		ex := p.parseExpr()
		p.expect(RPAREN)
		return ex
	case LBRACK:
		return p.parseList()
	case LBRACE:
		return p.parseDict()
	}
	p.failCur(fmt.Sprintf("unexpected %s", p.cur.K))
	return nil
}

func (p *Parser) parseList() Expr {
	pos := p.expect(LBRACK).P
	p.skipSemi()
	if p.cur.K == RBRACK {
		p.next()
		return &ListLit{P: pos}
	}

	var elems []Expr
	for {
		elems = append(elems, p.parseExpr())
		p.skipSemi()
		if p.cur.K == COMMA {
			p.next()
			p.skipSemi()
			if p.cur.K == RBRACK {
				break
			}
			continue
		}
		if p.cur.K == RBRACK {
			break
		}
		break
	}
	p.expect(RBRACK)
	return &ListLit{P: pos, Elems: elems}
}

func (p *Parser) parseDict() Expr {
	pos := p.expect(LBRACE).P
	p.skipSemi()
	if p.cur.K == RBRACE {
		p.next()
		return &DictLit{P: pos}
	}

	var entries []DictEntry
	for {
		var key string
		kp := p.cur.P
		switch p.cur.K {
		case STRING:
			key = p.cur.Lit
			p.next()
		case IDENT:
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
		p.expect(COLON)
		val := p.parseExpr()
		entries = append(entries, DictEntry{P: kp, Key: key, Val: val})
		p.skipSemi()
		if p.cur.K == COMMA {
			p.next()
			p.skipSemi()
			if p.cur.K == RBRACE {
				break
			}
			continue
		}
		if p.cur.K == RBRACE {
			break
		}
		break
	}
	p.expect(RBRACE)
	return &DictLit{P: pos, Entries: entries}
}

func (p *Parser) skipSemi() {
	for p.isSemi(p.cur.K) {
		p.next()
	}
}

func (p *Parser) isSemi(k Kind) bool {
	return k == SEMI || k == AUTO_SEMI
}

func (p *Parser) expectSemi() {
	if !p.isSemi(p.cur.K) {
		p.failCur(fmt.Sprintf("expected %s, got %s", SEMI, p.cur.K))
	}
	p.next()
}

func (p *Parser) next() {
	p.cur = p.peek
	if len(p.ahead) > 0 {
		p.peek = p.ahead[0]
		p.ahead = p.ahead[1:]
		return
	}
	p.peek = p.lx.Next()
}

func (p *Parser) peekN(n int) Tok {
	if n <= 0 {
		return p.cur
	}
	if n == 1 {
		return p.peek
	}
	for len(p.ahead) < n-1 {
		p.ahead = append(p.ahead, p.lx.Next())
	}
	return p.ahead[n-2]
}

func (p *Parser) expect(k Kind) Tok {
	if p.cur.K != k {
		p.failCur(fmt.Sprintf("expected %s, got %s", k, p.cur.K))
	}
	t := p.cur
	p.next()
	return t
}

func (p *Parser) fail(pos Pos, msg string) {
	panic(&ParseError{Pos: pos, Msg: msg})
}

// failCur reports a parse error at the current token. An ILLEGAL token already
// carries the lexer's diagnostic (e.g. unexpected '&')
func (p *Parser) failCur(fallback string) {
	msg := fallback
	if p.cur.K == ILLEGAL {
		msg = p.cur.Lit
	}
	p.fail(p.cur.P, msg)
}

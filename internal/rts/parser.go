package rts

import (
	"fmt"
	"strconv"
)

type Parser struct {
	lx        *Lexer
	cur       Tok
	peek      Tok
	ahead     []Tok
	loopDepth int
}

func NewParser(path string, src []byte) *Parser {
	return NewParserAt(path, src, Pos{Line: 1, Col: 1})
}

func NewParserAt(path string, src []byte, pos Pos) *Parser {
	lx := NewLexerAt(path, src, pos)
	p := &Parser{lx: lx}
	p.cur = lx.Next()
	p.peek = lx.Next()
	return p
}

func ParseModule(path string, src []byte) (m *Mod, err error) {
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
	p := NewParser(path, src)
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
		st := p.parseStmt()
		m.Stmts = append(m.Stmts, st)
		p.skipSemi()
	}
	return m
}

func (p *Parser) parseExprOnly() Expr {
	ex := p.parseExpr()
	for p.isSemi(p.cur.K) {
		p.next()
	}
	if p.cur.K != EOF {
		p.fail(p.cur.P, fmt.Sprintf("unexpected %s", p.cur.K))
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
	case KW_FOR:
		return p.parseFor()
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
	savedDepth := p.loopDepth
	p.loopDepth = 0
	body := p.parseBlock()
	p.loopDepth = savedDepth
	return &FnDef{P: pos, Exported: exp, Name: name, Params: params, Body: body}
}

func (p *Parser) parseParams() []string {
	if p.cur.K == RPAREN {
		return nil
	}
	var out []string
	for {
		if p.cur.K != IDENT {
			p.fail(p.cur.P, "expected parameter name")
		}

		out = append(out, p.cur.Lit)
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

func (p *Parser) parseFor() Stmt {
	pos := p.expect(KW_FOR).P
	if p.cur.K == LBRACE {
		return p.parseForBody(pos, nil, nil, nil, nil)
	}
	if p.isForRangeStart() {
		return p.parseForRange(pos)
	}

	if p.isSemi(p.cur.K) {
		p.next()
		cond := p.parseForCond()
		p.expectSemi()
		post := p.parseForPost()
		return p.parseForBody(pos, nil, cond, post, nil)
	}

	init := p.parseForInit()
	if p.isSemi(p.cur.K) {
		p.next()
		cond := p.parseForCond()
		p.expectSemi()
		post := p.parseForPost()
		return p.parseForBody(pos, init, cond, post, nil)
	}

	exprStmt, ok := init.(*ExprStmt)
	if !ok {
		p.fail(init.Pos(), "for condition must be expression")
	}
	return p.parseForBody(pos, nil, exprStmt.Exp, nil, nil)
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

func (p *Parser) parseForRange(pos Pos) Stmt {
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
	return p.parseForBody(pos, nil, nil, nil, rng)
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
	case KW_EXPORT, KW_FN, KW_IF, KW_FOR, KW_RETURN, KW_BREAK, KW_CONTINUE, KW_RANGE:
		p.fail(p.cur.P, fmt.Sprintf("invalid for %s clause", label))
	case IDENT:
		if p.peek.K == ASSIGN {
			return p.parseAssign()
		}
	}
	return p.parseExprStmt()
}

func (p *Parser) parseForBody(pos Pos, init Stmt, cond Expr, post Stmt, rng *ForRange) Stmt {
	p.loopDepth++
	body := p.parseBlock()
	p.loopDepth--
	return &ForStmt{P: pos, Init: init, Cond: cond, Post: post, Range: rng, Body: body}
}

func (p *Parser) parseBreak() Stmt {
	pos := p.expect(KW_BREAK).P
	if p.loopDepth == 0 {
		p.fail(pos, "break outside loop")
	}
	return &BreakStmt{P: pos}
}

func (p *Parser) parseContinue() Stmt {
	pos := p.expect(KW_CONTINUE).P
	if p.loopDepth == 0 {
		p.fail(pos, "continue outside loop")
	}
	return &ContinueStmt{P: pos}
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
	left := p.parseOr()
	for p.cur.K == COALESCE {
		pos := p.cur.P
		p.next()
		right := p.parseOr()
		left = &Binary{P: pos, Op: OpCoalesce, Left: left, Right: right}
	}
	return left
}

func (p *Parser) parseOr() Expr {
	left := p.parseAnd()
	for p.cur.K == KW_OR {
		pos := p.cur.P
		p.next()
		right := p.parseAnd()
		left = &Binary{P: pos, Op: OpOr, Left: left, Right: right}
	}
	return left
}

func (p *Parser) parseAnd() Expr {
	left := p.parseEq()
	for p.cur.K == KW_AND {
		pos := p.cur.P
		p.next()
		right := p.parseEq()
		left = &Binary{P: pos, Op: OpAnd, Left: left, Right: right}
	}
	return left
}

func (p *Parser) parseEq() Expr {
	left := p.parseCmp()
	for p.cur.K == EQ || p.cur.K == NE {
		pos := p.cur.P
		op := p.cur.K
		p.next()
		right := p.parseCmp()
		if op == EQ {
			left = &Binary{P: pos, Op: OpEq, Left: left, Right: right}
		} else {
			left = &Binary{P: pos, Op: OpNe, Left: left, Right: right}
		}
	}
	return left
}

func (p *Parser) parseCmp() Expr {
	left := p.parseAdd()
	for p.cur.K == LT || p.cur.K == LE || p.cur.K == GT || p.cur.K == GE {
		pos := p.cur.P
		op := p.cur.K
		p.next()
		right := p.parseAdd()
		switch op {
		case LT:
			left = &Binary{P: pos, Op: OpLt, Left: left, Right: right}
		case LE:
			left = &Binary{P: pos, Op: OpLe, Left: left, Right: right}
		case GT:
			left = &Binary{P: pos, Op: OpGt, Left: left, Right: right}
		case GE:
			left = &Binary{P: pos, Op: OpGe, Left: left, Right: right}
		}
	}
	return left
}

func (p *Parser) parseAdd() Expr {
	left := p.parseMul()
	for p.cur.K == PLUS || p.cur.K == MINUS {
		pos := p.cur.P
		op := p.cur.K
		p.next()
		right := p.parseMul()
		if op == PLUS {
			left = &Binary{P: pos, Op: OpAdd, Left: left, Right: right}
		} else {
			left = &Binary{P: pos, Op: OpSub, Left: left, Right: right}
		}
	}
	return left
}

func (p *Parser) parseMul() Expr {
	left := p.parseUnary()
	for p.cur.K == STAR || p.cur.K == SLASH || p.cur.K == PERCENT {
		pos := p.cur.P
		op := p.cur.K
		p.next()
		right := p.parseUnary()
		switch op {
		case STAR:
			left = &Binary{P: pos, Op: OpMul, Left: left, Right: right}
		case SLASH:
			left = &Binary{P: pos, Op: OpDiv, Left: left, Right: right}
		case PERCENT:
			left = &Binary{P: pos, Op: OpMod, Left: left, Right: right}
		}
	}
	return left
}

func (p *Parser) parseUnary() Expr {
	if p.cur.K == KW_TRY {
		pos := p.cur.P
		p.next()
		x := p.parseUnary()
		return &TryExpr{P: pos, X: x}
	}

	if p.cur.K == KW_NOT {
		pos := p.cur.P
		p.next()
		x := p.parseUnary()
		return &Unary{P: pos, Op: UnNot, X: x}
	}

	if p.cur.K == MINUS {
		pos := p.cur.P
		p.next()
		x := p.parseUnary()
		return &Unary{P: pos, Op: UnNeg, X: x}
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
	case ILLEGAL:
		p.fail(p.cur.P, p.cur.Lit)
	}
	p.fail(p.cur.P, fmt.Sprintf("unexpected %s", p.cur.K))
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
		p.fail(p.cur.P, fmt.Sprintf("expected %s, got %s", SEMI, p.cur.K))
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
		p.fail(p.cur.P, fmt.Sprintf("expected %s, got %s", k, p.cur.K))
	}
	t := p.cur
	p.next()
	return t
}

func (p *Parser) fail(pos Pos, msg string) {
	panic(&ParseError{Pos: pos, Msg: msg})
}

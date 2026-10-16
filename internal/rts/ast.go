package rts

// Mod is the syntax tree of one module or script block.
type Mod struct {
	Start   Pos    // position of the first byte of the text
	Name    string // declared module name, or ""
	NamePos Pos    // position of the module keyword, or the zero Pos
	Stmts   []Stmt // top-level statements
}

// Stmt is implemented by every statement node.
type Stmt interface {
	stmtNode()
	Pos() Pos // position of the keyword after any export or label, or of the first token
}

// Expr is implemented by every expression node.
type Expr interface {
	exprNode()
	Pos() Pos // position of the operator or opening bracket, or of the first token
}

// A LetStmt is a let or const declaration. Exported is set when it follows export.
type LetStmt struct {
	P        Pos
	Exported bool
	Const    bool
	Name     string
	Val      Expr
}

func (*LetStmt) stmtNode()  {}
func (s *LetStmt) Pos() Pos { return s.P }

// An AssignStmt is an assignment of Val to Name.
type AssignStmt struct {
	P    Pos
	Name string
	Val  Expr
}

func (*AssignStmt) stmtNode()  {}
func (s *AssignStmt) Pos() Pos { return s.P }

// A ReturnStmt is a return statement. Val is nil for a bare return.
type ReturnStmt struct {
	P   Pos
	Val Expr
}

func (*ReturnStmt) stmtNode()  {}
func (s *ReturnStmt) Pos() Pos { return s.P }

// An ExprStmt is an expression used as a statement.
type ExprStmt struct {
	P   Pos
	Exp Expr
}

func (*ExprStmt) stmtNode()  {}
func (s *ExprStmt) Pos() Pos { return s.P }

// A Block is a list of statements. P is the position of "{", or of ":" in a case clause.
type Block struct {
	P     Pos
	Stmts []Stmt
}

// A FnDef is a function declaration. Exported is set when it follows export.
type FnDef struct {
	P        Pos
	Exported bool
	Name     string
	Params   []string
	Body     *Block
}

func (*FnDef) stmtNode()  {}
func (s *FnDef) Pos() Pos { return s.P }

// An Elif is one elif clause of an IfStmt.
type Elif struct {
	Cond Expr
	Body *Block
}

// An IfStmt is an if statement. Else is nil when there is no else.
type IfStmt struct {
	P     Pos
	Cond  Expr
	Then  *Block
	Elifs []Elif
	Else  *Block
}

func (*IfStmt) stmtNode()  {}
func (s *IfStmt) Pos() Pos { return s.P }

// A CaseClause is a case or default clause. Exprs is nil for default.
type CaseClause struct {
	P     Pos
	Exprs []Expr
	Body  *Block
}

// A SwitchStmt is a switch statement. Tag is nil for the tagless form.
type SwitchStmt struct {
	P        Pos
	Label    string
	LabelPos Pos
	Tag      Expr
	Clauses  []CaseClause
}

func (*SwitchStmt) stmtNode()  {}
func (s *SwitchStmt) Pos() Pos { return s.P }

// A ForStmt is a for statement. The range form sets Range and leaves Init, Cond and Post nil.
type ForStmt struct {
	P        Pos
	Label    string
	LabelPos Pos
	Init     Stmt
	Cond     Expr
	Post     Stmt
	Range    *ForRange
	Body     *Block
}

func (*ForStmt) stmtNode()  {}
func (s *ForStmt) Pos() Pos { return s.P }

// A ForRange is the key, optional value and source of a range loop. Declare is set by let.
type ForRange struct {
	Key     string
	Val     string
	Expr    Expr
	Declare bool
}

// A BreakStmt is a break statement. An empty Label targets the nearest loop or switch.
type BreakStmt struct {
	P     Pos
	Label string
}

func (*BreakStmt) stmtNode()  {}
func (s *BreakStmt) Pos() Pos { return s.P }

// A ContinueStmt is a continue statement. An empty Label targets the nearest loop.
type ContinueStmt struct {
	P     Pos
	Label string
}

func (*ContinueStmt) stmtNode()  {}
func (s *ContinueStmt) Pos() Pos { return s.P }

// An Ident is an identifier.
type Ident struct {
	P    Pos
	Name string
}

func (*Ident) exprNode()  {}
func (e *Ident) Pos() Pos { return e.P }

// A LitKind is the kind of a Literal.
type LitKind int

const (
	LitNull LitKind = iota
	LitBool
	LitNum
	LitStr
)

// A Literal is a null, bool, number or string. Kind says which, and B, N or S holds the value unless it is null.
type Literal struct {
	P    Pos
	Kind LitKind
	B    bool
	N    float64
	S    string
}

func (*Literal) exprNode()  {}
func (e *Literal) Pos() Pos { return e.P }

// A BinOp is the operator of a Binary.
type BinOp int

const (
	OpAdd      BinOp = iota // +
	OpSub                   // -
	OpMul                   // *
	OpDiv                   // /
	OpMod                   // %
	OpEq                    // ==
	OpNe                    // !=
	OpLt                    // <
	OpLe                    // <=
	OpGt                    // >
	OpGe                    // >=
	OpIn                    // in
	OpNotIn                 // not in
	OpAnd                   // and, &&
	OpOr                    // or, ||
	OpCoalesce              // ??
)

// An UnOp is the operator of a Unary.
type UnOp int

const (
	UnNot UnOp = iota // not, !
	UnNeg             // -
)

// A Unary is a unary operation.
type Unary struct {
	P  Pos
	Op UnOp
	X  Expr
}

func (*Unary) exprNode()  {}
func (e *Unary) Pos() Pos { return e.P }

// A Binary is a binary operation. P is the position of the operator.
type Binary struct {
	P     Pos
	Op    BinOp
	Left  Expr
	Right Expr
}

func (*Binary) exprNode()  {}
func (e *Binary) Pos() Pos { return e.P }

// A Ternary is a conditional expression. P is the position of "?".
type Ternary struct {
	P    Pos
	Cond Expr
	Then Expr
	Else Expr
}

func (*Ternary) exprNode()  {}
func (e *Ternary) Pos() Pos { return e.P }

// A TryExpr is a try expression.
type TryExpr struct {
	P Pos
	X Expr
}

func (*TryExpr) exprNode()  {}
func (e *TryExpr) Pos() Pos { return e.P }

// A Call is a function call. P is the position of "(".
type Call struct {
	P      Pos
	Callee Expr
	Args   []Expr
}

func (*Call) exprNode()  {}
func (e *Call) Pos() Pos { return e.P }

// An Index is an index expression. P is the position of "[".
type Index struct {
	P   Pos
	X   Expr
	Idx Expr
}

func (*Index) exprNode()  {}
func (e *Index) Pos() Pos { return e.P }

// A Member is a member access. P is the position of ".".
type Member struct {
	P    Pos
	X    Expr
	Name string
}

func (*Member) exprNode()  {}
func (e *Member) Pos() Pos { return e.P }

// A ListLit is a list literal.
type ListLit struct {
	P     Pos
	Elems []Expr
}

func (*ListLit) exprNode()  {}
func (e *ListLit) Pos() Pos { return e.P }

// A DictEntry is one key and value of a DictLit. P is the position of the key.
type DictEntry struct {
	P   Pos
	Key string
	Val Expr
}

// A DictLit is a dict literal.
type DictLit struct {
	P       Pos
	Entries []DictEntry
}

func (*DictLit) exprNode()  {}
func (e *DictLit) Pos() Pos { return e.P }

// Inspect walks ex depth first and calls f for each expression. If f returns
// false, the children of that expression are skipped.
func Inspect(ex Expr, f func(Expr) bool) {
	if ex == nil || !f(ex) {
		return
	}
	switch e := ex.(type) {
	case *Unary:
		Inspect(e.X, f)
	case *Binary:
		Inspect(e.Left, f)
		Inspect(e.Right, f)
	case *Ternary:
		Inspect(e.Cond, f)
		Inspect(e.Then, f)
		Inspect(e.Else, f)
	case *TryExpr:
		Inspect(e.X, f)
	case *Call:
		Inspect(e.Callee, f)
		for _, arg := range e.Args {
			Inspect(arg, f)
		}
	case *Index:
		Inspect(e.X, f)
		Inspect(e.Idx, f)
	case *Member:
		Inspect(e.X, f)
	case *ListLit:
		for _, el := range e.Elems {
			Inspect(el, f)
		}
	case *DictLit:
		for _, en := range e.Entries {
			Inspect(en.Val, f)
		}
	}
}

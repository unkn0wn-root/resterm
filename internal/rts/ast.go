package rts

type Mod struct {
	Path    string
	Name    string
	NamePos Pos
	Stmts   []Stmt
}

type Stmt interface {
	stmtNode()
	Pos() Pos
}

type Expr interface {
	exprNode()
	Pos() Pos
}

type LetStmt struct {
	P        Pos
	Exported bool
	Const    bool
	Name     string
	Val      Expr
}

func (*LetStmt) stmtNode()  {}
func (s *LetStmt) Pos() Pos { return s.P }

type AssignStmt struct {
	P    Pos
	Name string
	Val  Expr
}

func (*AssignStmt) stmtNode()  {}
func (s *AssignStmt) Pos() Pos { return s.P }

type ReturnStmt struct {
	P   Pos
	Val Expr
}

func (*ReturnStmt) stmtNode()  {}
func (s *ReturnStmt) Pos() Pos { return s.P }

type ExprStmt struct {
	P   Pos
	Exp Expr
}

func (*ExprStmt) stmtNode()  {}
func (s *ExprStmt) Pos() Pos { return s.P }

type Block struct {
	P     Pos
	Stmts []Stmt
}

type FnDef struct {
	P        Pos
	Exported bool
	Name     string
	Params   []string
	Body     *Block
}

func (*FnDef) stmtNode()  {}
func (s *FnDef) Pos() Pos { return s.P }

type Elif struct {
	Cond Expr
	Body *Block
}

type IfStmt struct {
	P     Pos
	Cond  Expr
	Then  *Block
	Elifs []Elif
	Else  *Block
}

func (*IfStmt) stmtNode()  {}
func (s *IfStmt) Pos() Pos { return s.P }

// CaseClause is one clause of a switch. Exprs is nil for the default clause
// and holds at least one expression otherwise
type CaseClause struct {
	P     Pos
	Exprs []Expr
	Body  *Block
}

// SwitchStmt keeps its clauses in source order because evaluation stops at the
// first match. Tag is nil for the tagless form, which tests clauses for truth.
// Label is the optional name a break can target and is empty when unlabeled
type SwitchStmt struct {
	P        Pos
	Label    string
	LabelPos Pos
	Tag      Expr
	Clauses  []CaseClause
}

func (*SwitchStmt) stmtNode()  {}
func (s *SwitchStmt) Pos() Pos { return s.P }

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

type ForRange struct {
	Key     string
	Val     string
	Expr    Expr
	Declare bool
}

// Label names the enclosing for or switch to leave. It is empty for the plain
// form, which targets the nearest one
type BreakStmt struct {
	P     Pos
	Label string
}

func (*BreakStmt) stmtNode()  {}
func (s *BreakStmt) Pos() Pos { return s.P }

// Label names the enclosing loop to resume. It is empty for the plain form,
// which targets the nearest one
type ContinueStmt struct {
	P     Pos
	Label string
}

func (*ContinueStmt) stmtNode()  {}
func (s *ContinueStmt) Pos() Pos { return s.P }

type Ident struct {
	P    Pos
	Name string
}

func (*Ident) exprNode()  {}
func (e *Ident) Pos() Pos { return e.P }

type LitKind int

const (
	LitNull LitKind = iota
	LitBool
	LitNum
	LitStr
)

type Literal struct {
	P    Pos
	Kind LitKind
	B    bool
	N    float64
	S    string
}

func (*Literal) exprNode()  {}
func (e *Literal) Pos() Pos { return e.P }

type BinOp int

const (
	OpAdd BinOp = iota
	OpSub
	OpMul
	OpDiv
	OpMod
	OpEq
	OpNe
	OpLt
	OpLe
	OpGt
	OpGe
	OpIn
	OpNotIn
	OpAnd
	OpOr
	OpCoalesce
)

type UnOp int

const (
	UnNot UnOp = iota
	UnNeg
)

type Unary struct {
	P  Pos
	Op UnOp
	X  Expr
}

func (*Unary) exprNode()  {}
func (e *Unary) Pos() Pos { return e.P }

type Binary struct {
	P     Pos
	Op    BinOp
	Left  Expr
	Right Expr
}

func (*Binary) exprNode()  {}
func (e *Binary) Pos() Pos { return e.P }

type Ternary struct {
	P    Pos
	Cond Expr
	Then Expr
	Else Expr
}

func (*Ternary) exprNode()  {}
func (e *Ternary) Pos() Pos { return e.P }

type TryExpr struct {
	P Pos
	X Expr
}

func (*TryExpr) exprNode()  {}
func (e *TryExpr) Pos() Pos { return e.P }

type Call struct {
	P      Pos
	Callee Expr
	Args   []Expr
}

func (*Call) exprNode()  {}
func (e *Call) Pos() Pos { return e.P }

type Index struct {
	P   Pos
	X   Expr
	Idx Expr
}

func (*Index) exprNode()  {}
func (e *Index) Pos() Pos { return e.P }

type Member struct {
	P    Pos
	X    Expr
	Name string
}

func (*Member) exprNode()  {}
func (e *Member) Pos() Pos { return e.P }

type ListLit struct {
	P     Pos
	Elems []Expr
}

func (*ListLit) exprNode()  {}
func (e *ListLit) Pos() Pos { return e.P }

type DictEntry struct {
	P   Pos
	Key string
	Val Expr
}

type DictLit struct {
	P       Pos
	Entries []DictEntry
}

func (*DictLit) exprNode()  {}
func (e *DictLit) Pos() Pos { return e.P }

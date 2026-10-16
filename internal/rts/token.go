package rts

// tokKind is the set of lexical tokens of RTS.
type tokKind int

const (
	tokEOF tokKind = iota
	tokIllegal

	tokIdent
	tokNumber
	tokString

	tokExport
	tokModule
	tokFn
	tokLet
	tokConst
	tokIf
	tokElif
	tokElse
	tokSwitch
	tokCase
	tokDefault
	tokReturn
	tokFor
	tokBreak
	tokContinue
	tokRange
	tokTry
	tokTrue
	tokFalse
	tokNull
	tokAnd
	tokOr
	tokNot

	tokAssign
	tokEq
	tokNe
	tokLt
	tokLe
	tokGt
	tokGe
	tokPlus
	tokMinus
	tokStar
	tokSlash
	tokPercent
	// symbol forms of not, and, or
	tokBang
	tokAndAnd
	tokOrOr
	tokCoalesce
	tokQuestion
	tokColon

	tokLparen
	tokRparen
	tokLbrack
	tokRbrack
	tokLbrace
	tokRbrace
	tokComma
	tokDot
	tokSemi
	tokAutoSemi
)

// A KeywordClass is a group of reserved words.
type KeywordClass int

const (
	KeywordNone    KeywordClass = iota // not a reserved word
	KeywordDefault                     // a reserved word in no other class
	KeywordDecl                        // export, module, fn, let, const
	KeywordControl                     // control flow, such as if, for and return
	KeywordLiteral                     // true, false, null
	KeywordLogical                     // and, or, not
)

// A token is a kind, literal and position from the lexer. For an illegal token, Lit is the error message.
type token struct {
	K   tokKind
	Lit string
	P   Pos
}

func (k tokKind) String() string {
	switch k {
	case tokEOF:
		return "EOF"
	case tokIllegal:
		return "ILLEGAL"
	case tokIdent:
		return "IDENT"
	case tokNumber:
		return "NUMBER"
	case tokString:
		return "STRING"
	case tokExport:
		return "export"
	case tokModule:
		return "module"
	case tokFn:
		return "fn"
	case tokLet:
		return "let"
	case tokConst:
		return "const"
	case tokIf:
		return "if"
	case tokElif:
		return "elif"
	case tokElse:
		return "else"
	case tokSwitch:
		return "switch"
	case tokCase:
		return "case"
	case tokDefault:
		return "default"
	case tokReturn:
		return "return"
	case tokFor:
		return "for"
	case tokBreak:
		return "break"
	case tokContinue:
		return "continue"
	case tokRange:
		return "range"
	case tokTry:
		return "try"
	case tokTrue:
		return "true"
	case tokFalse:
		return "false"
	case tokNull:
		return "null"
	case tokAnd:
		return "and"
	case tokOr:
		return "or"
	case tokNot:
		return "not"
	case tokAssign:
		return "="
	case tokEq:
		return "=="
	case tokNe:
		return "!="
	case tokLt:
		return "<"
	case tokLe:
		return "<="
	case tokGt:
		return ">"
	case tokGe:
		return ">="
	case tokPlus:
		return "+"
	case tokMinus:
		return "-"
	case tokStar:
		return "*"
	case tokSlash:
		return "/"
	case tokPercent:
		return "%"
	case tokBang:
		return "!"
	case tokAndAnd:
		return "&&"
	case tokOrOr:
		return "||"
	case tokCoalesce:
		return "??"
	case tokQuestion:
		return "?"
	case tokColon:
		return ":"
	case tokLparen:
		return "("
	case tokRparen:
		return ")"
	case tokLbrack:
		return "["
	case tokRbrack:
		return "]"
	case tokLbrace:
		return "{"
	case tokRbrace:
		return "}"
	case tokComma:
		return ","
	case tokDot:
		return "."
	case tokSemi:
		return ";"
	case tokAutoSemi:
		return "<auto-semi>"
	default:
		return "?"
	}
}

// isKeyword reports whether k is a reserved word. The keyword kinds are
// contiguous in the enum, so a new one has to be declared between tokExport and
// tokNot to be recognized here.
func (k tokKind) isKeyword() bool {
	return k >= tokExport && k <= tokNot
}

// IsKeyword reports whether name is reserved. Hosts that turn user input into an
// RTS binding check this first, because a reserved name passes an ordinary
// identifier test but lexes as a keyword, so no code could refer to the binding.
func IsKeyword(name string) bool {
	_, ok := kw[name]
	return ok
}

var kw = map[string]tokKind{
	"export":   tokExport,
	"module":   tokModule,
	"fn":       tokFn,
	"let":      tokLet,
	"const":    tokConst,
	"if":       tokIf,
	"elif":     tokElif,
	"else":     tokElse,
	"switch":   tokSwitch,
	"case":     tokCase,
	"default":  tokDefault,
	"return":   tokReturn,
	"for":      tokFor,
	"break":    tokBreak,
	"continue": tokContinue,
	"range":    tokRange,
	"try":      tokTry,
	"true":     tokTrue,
	"false":    tokFalse,
	"null":     tokNull,
	"and":      tokAnd,
	"or":       tokOr,
	"not":      tokNot,
}

// KeywordClassOf returns the class of name, or KeywordNone when name is not reserved.
func KeywordClassOf(name string) KeywordClass {
	k, ok := kw[name]
	if !ok {
		return KeywordNone
	}
	return keywordClassForKind(k)
}

func keywordClassForKind(k tokKind) KeywordClass {
	switch k {
	case tokExport, tokModule, tokFn, tokLet, tokConst:
		return KeywordDecl
	case tokIf,
		tokElif,
		tokElse,
		tokSwitch,
		tokCase,
		tokDefault,
		tokReturn,
		tokFor,
		tokBreak,
		tokContinue,
		tokRange,
		tokTry:
		return KeywordControl
	case tokTrue, tokFalse, tokNull:
		return KeywordLiteral
	case tokAnd, tokOr, tokNot:
		return KeywordLogical
	default:
		return KeywordDefault
	}
}

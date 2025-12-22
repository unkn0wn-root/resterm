package rts

type Kind int

const (
	EOF Kind = iota
	ILLEGAL

	IDENT
	NUMBER
	STRING

	KW_EXPORT
	KW_FN
	KW_LET
	KW_CONST
	KW_IF
	KW_ELIF
	KW_ELSE
	KW_RETURN
	KW_FOR
	KW_BREAK
	KW_CONTINUE
	KW_RANGE
	KW_TRY
	KW_TRUE
	KW_FALSE
	KW_NULL
	KW_AND
	KW_OR
	KW_NOT

	ASSIGN
	EQ
	NE
	LT
	LE
	GT
	GE
	PLUS
	MINUS
	STAR
	SLASH
	PERCENT
	COALESCE
	QUESTION
	COLON

	LPAREN
	RPAREN
	LBRACK
	RBRACK
	LBRACE
	RBRACE
	COMMA
	DOT
	SEMI
	AUTO_SEMI
)

type KeywordClass int

const (
	KeywordNone KeywordClass = iota
	KeywordDefault
	KeywordDecl
	KeywordControl
	KeywordLiteral
	KeywordLogical
)

type Tok struct {
	K   Kind
	Lit string
	P   Pos
}

func (k Kind) String() string {
	switch k {
	case EOF:
		return "EOF"
	case ILLEGAL:
		return "ILLEGAL"
	case IDENT:
		return "IDENT"
	case NUMBER:
		return "NUMBER"
	case STRING:
		return "STRING"
	case KW_EXPORT:
		return "export"
	case KW_FN:
		return "fn"
	case KW_LET:
		return "let"
	case KW_CONST:
		return "const"
	case KW_IF:
		return "if"
	case KW_ELIF:
		return "elif"
	case KW_ELSE:
		return "else"
	case KW_RETURN:
		return "return"
	case KW_FOR:
		return "for"
	case KW_BREAK:
		return "break"
	case KW_CONTINUE:
		return "continue"
	case KW_RANGE:
		return "range"
	case KW_TRY:
		return "try"
	case KW_TRUE:
		return "true"
	case KW_FALSE:
		return "false"
	case KW_NULL:
		return "null"
	case KW_AND:
		return "and"
	case KW_OR:
		return "or"
	case KW_NOT:
		return "not"
	case ASSIGN:
		return "="
	case EQ:
		return "=="
	case NE:
		return "!="
	case LT:
		return "<"
	case LE:
		return "<="
	case GT:
		return ">"
	case GE:
		return ">="
	case PLUS:
		return "+"
	case MINUS:
		return "-"
	case STAR:
		return "*"
	case SLASH:
		return "/"
	case PERCENT:
		return "%"
	case COALESCE:
		return "??"
	case QUESTION:
		return "?"
	case COLON:
		return ":"
	case LPAREN:
		return "("
	case RPAREN:
		return ")"
	case LBRACK:
		return "["
	case RBRACK:
		return "]"
	case LBRACE:
		return "{"
	case RBRACE:
		return "}"
	case COMMA:
		return ","
	case DOT:
		return "."
	case SEMI:
		return ";"
	case AUTO_SEMI:
		return "<auto-semi>"
	default:
		return "?"
	}
}

var kw = map[string]Kind{
	"export":   KW_EXPORT,
	"fn":       KW_FN,
	"let":      KW_LET,
	"const":    KW_CONST,
	"if":       KW_IF,
	"elif":     KW_ELIF,
	"else":     KW_ELSE,
	"return":   KW_RETURN,
	"for":      KW_FOR,
	"break":    KW_BREAK,
	"continue": KW_CONTINUE,
	"range":    KW_RANGE,
	"try":      KW_TRY,
	"true":     KW_TRUE,
	"false":    KW_FALSE,
	"null":     KW_NULL,
	"and":      KW_AND,
	"or":       KW_OR,
	"not":      KW_NOT,
}

// IsKeyword reports whether name is a RestermScript keyword.
func IsKeyword(name string) bool {
	_, ok := kw[name]
	return ok
}

// KeywordClassOf returns the keyword class for name, or KeywordNone.
func KeywordClassOf(name string) KeywordClass {
	k, ok := kw[name]
	if !ok {
		return KeywordNone
	}
	return keywordClassForKind(k)
}

func keywordClassForKind(k Kind) KeywordClass {
	switch k {
	case KW_EXPORT, KW_FN, KW_LET, KW_CONST:
		return KeywordDecl
	case KW_IF, KW_ELIF, KW_ELSE, KW_RETURN, KW_FOR, KW_BREAK, KW_CONTINUE, KW_RANGE, KW_TRY:
		return KeywordControl
	case KW_TRUE, KW_FALSE, KW_NULL:
		return KeywordLiteral
	case KW_AND, KW_OR, KW_NOT:
		return KeywordLogical
	default:
		return KeywordDefault
	}
}

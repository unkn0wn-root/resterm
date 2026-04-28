package workspace

import (
	"github.com/unkn0wn-root/resterm/internal/rts"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

const (
	jsonFileMember = "file"

	jsonNamespace   = "json"
	rtsNamespace    = "rts"
	stdlibNamespace = "stdlib"
)

func jsonFileExprs(path string, line, col int, src string) []string {
	src = str.Trim(src)
	if src == "" {
		return nil
	}
	ex, err := rts.ParseExpr(path, line, col, src)
	if err != nil {
		return nil
	}

	var refs []string
	walkExpr(ex, func(path string) {
		refs = append(refs, path)
	})
	return refs
}

func jsonFileModuleRefs(path string, src string) []string {
	mod, err := rts.ParseModule(path, []byte(src))
	if err != nil {
		return nil
	}

	var refs []string
	walkStmts(mod.Stmts, func(path string) {
		refs = append(refs, path)
	})
	return refs
}

func walkStmts(stmts []rts.Stmt, add func(string)) {
	for _, st := range stmts {
		walkStmt(st, add)
	}
}

func walkStmt(st rts.Stmt, add func(string)) {
	switch s := st.(type) {
	case *rts.LetStmt:
		walkExpr(s.Val, add)
	case *rts.AssignStmt:
		walkExpr(s.Val, add)
	case *rts.ReturnStmt:
		walkExpr(s.Val, add)
	case *rts.ExprStmt:
		walkExpr(s.Exp, add)
	case *rts.FnDef:
		walkBlock(s.Body, add)
	case *rts.IfStmt:
		walkExpr(s.Cond, add)
		walkBlock(s.Then, add)
		for _, el := range s.Elifs {
			walkExpr(el.Cond, add)
			walkBlock(el.Body, add)
		}
		walkBlock(s.Else, add)
	case *rts.ForStmt:
		walkStmt(s.Init, add)
		walkExpr(s.Cond, add)
		walkStmt(s.Post, add)
		if s.Range != nil {
			walkExpr(s.Range.Expr, add)
		}
		walkBlock(s.Body, add)
	}
}

func walkBlock(block *rts.Block, add func(string)) {
	if block == nil {
		return
	}
	walkStmts(block.Stmts, add)
}

func walkExpr(ex rts.Expr, add func(string)) {
	switch e := ex.(type) {
	case *rts.Unary:
		walkExpr(e.X, add)
	case *rts.Binary:
		walkExpr(e.Left, add)
		walkExpr(e.Right, add)
	case *rts.Ternary:
		walkExpr(e.Cond, add)
		walkExpr(e.Then, add)
		walkExpr(e.Else, add)
	case *rts.TryExpr:
		walkExpr(e.X, add)
	case *rts.Call:
		if path, ok := literalJSONFileCall(e); ok {
			add(path)
		}
		walkExpr(e.Callee, add)
		for _, arg := range e.Args {
			walkExpr(arg, add)
		}
	case *rts.Index:
		walkExpr(e.X, add)
		walkExpr(e.Idx, add)
	case *rts.Member:
		walkExpr(e.X, add)
	case *rts.ListLit:
		for _, elem := range e.Elems {
			walkExpr(elem, add)
		}
	case *rts.DictLit:
		for _, entry := range e.Entries {
			walkExpr(entry.Val, add)
		}
	}
}

func literalJSONFileCall(call *rts.Call) (string, bool) {
	if call == nil || len(call.Args) == 0 || !isJSONFileCallee(call.Callee) {
		return "", false
	}
	lit, ok := call.Args[0].(*rts.Literal)
	if !ok || lit.Kind != rts.LitStr {
		return "", false
	}
	path := str.Trim(lit.S)
	return path, path != ""
}

func isJSONFileCallee(ex rts.Expr) bool {
	mem, ok := ex.(*rts.Member)
	if !ok || mem.Name != jsonFileMember {
		return false
	}

	switch base := mem.X.(type) {
	case *rts.Ident:
		return base.Name == jsonNamespace
	case *rts.Member:
		if base.Name != jsonNamespace {
			return false
		}
		root, ok := base.X.(*rts.Ident)
		return ok && (root.Name == rtsNamespace || root.Name == stdlibNamespace)
	default:
		return false
	}
}

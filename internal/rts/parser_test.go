package rts

import "testing"

func TestParseFnIf(t *testing.T) {
	src := "export fn f(a, b) {\nif a { return b } elif b { return a } else { return null }\n}\n"
	m, err := ParseModule("test", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(m.Stmts) != 1 {
		t.Fatalf("expected 1 stmt, got %d", len(m.Stmts))
	}
	fn, ok := m.Stmts[0].(*FnDef)
	if !ok {
		t.Fatalf("expected fn def")
	}
	if !fn.Exported {
		t.Fatalf("expected exported fn")
	}
}

func TestParseDict(t *testing.T) {
	src := "let x = {\"a\": 1, b: 2}\n"
	m, err := ParseModule("test", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(m.Stmts) != 1 {
		t.Fatalf("expected 1 stmt")
	}
	let, ok := m.Stmts[0].(*LetStmt)
	if !ok {
		t.Fatalf("expected let")
	}
	if _, ok := let.Val.(*DictLit); !ok {
		t.Fatalf("expected dict literal")
	}
}

func TestParseDictMultiline(t *testing.T) {
	src := "let x = {\n  a: 1,\n  b: 2\n}\n"
	m, err := ParseModule("test", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(m.Stmts) != 1 {
		t.Fatalf("expected 1 stmt")
	}
	let, ok := m.Stmts[0].(*LetStmt)
	if !ok {
		t.Fatalf("expected let")
	}
	dict, ok := let.Val.(*DictLit)
	if !ok {
		t.Fatalf("expected dict literal")
	}
	if len(dict.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(dict.Entries))
	}
}

func TestParseListMultiline(t *testing.T) {
	src := "let x = [\n  1,\n  2,\n  3\n]\n"
	m, err := ParseModule("test", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(m.Stmts) != 1 {
		t.Fatalf("expected 1 stmt")
	}
	let, ok := m.Stmts[0].(*LetStmt)
	if !ok {
		t.Fatalf("expected let")
	}
	list, ok := let.Val.(*ListLit)
	if !ok {
		t.Fatalf("expected list literal")
	}
	if len(list.Elems) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(list.Elems))
	}
}

func TestParseConst(t *testing.T) {
	src := "const x = 1\n"
	m, err := ParseModule("test", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(m.Stmts) != 1 {
		t.Fatalf("expected 1 stmt")
	}
	let, ok := m.Stmts[0].(*LetStmt)
	if !ok || !let.Const || let.Name != "x" {
		t.Fatalf("expected const let stmt")
	}
}

func TestParseTryExpr(t *testing.T) {
	src := "let x = try missing\n"
	m, err := ParseModule("test", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(m.Stmts) != 1 {
		t.Fatalf("expected 1 stmt")
	}
	let, ok := m.Stmts[0].(*LetStmt)
	if !ok {
		t.Fatalf("expected let stmt")
	}
	if _, ok := let.Val.(*TryExpr); !ok {
		t.Fatalf("expected try expr")
	}
}

func TestParseForClassic(t *testing.T) {
	src := "for let i = 0; i < 3; i = i + 1 { let x = i }\n"
	m, err := ParseModule("test", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(m.Stmts) != 1 {
		t.Fatalf("expected 1 stmt")
	}
	loop, ok := m.Stmts[0].(*ForStmt)
	if !ok {
		t.Fatalf("expected for stmt")
	}
	if loop.Init == nil || loop.Cond == nil || loop.Post == nil || loop.Body == nil {
		t.Fatalf("expected init/cond/post/body")
	}
}

func TestParseForCond(t *testing.T) {
	src := "for 1 < 2 { let x = 1 }\n"
	m, err := ParseModule("test", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(m.Stmts) != 1 {
		t.Fatalf("expected 1 stmt")
	}
	loop, ok := m.Stmts[0].(*ForStmt)
	if !ok {
		t.Fatalf("expected for stmt")
	}
	if loop.Cond == nil || loop.Init != nil || loop.Post != nil {
		t.Fatalf("expected condition-only loop")
	}
}

func TestParseForRange(t *testing.T) {
	src := "for let i, v range items { return v }\n"
	m, err := ParseModule("test", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(m.Stmts) != 1 {
		t.Fatalf("expected 1 stmt")
	}
	loop, ok := m.Stmts[0].(*ForStmt)
	if !ok {
		t.Fatalf("expected for stmt")
	}
	if loop.Range == nil {
		t.Fatalf("expected range loop")
	}
	if !loop.Range.Declare {
		t.Fatalf("expected range variables to declare")
	}
	if loop.Range.Key != "i" || loop.Range.Val != "v" {
		t.Fatalf("unexpected range vars: %q, %q", loop.Range.Key, loop.Range.Val)
	}
}

func TestParseForConstInitRejected(t *testing.T) {
	if _, err := ParseModule(
		"test",
		[]byte("for const i = 0; i < 1; i = i + 1 { }\n"),
	); err == nil {
		t.Fatalf("expected const in for init to error")
	}
	if _, err := ParseModule("test", []byte("for const i range items { }\n")); err == nil {
		t.Fatalf("expected const in range header to error")
	}
}

func TestParseBreakContinueOutsideLoop(t *testing.T) {
	if _, err := ParseModule("test", []byte("break\n")); err == nil {
		t.Fatalf("expected break outside loop error")
	}
	if _, err := ParseModule("test", []byte("continue\n")); err == nil {
		t.Fatalf("expected continue outside loop error")
	}
	if _, err := ParseModule("test", []byte("fn f(){ break }\n")); err == nil {
		t.Fatalf("expected break outside loop error in fn")
	}
}

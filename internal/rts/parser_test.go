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

func TestParseDuplicateParamRejected(t *testing.T) {
	if _, err := ParseModule("test", []byte("fn f(a, a) { return a }\n")); err == nil {
		t.Fatalf("expected duplicate parameter error")
	}
}

func TestParseModuleAtUsesInitialPos(t *testing.T) {
	_, err := ParseModuleAt("hook.rts", []byte("  fn f( {}\n"), Pos{Line: 10, Col: 1})
	if err == nil {
		t.Fatalf("expected parse error")
	}
	parseErr, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if parseErr.Pos.Path != "hook.rts" || parseErr.Pos.Line != 10 || parseErr.Pos.Col != 9 {
		t.Fatalf("unexpected parse position: %+v", parseErr.Pos)
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

func parseSwitchStmt(t *testing.T, src string) *SwitchStmt {
	t.Helper()
	m, err := ParseModule("test", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(m.Stmts) != 1 {
		t.Fatalf("expected 1 stmt, got %d", len(m.Stmts))
	}
	sw, ok := m.Stmts[0].(*SwitchStmt)
	if !ok {
		t.Fatalf("expected switch stmt, got %T", m.Stmts[0])
	}
	return sw
}

func TestParseSwitchTagged(t *testing.T) {
	src := "switch code {\ncase 200, 201:\n  out = \"ok\"\ncase 401:\n  out = \"no\"\ndefault:\n  out = \"?\"\n}\n"
	sw := parseSwitchStmt(t, src)
	if sw.Tag == nil {
		t.Fatalf("expected tag expression")
	}
	if len(sw.Clauses) != 3 {
		t.Fatalf("expected 3 clauses, got %d", len(sw.Clauses))
	}
	if len(sw.Clauses[0].Exprs) != 2 {
		t.Fatalf("expected 2 case expressions, got %d", len(sw.Clauses[0].Exprs))
	}
	if sw.Clauses[2].Exprs != nil {
		t.Fatalf("expected default clause to carry no expressions")
	}
	for i, cl := range sw.Clauses {
		if cl.Body == nil || len(cl.Body.Stmts) != 1 {
			t.Fatalf("clause %d: expected one body statement", i)
		}
	}
}

func TestParseSwitchTagless(t *testing.T) {
	src := "switch {\ncase score >= 90:\n  grade = \"A\"\ndefault:\n  grade = \"C\"\n}\n"
	sw := parseSwitchStmt(t, src)
	if sw.Tag != nil {
		t.Fatalf("expected tagless switch, got tag %T", sw.Tag)
	}
	if len(sw.Clauses) != 2 {
		t.Fatalf("expected 2 clauses, got %d", len(sw.Clauses))
	}
	if _, ok := sw.Clauses[0].Exprs[0].(*Binary); !ok {
		t.Fatalf("expected binary case expression, got %T", sw.Clauses[0].Exprs[0])
	}
}

func TestParseSwitchBraceIsAlwaysTagless(t *testing.T) {
	if _, err := ParseModule("test", []byte("switch {a: 1} {\ncase 1:\n}\n")); err == nil {
		t.Fatalf("expected a bare dict tag to be read as the tagless form")
	}
	sw := parseSwitchStmt(t, "switch ({a: 1}) {\ncase 1:\n}\n")
	if _, ok := sw.Tag.(*DictLit); !ok {
		t.Fatalf("expected parenthesized dict tag, got %T", sw.Tag)
	}
}

func TestParseSwitchEmptyForms(t *testing.T) {
	sw := parseSwitchStmt(t, "switch x {\n}\n")
	if len(sw.Clauses) != 0 {
		t.Fatalf("expected no clauses, got %d", len(sw.Clauses))
	}

	sw = parseSwitchStmt(t, "switch x {\ncase 1:\ncase 2:\ndefault:\n}\n")
	if len(sw.Clauses) != 3 {
		t.Fatalf("expected 3 clauses, got %d", len(sw.Clauses))
	}
	for i, cl := range sw.Clauses {
		if cl.Body == nil {
			t.Fatalf("clause %d: expected body block", i)
		}
		if len(cl.Body.Stmts) != 0 {
			t.Fatalf("clause %d: expected empty body, got %d stmts", i, len(cl.Body.Stmts))
		}
	}
}

func TestParseSwitchDefaultFirst(t *testing.T) {
	sw := parseSwitchStmt(t, "switch x {\ndefault:\n  y = 0\ncase 1:\n  y = 1\n}\n")
	if sw.Clauses[0].Exprs != nil {
		t.Fatalf("expected leading default clause")
	}
	if len(sw.Clauses[1].Exprs) != 1 {
		t.Fatalf("expected trailing case clause")
	}
}

func TestParseSwitchMultilineCaseList(t *testing.T) {
	sw := parseSwitchStmt(t, "switch x {\ncase 1,\n  2,\n  3:\n  y = 1\n}\n")
	if len(sw.Clauses[0].Exprs) != 3 {
		t.Fatalf("expected 3 case expressions, got %d", len(sw.Clauses[0].Exprs))
	}
}

func TestParseSwitchNested(t *testing.T) {
	src := "switch a {\ncase 1:\n  switch b {\n  case 2:\n    y = 1\n  }\n}\n"
	sw := parseSwitchStmt(t, src)
	inner, ok := sw.Clauses[0].Body.Stmts[0].(*SwitchStmt)
	if !ok {
		t.Fatalf("expected nested switch, got %T", sw.Clauses[0].Body.Stmts[0])
	}
	if len(inner.Clauses) != 1 {
		t.Fatalf("expected 1 inner clause, got %d", len(inner.Clauses))
	}
}

// a clause body that ends in a block leaves the lexer on an auto semi, which
// must not leak into the next clause
func TestParseSwitchClauseEndingInBlock(t *testing.T) {
	src := "switch x {\ncase 1:\n  if y {\n    z = 1\n  }\ncase 2:\n  for {\n    break\n  }\ndefault:\n  z = 3\n}\n"
	sw := parseSwitchStmt(t, src)
	if len(sw.Clauses) != 3 {
		t.Fatalf("expected 3 clauses, got %d", len(sw.Clauses))
	}
	for i, cl := range sw.Clauses {
		if len(cl.Body.Stmts) != 1 {
			t.Fatalf("clause %d: expected 1 stmt, got %d", i, len(cl.Body.Stmts))
		}
	}
}

func TestParseSwitchPositions(t *testing.T) {
	src := "let x = 1\nswitch x {\ncase 1:\n  x = 2\ndefault:\n  x = 3\n}\n"
	m, err := ParseModule("test", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sw, ok := m.Stmts[1].(*SwitchStmt)
	if !ok {
		t.Fatalf("expected switch stmt, got %T", m.Stmts[1])
	}
	if sw.Pos().Line != 2 || sw.Pos().Col != 1 {
		t.Fatalf("switch pos: got %d:%d, want 2:1", sw.Pos().Line, sw.Pos().Col)
	}
	if sw.Tag.Pos().Line != 2 || sw.Tag.Pos().Col != 8 {
		t.Fatalf("tag pos: got %d:%d, want 2:8", sw.Tag.Pos().Line, sw.Tag.Pos().Col)
	}
	if sw.Clauses[0].P.Line != 3 || sw.Clauses[0].P.Col != 1 {
		t.Fatalf("case pos: got %d:%d, want 3:1", sw.Clauses[0].P.Line, sw.Clauses[0].P.Col)
	}
	if sw.Clauses[0].Body.P.Line != 3 || sw.Clauses[0].Body.P.Col != 7 {
		t.Fatalf(
			"case body pos: got %d:%d, want 3:7",
			sw.Clauses[0].Body.P.Line,
			sw.Clauses[0].Body.P.Col,
		)
	}
	if sw.Clauses[1].P.Line != 5 || sw.Clauses[1].P.Col != 1 {
		t.Fatalf("default pos: got %d:%d, want 5:1", sw.Clauses[1].P.Line, sw.Clauses[1].P.Col)
	}
}

func TestParseSwitchMalformed(t *testing.T) {
	cases := []struct {
		name string
		src  string
		msg  string
	}{
		{"empty case list", "switch x {\ncase:\n}\n", "case requires an expression"},
		{"trailing comma", "switch x {\ncase 1, :\n}\n", "case requires an expression"},
		{"colon on next line", "switch x {\ncase 1\n:\n}\n", "expected :, got <auto-semi>"},
		{"brace on next line", "switch x\n{\n}\n", "expected {, got <auto-semi>"},
		{
			"statement before clause",
			"switch x {\nlet y = 1\ncase 1:\n}\n",
			"expected case or default, got let",
		},
		{"duplicate default", "switch x {\ndefault:\ndefault:\n}\n", "duplicate default in switch"},
		{"unterminated", "switch x {\ncase 1:\n  y = 1\n", "unterminated switch"},
		{"case outside switch", "case 1:\n", "case outside switch body"},
		{"default outside switch", "default:\n", "default outside switch body"},
		{"default in nested block", "switch x {\ncase 1:\n  if y {\n  default:\n  }\n}\n",
			"default outside switch body"},
		{"switch in for init", "for switch x { }; y; z { }\n", "invalid for init clause"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseModule("test", []byte(tc.src))
			pe, ok := err.(*ParseError)
			if !ok {
				t.Fatalf("expected *ParseError, got %T (%v)", err, err)
			}
			if pe.Msg != tc.msg {
				t.Fatalf("msg: got %q, want %q", pe.Msg, tc.msg)
			}
		})
	}
}

func TestParseSwitchBreakAndContinue(t *testing.T) {
	ok := []string{
		"switch 1 {\ncase 1:\n  break\n}\n",
		"for {\n  switch 1 {\n  case 1:\n    continue\n  }\n}\n",
		"for {\n  switch 1 {\n  case 1:\n    break\n  }\n  break\n}\n",
	}
	for _, src := range ok {
		if _, err := ParseModule("test", []byte(src)); err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
	}

	bad := []struct {
		src string
		msg string
	}{
		{"switch 1 {\ncase 1:\n  continue\n}\n", "continue outside loop"},
		{"switch 1 {\ncase 1:\n  fn g() {\n    break\n  }\n}\n", "break outside loop or switch"},
		{"break\n", "break outside loop or switch"},
	}
	for _, tc := range bad {
		_, err := ParseModule("test", []byte(tc.src))
		pe, ok := err.(*ParseError)
		if !ok {
			t.Fatalf("expected *ParseError for %q, got %T", tc.src, err)
		}
		if pe.Msg != tc.msg {
			t.Fatalf("msg: got %q, want %q", pe.Msg, tc.msg)
		}
	}
}

func TestParseDefaultStaysIdentifier(t *testing.T) {
	src := "let a = default(null, \"x\")\nlet b = rts.default(a, \"y\")\nlet c = {default: 1}.default\n"
	m, err := ParseModule("test", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(m.Stmts) != 3 {
		t.Fatalf("expected 3 stmts, got %d", len(m.Stmts))
	}
}

func TestParseModuleDecl(t *testing.T) {
	src := "module mod\nexport let x = 1\n"
	m, err := ParseModule("test", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.Name != "mod" {
		t.Fatalf("expected module name mod, got %q", m.Name)
	}
	if m.NamePos.Line != 1 || m.NamePos.Col != 1 {
		t.Fatalf("unexpected module pos: %v", m.NamePos)
	}
}

func TestParseModuleDeclNotFirst(t *testing.T) {
	if _, err := ParseModule("test", []byte("let x = 1\nmodule mod\n")); err == nil {
		t.Fatalf("expected module not-first error")
	}
}

func TestParseModuleDeclMissingName(t *testing.T) {
	if _, err := ParseModule("test", []byte("module\n")); err == nil {
		t.Fatalf("expected module missing name error")
	}
}

func TestParseModuleDeclDuplicate(t *testing.T) {
	if _, err := ParseModule("test", []byte("module a\nmodule b\n")); err == nil {
		t.Fatalf("expected module duplicate error")
	}
}

func TestParseExprIllegalTokenMessage(t *testing.T) {
	cases := []struct {
		name string
		src  string
		col  int
	}{
		{"trailing operator", "a && b", 3},
		{"inside parens", "(a & b)", 4},
		{"leading operator", "& a", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseExpr("t", 1, 1, tc.src)
			pe, ok := err.(*ParseError)
			if !ok {
				t.Fatalf("expected *ParseError, got %T", err)
			}
			if pe.Msg != "unexpected '&'" {
				t.Fatalf("msg: got %q, want %q", pe.Msg, "unexpected '&'")
			}
			if pe.Pos.Line != 1 || pe.Pos.Col != tc.col {
				t.Fatalf("pos: got %d:%d, want 1:%d", pe.Pos.Line, pe.Pos.Col, tc.col)
			}
		})
	}
}

func TestParseModuleIllegalTokenMessage(t *testing.T) {
	_, err := ParseModule("t", []byte("let x = !\n"))
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected *ParseError, got %T", err)
	}
	if pe.Msg != "unexpected '!'" {
		t.Fatalf("msg: got %q, want %q", pe.Msg, "unexpected '!'")
	}
	if pe.Pos.Line != 1 || pe.Pos.Col != 9 {
		t.Fatalf("pos: got %d:%d, want 1:9", pe.Pos.Line, pe.Pos.Col)
	}
	if got := pe.Error(); got != "t:1:9: unexpected '!'" {
		t.Fatalf("error string: got %q", got)
	}
}

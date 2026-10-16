package rts

import "slices"

import "testing"

func lexKinds(src string) []tokKind {
	lx := newLexer(NewFile("test", []byte(src)))
	var out []tokKind
	for {
		t := lx.scan()
		out = append(out, t.K)
		if t.K == tokEOF {
			return out
		}
	}
}

func TestLexerAutoSemi(t *testing.T) {
	src := "let a = 1\nlet b = 2\n"
	k := lexKinds(src)
	seen := slices.Contains(k, tokAutoSemi)
	if !seen {
		t.Fatalf("expected auto semi")
	}
}

func TestLexerSwitchKeywords(t *testing.T) {
	got := lexKinds("switch case default")
	want := []tokKind{tokSwitch, tokCase, tokDefault, tokEOF}
	if !slices.Equal(got, want) {
		t.Fatalf("kinds: got %v, want %v", got, want)
	}
}

// default carries a colon like case does, so it must not close a statement
func TestLexerNoSemiAfterDefault(t *testing.T) {
	k := lexKinds("switch x {\ndefault:\n  y = 1\n}\n")
	for i := 0; i < len(k)-1; i++ {
		if k[i] == tokDefault && k[i+1] == tokAutoSemi {
			t.Fatalf("unexpected auto semi after default")
		}
	}
}

func TestKeywordClassSwitchCase(t *testing.T) {
	cases := []struct {
		name string
		want KeywordClass
	}{
		{"switch", KeywordControl},
		{"case", KeywordControl},
		{"default", KeywordControl},
	}
	for _, tc := range cases {
		if got := KeywordClassOf(tc.name); got != tc.want {
			t.Fatalf("%s: got class %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestLexerKeepsInAsAnIdentifier(t *testing.T) {
	got := lexKinds("in object.in {in: 1}")
	want := []tokKind{
		tokIdent,
		tokIdent, tokDot, tokIdent,
		tokLbrace, tokIdent, tokColon, tokNumber, tokRbrace,
		tokAutoSemi, tokEOF,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("kinds: got %v, want %v", got, want)
	}
	if IsKeyword("in") {
		t.Fatal("in must remain available as a binding name")
	}
	if got := KeywordClassOf("in"); got != KeywordNone {
		t.Fatalf("in keyword class = %v, want none", got)
	}
}

func TestLexerNoSemiInCaseList(t *testing.T) {
	k := lexKinds("switch x {\ncase 1,\n2:\n}\n")
	for i := 0; i < len(k)-1; i++ {
		if k[i] == tokComma && k[i+1] == tokAutoSemi {
			t.Fatalf("unexpected auto semi after case comma")
		}
		if k[i] == tokColon && k[i+1] == tokAutoSemi {
			t.Fatalf("unexpected auto semi after case colon")
		}
	}
}

func TestLexerLogicalSymbols(t *testing.T) {
	got := lexKinds("!a && b || c")
	want := []tokKind{tokBang, tokIdent, tokAndAnd, tokIdent, tokOrOr, tokIdent, tokAutoSemi, tokEOF}
	if !slices.Equal(got, want) {
		t.Fatalf("kinds: got %v, want %v", got, want)
	}
}

func TestLexerBangKeepsNotEqual(t *testing.T) {
	got := lexKinds("a != !b")
	want := []tokKind{tokIdent, tokNe, tokBang, tokIdent, tokAutoSemi, tokEOF}
	if !slices.Equal(got, want) {
		t.Fatalf("kinds: got %v, want %v", got, want)
	}
}

func TestLexerLoneAmpersandOrPipeIsIllegal(t *testing.T) {
	cases := []struct {
		src string
		msg string
	}{
		{"a & b", "unexpected '&'"},
		{"a | b", "unexpected '|'"},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			lx := newLexer(NewFile("test", []byte(tc.src)))
			lx.scan()
			tok := lx.scan()
			if tok.K != tokIllegal || tok.Lit != tc.msg {
				t.Fatalf("token: got %v %q, want ILLEGAL %q", tok.K, tok.Lit, tc.msg)
			}
			if tok.P.Line != 1 || tok.P.Col != 3 {
				t.Fatalf("pos: got %d:%d, want 1:3", tok.P.Line, tok.P.Col)
			}
		})
	}
}

func TestLexerNoSemiAfterLogicalSymbol(t *testing.T) {
	k := lexKinds("let a = b &&\nc ||\nd\n")
	for i := 0; i < len(k)-1; i++ {
		if (k[i] == tokAndAnd || k[i] == tokOrOr) && k[i+1] == tokAutoSemi {
			t.Fatalf("unexpected auto semi after %v", k[i])
		}
	}
}

func TestLexerNoSemiInParens(t *testing.T) {
	src := "let a = (1\n+2)\n"
	k := lexKinds(src)
	for i := 0; i < len(k)-1; i++ {
		if k[i] == tokNumber && k[i+1] == tokAutoSemi {
			t.Fatalf("unexpected auto semi inside parens")
		}
	}
}

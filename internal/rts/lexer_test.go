package rts

import "slices"

import "testing"

func lexKinds(src string) []Kind {
	lx := NewLexer("test", []byte(src))
	var out []Kind
	for {
		t := lx.Next()
		out = append(out, t.K)
		if t.K == EOF {
			return out
		}
	}
}

func TestLexerAutoSemi(t *testing.T) {
	src := "let a = 1\nlet b = 2\n"
	k := lexKinds(src)
	seen := slices.Contains(k, AUTO_SEMI)
	if !seen {
		t.Fatalf("expected auto semi")
	}
}

func TestLexerSwitchKeywords(t *testing.T) {
	got := lexKinds("switch case default")
	want := []Kind{KW_SWITCH, KW_CASE, KW_DEFAULT, EOF}
	if !slices.Equal(got, want) {
		t.Fatalf("kinds: got %v, want %v", got, want)
	}
}

// default carries a colon like case does, so it must not close a statement
func TestLexerNoSemiAfterDefault(t *testing.T) {
	k := lexKinds("switch x {\ndefault:\n  y = 1\n}\n")
	for i := 0; i < len(k)-1; i++ {
		if k[i] == KW_DEFAULT && k[i+1] == AUTO_SEMI {
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

func TestLexerNoSemiInCaseList(t *testing.T) {
	k := lexKinds("switch x {\ncase 1,\n2:\n}\n")
	for i := 0; i < len(k)-1; i++ {
		if k[i] == COMMA && k[i+1] == AUTO_SEMI {
			t.Fatalf("unexpected auto semi after case comma")
		}
		if k[i] == COLON && k[i+1] == AUTO_SEMI {
			t.Fatalf("unexpected auto semi after case colon")
		}
	}
}

func TestLexerNoSemiInParens(t *testing.T) {
	src := "let a = (1\n+2)\n"
	k := lexKinds(src)
	for i := 0; i < len(k)-1; i++ {
		if k[i] == NUMBER && k[i+1] == AUTO_SEMI {
			t.Fatalf("unexpected auto semi inside parens")
		}
	}
}

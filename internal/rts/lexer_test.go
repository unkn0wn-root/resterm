package rts

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
	seen := false
	for _, it := range k {
		if it == AUTO_SEMI {
			seen = true
			break
		}
	}
	if !seen {
		t.Fatalf("expected auto semi")
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

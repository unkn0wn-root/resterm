package digest

import (
	"maps"
	"testing"
)

func TestParseChallenges(t *testing.T) {
	type ch = map[string]string
	tests := []struct {
		name    string
		value   string
		schemes []string
		params  []ch
	}{
		{
			name:    "several challenges in one value",
			value:   `Basic realm="a", Digest realm="b", nonce="c"`,
			schemes: []string{"Basic", "Digest"},
			params:  []ch{{"realm": "a"}, {"realm": "b", "nonce": "c"}},
		},
		{
			name:    "comma inside quotes",
			value:   `Digest realm="a, b", nonce=n`,
			schemes: []string{"Digest"},
			params:  []ch{{"realm": "a, b", "nonce": "n"}},
		},
		{
			name:    "escaped quote and backslash",
			value:   `Digest realm="say \"hi\" \\o/", nonce="a\,b"`,
			schemes: []string{"Digest"},
			params:  []ch{{"realm": `say "hi" \o/`, "nonce": "a,b"}},
		},
		{
			name:    "spaces around equals",
			value:   `Digest realm = "r" , nonce	=	n`,
			schemes: []string{"Digest"},
			params:  []ch{{"realm": "r", "nonce": "n"}},
		},
		{
			name:    "names are case insensitive",
			value:   `Digest Realm="r", NONCE="n"`,
			schemes: []string{"Digest"},
			params:  []ch{{"realm": "r", "nonce": "n"}},
		},
		{
			name:    "token68 is not a parameter",
			value:   `Negotiate abc==, Digest nonce="n"`,
			schemes: []string{"Negotiate", "Digest"},
			params:  []ch{{}, {"nonce": "n"}},
		},
		{
			name:    "bare scheme and empty items",
			value:   ` , Basic,, Digest nonce="n",`,
			schemes: []string{"Basic", "Digest"},
			params:  []ch{{}, {"nonce": "n"}},
		},
		{
			name:    "equals inside a quoted value",
			value:   `Digest nonce="a=b=="`,
			schemes: []string{"Digest"},
			params:  []ch{{"nonce": "a=b=="}},
		},
		{
			name:    "unclosed quote is dropped",
			value:   `Digest realm="r", nonce="n`,
			schemes: []string{"Digest"},
			params:  []ch{{"realm": "r"}},
		},
		{
			name:    "parameter before any scheme is dropped",
			value:   `realm="r", Digest nonce="n"`,
			schemes: []string{"Digest"},
			params:  []ch{{"nonce": "n"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseChallenges(tt.value)
			if len(got) != len(tt.schemes) {
				t.Fatalf("got %d challenges %+v, want %d", len(got), got, len(tt.schemes))
			}
			for i, c := range got {
				if c.scheme != tt.schemes[i] || !maps.Equal(c.params, tt.params[i]) {
					t.Fatalf("challenge %d = %s %v, want %s %v", i, c.scheme, c.params, tt.schemes[i], tt.params[i])
				}
			}
		})
	}
}

func FuzzPick(f *testing.F) {
	for _, seed := range []string{
		rfc2617Challenge,
		`Basic realm="a", Digest realm="b", nonce="c", algorithm=SHA-256-sess, qop="auth-int"`,
		`Negotiate abc==, Digest nonce="n\"\\"`,
		`Digest realm = "r" , nonce	=	n,,`,
		`"`,
		`=`,
		`Digest nonce="`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		c, ok := Pick([]string{value})
		if !ok {
			return
		}
		got := c.Authorization(rfc2617User, "GET", "/", []byte("body"))
		back := parseChallenges(got)
		if len(back) != 1 || back[0].params["nonce"] != c.params["nonce"] {
			t.Fatalf("answer %q does not read back with nonce %q", got, c.params["nonce"])
		}
	})
}

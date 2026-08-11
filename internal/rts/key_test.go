package rts

import (
	"context"
	"testing"
)

func TestLookupIgnoresSurroundingWhitespace(t *testing.T) {
	e := NewEng(testStdlib)
	rt := RT{
		Env:     map[string]string{"mode": "dev"},
		Vars:    map[string]string{"token": "abc"},
		Globals: map[string]string{"flag": "on"},
	}
	pos := Pos{Path: "test", Line: 1, Col: 1}

	tests := []struct {
		src  string
		want string
	}{
		{`vars.get(" token ")`, "abc"},
		{`vars.require(" token ")`, "abc"},
		{`vars[" token "]`, "abc"},
		{`vars.global.get(" flag ")`, "on"},
		{`vars.global.require(" flag ")`, "on"},
		{`env.get(" mode ")`, "dev"},
	}
	for _, tt := range tests {
		v, err := e.Eval(context.Background(), rt, tt.src, pos)
		if err != nil {
			t.Errorf("%s: %v", tt.src, err)
			continue
		}
		if v.K != VStr || v.S != tt.want {
			t.Errorf("%s = %+v, want %q", tt.src, v, tt.want)
		}
	}

	for _, src := range []string{`vars.has(" token ")`, `vars.global.has(" flag ")`} {
		v, err := e.Eval(context.Background(), rt, src, pos)
		if err != nil {
			t.Errorf("%s: %v", src, err)
			continue
		}
		if v.K != VBool || !v.B {
			t.Errorf("%s = %+v, want true", src, v)
		}
	}
}

func TestSetTrimsNameSoLaterReadsMatch(t *testing.T) {
	e := NewEng(testStdlib)
	mut := &recordingVarsMut{}
	rt := RT{Vars: map[string]string{}, VarsMut: mut}
	pos := Pos{Path: "test", Line: 1, Col: 1}

	v, err := e.Eval(
		context.Background(),
		rt,
		`vars.set(" token ", "abc") ?? vars.get(" token ")`,
		pos,
	)
	if err != nil {
		t.Fatalf("set then get: %v", err)
	}
	if v.K != VStr || v.S != "abc" {
		t.Fatalf("vars.get(\" token \") = %+v, want abc", v)
	}
	if mut.name != "token" {
		t.Fatalf("recorded name = %q, want %q", mut.name, "token")
	}
}

type recordingVarsMut struct {
	name  string
	value string
}

func (m *recordingVarsMut) SetVar(name, value string) {
	m.name, m.value = name, value
}

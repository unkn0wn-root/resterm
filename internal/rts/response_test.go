package rts

import (
	"context"
	"strings"
	"testing"
)

func evalRT2(t *testing.T, rt RT, src string) Value {
	t.Helper()
	e := NewEng(testStdlib)
	v, err := e.Eval(context.Background(), rt, src, Pos{Path: "test", Line: 1, Col: 1})
	if err != nil {
		t.Fatalf("eval %q: %v", src, err)
	}
	return v
}

func TestResponseObject(t *testing.T) {
	resp := &Resp{
		Status: "200 OK",
		Code:   200,
		H:      map[string][]string{"Content-Type": {"application/json"}},
		Body:   []byte(`{"ok":true}`),
		URL:    "https://example.com",
	}
	rt := RT{Res: resp}
	v := evalRT2(t, rt, "response.statusCode")
	if v.K != VNum || v.N != 200 {
		t.Fatalf("expected statusCode 200, got %+v", v)
	}
	v = evalRT2(t, rt, "response.status")
	if v.K != VNum || v.N != 200 {
		t.Fatalf("expected status 200, got %+v", v)
	}
	v = evalRT2(t, rt, "response.statusText")
	if v.K != VStr || v.S != "200 OK" {
		t.Fatalf("expected status text, got %+v", v)
	}
	v = evalRT2(t, rt, "response.header(\"Content-Type\")")
	if v.K != VStr || v.S != "application/json" {
		t.Fatalf("expected content type, got %+v", v)
	}
	v = evalRT2(t, rt, "response.json().ok")
	if v.K != VBool || v.B != true {
		t.Fatalf("expected json ok true, got %+v", v)
	}
	v = evalRT2(t, rt, "response.text()")
	if v.K != VStr || v.S != `{"ok":true}` {
		t.Fatalf("expected text body, got %+v", v)
	}
}

func TestUnboundResponseRejectsEveryRead(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{name: "bare", expr: "response"},
		{name: "truthiness", expr: "not response"},
		{name: "equality", expr: "response == null"},
		{name: "coalesce", expr: "response ?? last"},
		{name: "member", expr: "response.statusCode"},
		{name: "try bare", expr: "try response"},
		{name: "try member", expr: "try response.statusCode"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := NewEng(testStdlib)
			_, err := eng.Eval(
				context.Background(),
				RT{Resp: &Resp{Code: 204}},
				tt.expr,
				Pos{Path: "test", Line: 1, Col: 1},
			)
			if err == nil {
				t.Fatalf("eval %q: expected unbound response error", tt.expr)
			}
			if !isAbort(err) {
				t.Fatalf("eval %q: error = %v, want an abort", tt.expr, err)
			}
			if !strings.Contains(err.Error(), unboundResponse) {
				t.Fatalf("eval %q: error = %v, want %q", tt.expr, err, unboundResponse)
			}
		})
	}
}

// Reaching the object through a container skips the identifier check, so the
// member and index paths have to stop it themselves.
func TestUnboundObjectAbortsThroughMemberAndIndex(t *testing.T) {
	unbound := func() Value { return Obj(newUnboundRespObj("response", unboundResponse)) }
	rt := RT{Extra: map[string]Value{
		"holder": Dict(map[string]Value{"resp": unbound()}),
		"items":  List([]Value{unbound()}),
	}}

	tests := map[string]string{
		"member through a dict": `holder.resp.statusCode`,
		"index through a dict":  `holder.resp["statusCode"]`,
		"member through a list": `items[0].statusCode`,
		"index through a list":  `items[0]["statusCode"]`,
		"member behind try":     `try holder.resp.statusCode`,
		"index behind try":      `try items[0]["statusCode"]`,
	}

	for name, expr := range tests {
		t.Run(name, func(t *testing.T) {
			eng := NewEng(testStdlib)
			_, err := eng.Eval(context.Background(), rt, expr, Pos{Path: "test", Line: 1, Col: 1})
			if err == nil {
				t.Fatalf("eval %q: expected unbound response error", expr)
			}
			if !isAbort(err) {
				t.Fatalf("eval %q: error = %v, want an abort", expr, err)
			}
			if !strings.Contains(err.Error(), unboundResponse) {
				t.Fatalf("eval %q: error = %v, want %q", expr, err, unboundResponse)
			}
		})
	}
}

func TestAssertExtra(t *testing.T) {
	resp := &Resp{
		Status: "201 Created",
		Code:   201,
		H:      map[string][]string{"Content-Type": {"application/json"}},
		Body:   []byte(`{"ok":true}`),
	}
	rt := RT{Res: resp, Extra: AssertExtra(resp)}
	v := evalRT2(t, rt, "status == 201")
	if v.K != VBool || v.B != true {
		t.Fatalf("expected status == 201, got %+v", v)
	}
	v = evalRT2(t, rt, "header(\"Content-Type\") == \"application/json\"")
	if v.K != VBool || v.B != true {
		t.Fatalf("expected header match, got %+v", v)
	}
	v = evalRT2(t, rt, "text() == \"{\\\"ok\\\":true}\"")
	if v.K != VBool || v.B != true {
		t.Fatalf("expected text match, got %+v", v)
	}
}

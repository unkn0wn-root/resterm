package rts

import (
	"context"
	"testing"
)

func evalRT2(t *testing.T, rt RT, src string) Value {
	t.Helper()
	e := NewEng()
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

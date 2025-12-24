package rts

import (
	"context"
	"strings"
	"testing"
)

func evalExprCtx(t *testing.T, ctx *Ctx, src string) Value {
	ex, err := ParseExpr("test", 1, 1, src)
	if err != nil {
		t.Fatalf("parse expr: %v", err)
	}
	vm := &VM{ctx: ctx}
	env := NewEnv(nil)
	for k, v := range Builtins() {
		env.DefConst(k, v)
	}
	v, err := vm.eval(env, ex)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	return v
}

func TestBuiltinsCore(t *testing.T) {
	ctx := NewCtx(context.Background(), Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, "len([1,2,3])")
	if v.K != VNum || v.N != 3 {
		t.Fatalf("expected 3")
	}
	v = evalExprCtx(t, ctx, "contains([\"a\",\"b\"], \"b\")")
	if v.K != VBool || v.B != true {
		t.Fatalf("expected true")
	}
	v = evalExprCtx(t, ctx, "match(\"a.*\", \"abc\")")
	if v.K != VBool || v.B != true {
		t.Fatalf("expected true")
	}
}

func TestBuiltinJSONFile(t *testing.T) {
	ctx := NewCtx(context.Background(), Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	ctx.ReadFile = func(path string) ([]byte, error) {
		return []byte("[{\"id\":1}]"), nil
	}
	v := evalExprCtx(t, ctx, "json.file(\"x.json\")")
	if v.K != VList || len(v.L) != 1 {
		t.Fatalf("expected list")
	}
	if v.L[0].K != VDict {
		t.Fatalf("expected dict")
	}
}

func TestBuiltinJSONParseStringify(t *testing.T) {
	ctx := NewCtx(context.Background(), Limits{MaxStr: 4096, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, "json.parse(\"{\\\"a\\\":1}\").a")
	if v.K != VNum || v.N != 1 {
		t.Fatalf("expected 1")
	}
	v = evalExprCtx(t, ctx, "json.stringify({a:1})")
	if v.K != VStr || v.S != "{\"a\":1}" {
		t.Fatalf("unexpected json: %q", v.S)
	}
	v = evalExprCtx(t, ctx, "json.stringify({a:1}, 2)")
	if v.K != VStr || !strings.Contains(v.S, "\n") || !strings.Contains(v.S, "\"a\": 1") {
		t.Fatalf("expected indented json")
	}
}

func TestBuiltinHeadersHelpers(t *testing.T) {
	ctx := NewCtx(context.Background(), Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, "headers.get(headers.normalize({\"X-Test\":\"ok\"}), \"x-test\")")
	if v.K != VStr || v.S != "ok" {
		t.Fatalf("expected header value")
	}
	v = evalExprCtx(t, ctx, "len(headers.merge({\"a\":\"1\",\"b\":\"2\"}, {\"b\": null}))")
	if v.K != VNum || v.N != 1 {
		t.Fatalf("expected merged headers length 1")
	}
	v = evalExprCtx(t, ctx, "headers.has({\"A\": [\"1\",\"2\"]}, \"a\")")
	if v.K != VBool || v.B != true {
		t.Fatalf("expected headers.has true")
	}
}

func TestBuiltinQueryHelpers(t *testing.T) {
	ctx := NewCtx(context.Background(), Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, "len(query.parse(\"https://x.test?p=1&p=2\").p)")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected 2 query values")
	}
	v = evalExprCtx(t, ctx, "query.parse(query.encode({a:\"1\", b:[\"x\",\"y\"]})).a")
	if v.K != VStr || v.S != "1" {
		t.Fatalf("expected query value")
	}
	v = evalExprCtx(t, ctx, "len(query.parse(query.merge(\"https://x.test?p=1&q=2\", {q: null})))")
	if v.K != VNum || v.N != 1 {
		t.Fatalf("expected query length 1")
	}
}

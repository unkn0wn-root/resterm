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
	for k, v := range Stdlib() {
		env.DefConst(k, v)
	}
	v, err := vm.eval(env, ex)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	return v
}

func TestStdlibCore(t *testing.T) {
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

func TestStdlibJSONFile(t *testing.T) {
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

func TestStdlibJSONParseStringify(t *testing.T) {
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

func TestStdlibJSONGet(t *testing.T) {
	ctx := NewCtx(context.Background(), Limits{MaxStr: 4096, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, "json.get({a:{b:[1,2]}}, \"a.b[1]\")")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected json.get path 2")
	}
	v = evalExprCtx(t, ctx, "json.get({a:{b:[1,2]}}, \"$.a.b[0]\")")
	if v.K != VNum || v.N != 1 {
		t.Fatalf("expected json.get $ path 1")
	}
	v = evalExprCtx(t, ctx, "json.get({a:1}, \"missing\")")
	if v.K != VNull {
		t.Fatalf("expected json.get missing null")
	}
	v = evalExprCtx(t, ctx, "json.get({a:1}).a")
	if v.K != VNum || v.N != 1 {
		t.Fatalf("expected json.get value a=1")
	}
}

func TestStdlibHeadersHelpers(t *testing.T) {
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

func TestStdlibQueryHelpers(t *testing.T) {
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

func TestStdlibTextHelpers(t *testing.T) {
	ctx := NewCtx(context.Background(), Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, "stdlib.text.lower(\"AbC\")")
	if v.K != VStr || v.S != "abc" {
		t.Fatalf("expected lower abc")
	}
	v = evalExprCtx(t, ctx, "stdlib.text.upper(\"AbC\")")
	if v.K != VStr || v.S != "ABC" {
		t.Fatalf("expected upper ABC")
	}
	v = evalExprCtx(t, ctx, "stdlib.text.trim(\"  a \\t\")")
	if v.K != VStr || v.S != "a" {
		t.Fatalf("expected trim a")
	}
	v = evalExprCtx(t, ctx, "stdlib.text.split(\"a,b\", \",\")[1]")
	if v.K != VStr || v.S != "b" {
		t.Fatalf("expected split b")
	}
	v = evalExprCtx(t, ctx, "stdlib.text.join([\"a\",\"b\"], \"-\")")
	if v.K != VStr || v.S != "a-b" {
		t.Fatalf("expected join a-b")
	}
	v = evalExprCtx(t, ctx, "stdlib.text.replace(\"a-b\", \"-\", \":\")")
	if v.K != VStr || v.S != "a:b" {
		t.Fatalf("expected replace a:b")
	}
	v = evalExprCtx(t, ctx, "stdlib.text.startsWith(\"hello\", \"he\")")
	if v.K != VBool || v.B != true {
		t.Fatalf("expected startsWith true")
	}
	v = evalExprCtx(t, ctx, "stdlib.text.endsWith(\"hello\", \"lo\")")
	if v.K != VBool || v.B != true {
		t.Fatalf("expected endsWith true")
	}
	v = evalExprCtx(t, ctx, "stdlib.text.lower(\"XY\")")
	if v.K != VStr || v.S != "xy" {
		t.Fatalf("expected stdlib text lower")
	}
}

func TestStdlibListDictHelpers(t *testing.T) {
	ctx := NewCtx(context.Background(), Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, "stdlib.list.append([1,2], 3)[2]")
	if v.K != VNum || v.N != 3 {
		t.Fatalf("expected list.append to add 3")
	}
	v = evalExprCtx(t, ctx, "len(stdlib.list.concat([1], [2,3]))")
	if v.K != VNum || v.N != 3 {
		t.Fatalf("expected list.concat length 3")
	}
	v = evalExprCtx(t, ctx, "stdlib.list.sort([3,1,2])[0]")
	if v.K != VNum || v.N != 1 {
		t.Fatalf("expected list.sort numbers")
	}
	v = evalExprCtx(t, ctx, "stdlib.list.sort([\"b\",\"a\"])[0]")
	if v.K != VStr || v.S != "a" {
		t.Fatalf("expected list.sort strings")
	}
	v = evalExprCtx(t, ctx, "stdlib.dict.keys({b:1, a:2})[0]")
	if v.K != VStr || v.S != "a" {
		t.Fatalf("expected dict.keys sorted")
	}
	v = evalExprCtx(t, ctx, "stdlib.dict.values({b:1, a:2})[0]")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected dict.values sorted")
	}
	v = evalExprCtx(t, ctx, "stdlib.dict.items({a:1})[0].key")
	if v.K != VStr || v.S != "a" {
		t.Fatalf("expected dict.items key")
	}
	v = evalExprCtx(t, ctx, "stdlib.dict.items({a:1})[0].value")
	if v.K != VNum || v.N != 1 {
		t.Fatalf("expected dict.items value")
	}
	v = evalExprCtx(t, ctx, "stdlib.dict.set({a:1}, \"b\", 2).b")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected dict.set b=2")
	}
	v = evalExprCtx(t, ctx, "stdlib.dict.merge({a:1}, {a:2, b:3}).a")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected dict.merge to override")
	}
	v = evalExprCtx(t, ctx, "stdlib.dict.remove({a:1,b:2}, \"a\").a")
	if v.K != VNull {
		t.Fatalf("expected dict.remove to drop key")
	}
}

func TestStdlibMathHelpers(t *testing.T) {
	ctx := NewCtx(context.Background(), Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, "stdlib.math.abs(-2)")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected math.abs 2")
	}
	v = evalExprCtx(t, ctx, "stdlib.math.min(2, -1)")
	if v.K != VNum || v.N != -1 {
		t.Fatalf("expected math.min -1")
	}
	v = evalExprCtx(t, ctx, "stdlib.math.max(2, -1)")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected math.max 2")
	}
	v = evalExprCtx(t, ctx, "stdlib.math.clamp(5, 1, 3)")
	if v.K != VNum || v.N != 3 {
		t.Fatalf("expected math.clamp high 3")
	}
	v = evalExprCtx(t, ctx, "stdlib.math.clamp(2, 1, 3)")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected math.clamp mid 2")
	}
	v = evalExprCtx(t, ctx, "stdlib.math.floor(1.8)")
	if v.K != VNum || v.N != 1 {
		t.Fatalf("expected math.floor 1")
	}
	v = evalExprCtx(t, ctx, "stdlib.math.ceil(1.2)")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected math.ceil 2")
	}
	v = evalExprCtx(t, ctx, "stdlib.math.round(1.5)")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected math.round 2")
	}
}

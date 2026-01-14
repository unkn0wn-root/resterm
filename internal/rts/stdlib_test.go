package rts

import (
	"context"
	"strings"
	"testing"
	"time"
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

func evalExprMod(t *testing.T, ctx *Ctx, modSrc, expr string) Value {
	mod, err := ParseModule("test", []byte(modSrc))
	if err != nil {
		t.Fatalf("parse mod: %v", err)
	}
	comp, err := Exec(ctx, mod, Stdlib())
	if err != nil {
		t.Fatalf("exec mod: %v", err)
	}
	ex, err := ParseExpr("test", 1, 1, expr)
	if err != nil {
		t.Fatalf("parse expr: %v", err)
	}
	vm := &VM{ctx: ctx}
	v, err := vm.eval(comp.Env, ex)
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

func TestStdlibCrypto(t *testing.T) {
	ctx := NewCtx(context.Background(), Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, "crypto.sha256(\"abc\")")
	if v.K != VStr || v.S != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("expected sha256")
	}
	v = evalExprCtx(t, ctx, "crypto.hmacSha256(\"key\", \"data\")")
	if v.K != VStr || v.S != "5031fe3d989c6d1537a013fa6e739da23463fdaec3b70137d828e36ace221bd0" {
		t.Fatalf("expected hmac sha256")
	}
}

func TestStdlibEncoding(t *testing.T) {
	ctx := NewCtx(context.Background(), Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, "encoding.hex.encode(\"hi\")")
	if v.K != VStr || v.S != "6869" {
		t.Fatalf("expected hex encode")
	}
	v = evalExprCtx(t, ctx, "encoding.hex.decode(\"6869\")")
	if v.K != VStr || v.S != "hi" {
		t.Fatalf("expected hex decode")
	}
	v = evalExprCtx(t, ctx, "encoding.base64url.encode(\"hi\")")
	if v.K != VStr || v.S != "aGk" {
		t.Fatalf("expected base64url encode")
	}
	v = evalExprCtx(t, ctx, "encoding.base64url.decode(\"aGk=\")")
	if v.K != VStr || v.S != "hi" {
		t.Fatalf("expected base64url decode")
	}
}

func TestStdlibTypes(t *testing.T) {
	ctx := NewCtx(context.Background(), Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, "num(\"1.5\")")
	if v.K != VNum || v.N != 1.5 {
		t.Fatalf("expected num 1.5")
	}
	v = evalExprCtx(t, ctx, "num(true)")
	if v.K != VNum || v.N != 1 {
		t.Fatalf("expected num true")
	}
	v = evalExprCtx(t, ctx, "num(\"bad\", 2)")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected num default")
	}
	v = evalExprCtx(t, ctx, "int(3.0)")
	if v.K != VNum || v.N != 3 {
		t.Fatalf("expected int 3")
	}
	v = evalExprCtx(t, ctx, "int(\"7\")")
	if v.K != VNum || v.N != 7 {
		t.Fatalf("expected int 7")
	}
	v = evalExprCtx(t, ctx, "int(3.2, 4)")
	if v.K != VNum || v.N != 4 {
		t.Fatalf("expected int default")
	}
	v = evalExprCtx(t, ctx, "bool(\"true\")")
	if v.K != VBool || v.B != true {
		t.Fatalf("expected bool true")
	}
	v = evalExprCtx(t, ctx, "bool(\"0\")")
	if v.K != VBool || v.B != false {
		t.Fatalf("expected bool false")
	}
	v = evalExprCtx(t, ctx, "bool(\"\", true)")
	if v.K != VBool || v.B != true {
		t.Fatalf("expected bool default")
	}
	v = evalExprCtx(t, ctx, "typeof(rts)")
	if v.K != VStr || v.S != "rts" {
		t.Fatalf("expected typeof rts")
	}
	v = evalExprCtx(t, ctx, "typeof(stdlib)")
	if v.K != VStr || v.S != "stdlib" {
		t.Fatalf("expected typeof stdlib alias")
	}
}

func TestStdlibTimeExtras(t *testing.T) {
	ctx := NewCtx(context.Background(), Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	base := time.Date(2024, 1, 2, 3, 4, 5, 600*int(time.Millisecond), time.UTC)
	ctx.Now = func() time.Time { return base }

	v := evalExprCtx(t, ctx, "time.nowUnix()")
	if v.K != VNum || v.N != float64(base.Unix()) {
		t.Fatalf("expected nowUnix")
	}
	v = evalExprCtx(t, ctx, "time.nowUnixMs()")
	ms := base.UnixNano() / int64(time.Millisecond)
	if v.K != VNum || v.N != float64(ms) {
		t.Fatalf("expected nowUnixMs")
	}
	v = evalExprCtx(t, ctx, "time.format(\"2006-01-02\")")
	if v.K != VStr || v.S != "2024-01-02" {
		t.Fatalf("expected time.format")
	}
	v = evalExprCtx(t, ctx, "time.parse(\"2006-01-02T15:04:05Z07:00\", \"1970-01-01T00:00:01Z\")")
	if v.K != VNum || v.N != 1 {
		t.Fatalf("expected time.parse unix")
	}
	v = evalExprCtx(
		t,
		ctx,
		"time.parse(\"2006-01-02T15:04:05.000Z07:00\", \"1970-01-01T00:00:01.250Z\")",
	)
	if v.K != VNum || v.N != 1.25 {
		t.Fatalf("expected time.parse fractional")
	}
	v = evalExprCtx(t, ctx, "time.formatUnix(0, \"2006-01-02\")")
	if v.K != VStr || v.S != "1970-01-01" {
		t.Fatalf("expected time.formatUnix")
	}
	v = evalExprCtx(t, ctx, "time.formatUnix(1.25, \"2006-01-02T15:04:05.000Z07:00\")")
	if v.K != VStr || v.S != "1970-01-01T00:00:01.250Z" {
		t.Fatalf("expected time.formatUnix fractional")
	}
	v = evalExprCtx(t, ctx, "time.addUnix(1.5, 2.25)")
	if v.K != VNum || v.N != 3.75 {
		t.Fatalf("expected time.addUnix")
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
	v = evalExprCtx(t, ctx, "json.get({\"a.b\":1}, \"[\\\"a.b\\\"]\")")
	if v.K != VNum || v.N != 1 {
		t.Fatalf("expected json.get quoted key")
	}
	v = evalExprCtx(t, ctx, "json.get({\"a.b\":2}, \"['a.b']\")")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected json.get quoted key single")
	}
	v = evalExprCtx(t, ctx, "json.get({a:1}, \"missing\")")
	if v.K != VNull {
		t.Fatalf("expected json.get missing null")
	}
	v = evalExprCtx(t, ctx, "json.get({a:1}).a")
	if v.K != VNum || v.N != 1 {
		t.Fatalf("expected json.get value a=1")
	}
	v = evalExprCtx(t, ctx, "json.has({a:null}, \"a\")")
	if v.K != VBool || v.B != true {
		t.Fatalf("expected json.has true for null")
	}
	v = evalExprCtx(t, ctx, "json.has({a:1}, \"b\")")
	if v.K != VBool || v.B != false {
		t.Fatalf("expected json.has false for missing")
	}
	v = evalExprCtx(t, ctx, "json.has({\"a.b\":1}, \"[\\\"a.b\\\"]\")")
	if v.K != VBool || v.B != true {
		t.Fatalf("expected json.has quoted")
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
	v := evalExprCtx(t, ctx, "rts.text.lower(\"AbC\")")
	if v.K != VStr || v.S != "abc" {
		t.Fatalf("expected lower abc")
	}
	v = evalExprCtx(t, ctx, "rts.text.upper(\"AbC\")")
	if v.K != VStr || v.S != "ABC" {
		t.Fatalf("expected upper ABC")
	}
	v = evalExprCtx(t, ctx, "rts.text.trim(\"  a \\t\")")
	if v.K != VStr || v.S != "a" {
		t.Fatalf("expected trim a")
	}
	v = evalExprCtx(t, ctx, "rts.text.split(\"a,b\", \",\")[1]")
	if v.K != VStr || v.S != "b" {
		t.Fatalf("expected split b")
	}
	v = evalExprCtx(t, ctx, "rts.text.join([\"a\",\"b\"], \"-\")")
	if v.K != VStr || v.S != "a-b" {
		t.Fatalf("expected join a-b")
	}
	v = evalExprCtx(t, ctx, "rts.text.replace(\"a-b\", \"-\", \":\")")
	if v.K != VStr || v.S != "a:b" {
		t.Fatalf("expected replace a:b")
	}
	v = evalExprCtx(t, ctx, "rts.text.startsWith(\"hello\", \"he\")")
	if v.K != VBool || v.B != true {
		t.Fatalf("expected startsWith true")
	}
	v = evalExprCtx(t, ctx, "rts.text.endsWith(\"hello\", \"lo\")")
	if v.K != VBool || v.B != true {
		t.Fatalf("expected endsWith true")
	}
	v = evalExprCtx(t, ctx, "stdlib.text.lower(\"XY\")")
	if v.K != VStr || v.S != "xy" {
		t.Fatalf("expected stdlib alias text lower")
	}
}

func TestStdlibListDictHelpers(t *testing.T) {
	ctx := NewCtx(context.Background(), Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, "rts.list.append([1,2], 3)[2]")
	if v.K != VNum || v.N != 3 {
		t.Fatalf("expected list.append to add 3")
	}
	v = evalExprCtx(t, ctx, "len(rts.list.concat([1], [2,3]))")
	if v.K != VNum || v.N != 3 {
		t.Fatalf("expected list.concat length 3")
	}
	v = evalExprCtx(t, ctx, "rts.list.sort([3,1,2])[0]")
	if v.K != VNum || v.N != 1 {
		t.Fatalf("expected list.sort numbers")
	}
	v = evalExprCtx(t, ctx, "rts.list.sort([\"b\",\"a\"])[0]")
	if v.K != VStr || v.S != "a" {
		t.Fatalf("expected list.sort strings")
	}
	v = evalExprCtx(t, ctx, "rts.dict.keys({b:1, a:2})[0]")
	if v.K != VStr || v.S != "a" {
		t.Fatalf("expected dict.keys sorted")
	}
	v = evalExprCtx(t, ctx, "rts.dict.values({b:1, a:2})[0]")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected dict.values sorted")
	}
	v = evalExprCtx(t, ctx, "rts.dict.items({a:1})[0].key")
	if v.K != VStr || v.S != "a" {
		t.Fatalf("expected dict.items key")
	}
	v = evalExprCtx(t, ctx, "rts.dict.items({a:1})[0].value")
	if v.K != VNum || v.N != 1 {
		t.Fatalf("expected dict.items value")
	}
	v = evalExprCtx(t, ctx, "rts.dict.set({a:1}, \"b\", 2).b")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected dict.set b=2")
	}
	v = evalExprCtx(t, ctx, "rts.dict.merge({a:1}, {a:2, b:3}).a")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected dict.merge to override")
	}
	v = evalExprCtx(t, ctx, "rts.dict.remove({a:1,b:2}, \"a\").a")
	if v.K != VNull {
		t.Fatalf("expected dict.remove to drop key")
	}
}

func TestStdlibListDictExtras(t *testing.T) {
	ctx := NewCtx(context.Background(), Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	mod := "fn add1(x) { return x + 1 }\nfn even(x) { return x % 2 == 0 }\nfn gt3(x) { return x > 3 }\n"
	v := evalExprMod(t, ctx, mod, "rts.list.map([1,2], add1)[1]")
	if v.K != VNum || v.N != 3 {
		t.Fatalf("expected list.map")
	}
	v = evalExprMod(t, ctx, mod, "len(rts.list.filter([1,2,3,4], even))")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected list.filter length 2")
	}
	v = evalExprMod(t, ctx, mod, "rts.list.any([1,2,3,4], gt3)")
	if v.K != VBool || v.B != true {
		t.Fatalf("expected list.any true")
	}
	v = evalExprMod(t, ctx, mod, "rts.list.any([], gt3)")
	if v.K != VBool || v.B != false {
		t.Fatalf("expected list.any empty false")
	}
	v = evalExprMod(t, ctx, mod, "rts.list.all([2,4], even)")
	if v.K != VBool || v.B != true {
		t.Fatalf("expected list.all true")
	}
	v = evalExprMod(t, ctx, mod, "rts.list.all([], even)")
	if v.K != VBool || v.B != true {
		t.Fatalf("expected list.all empty true")
	}
	v = evalExprCtx(t, ctx, "rts.list.slice([1,2,3,4], 1, 3)[0]")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected list.slice 2")
	}
	v = evalExprCtx(t, ctx, "rts.list.slice([1,2,3,4], -2)[0]")
	if v.K != VNum || v.N != 3 {
		t.Fatalf("expected list.slice -2")
	}
	v = evalExprCtx(t, ctx, "rts.list.unique([1,1,2,1])[1]")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected list.unique")
	}
	v = evalExprCtx(t, ctx, "rts.dict.get({a:1}, \"a\")")
	if v.K != VNum || v.N != 1 {
		t.Fatalf("expected dict.get a")
	}
	v = evalExprCtx(t, ctx, "rts.dict.get({a:1}, \"b\", 2)")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected dict.get default")
	}
	v = evalExprCtx(t, ctx, "rts.dict.has({a:1}, \"a\")")
	if v.K != VBool || v.B != true {
		t.Fatalf("expected dict.has true")
	}
	v = evalExprCtx(t, ctx, "rts.dict.pick({a:1,b:2,c:3}, [\"a\",\"c\"]).b")
	if v.K != VNull {
		t.Fatalf("expected dict.pick b null")
	}
	v = evalExprCtx(t, ctx, "rts.dict.pick({a:1}, \"a\").a")
	if v.K != VNum || v.N != 1 {
		t.Fatalf("expected dict.pick string key")
	}
	v = evalExprCtx(t, ctx, "rts.dict.omit({a:1,b:2,c:3}, [\"b\",\"c\"]).a")
	if v.K != VNum || v.N != 1 {
		t.Fatalf("expected dict.omit a")
	}
	v = evalExprCtx(t, ctx, "rts.dict.omit({a:1,b:2,c:3}, [\"b\",\"c\"]).b")
	if v.K != VNull {
		t.Fatalf("expected dict.omit b null")
	}
}

func TestStdlibMathHelpers(t *testing.T) {
	ctx := NewCtx(context.Background(), Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, "rts.math.abs(-2)")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected math.abs 2")
	}
	v = evalExprCtx(t, ctx, "rts.math.min(2, -1)")
	if v.K != VNum || v.N != -1 {
		t.Fatalf("expected math.min -1")
	}
	v = evalExprCtx(t, ctx, "rts.math.max(2, -1)")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected math.max 2")
	}
	v = evalExprCtx(t, ctx, "rts.math.clamp(5, 1, 3)")
	if v.K != VNum || v.N != 3 {
		t.Fatalf("expected math.clamp high 3")
	}
	v = evalExprCtx(t, ctx, "rts.math.clamp(2, 1, 3)")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected math.clamp mid 2")
	}
	v = evalExprCtx(t, ctx, "rts.math.floor(1.8)")
	if v.K != VNum || v.N != 1 {
		t.Fatalf("expected math.floor 1")
	}
	v = evalExprCtx(t, ctx, "rts.math.ceil(1.2)")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected math.ceil 2")
	}
	v = evalExprCtx(t, ctx, "rts.math.round(1.5)")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected math.round 2")
	}
}

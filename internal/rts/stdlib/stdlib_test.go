package stdlib

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/rts"
)

func evalExprCtx(t *testing.T, ctx *rts.Ctx, src string) rts.Value {
	t.Helper()
	mod, err := rts.ParseModule("test", []byte("export let __v = "+src))
	if err != nil {
		t.Fatalf("parse mod: %v", err)
	}
	comp, err := rts.Exec(ctx, mod, New())
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	return comp.Exp["__v"]
}

func evalExprMod(t *testing.T, ctx *rts.Ctx, modSrc, expr string) rts.Value {
	t.Helper()
	mod, err := rts.ParseModule("test", []byte(modSrc+"\nexport let __v = "+expr))
	if err != nil {
		t.Fatalf("parse mod: %v", err)
	}
	comp, err := rts.Exec(ctx, mod, New())
	if err != nil {
		t.Fatalf("exec mod: %v", err)
	}
	return comp.Exp["__v"]
}

func evalExprErrCtx(t *testing.T, ctx *rts.Ctx, src string) error {
	t.Helper()
	mod, err := rts.ParseModule("test", []byte("export let __v = "+src))
	if err != nil {
		t.Fatalf("parse mod: %v", err)
	}
	if _, err := rts.Exec(ctx, mod, New()); err != nil {
		return err
	}
	t.Fatalf("expected eval error for %q", src)
	return nil
}

func TestStdlibCore(t *testing.T) {
	ctx := rts.NewCtx(t.Context(), rts.Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, "len([1,2,3])")
	if v.K != rts.VNum || v.N != 3 {
		t.Fatalf("expected 3")
	}
	v = evalExprCtx(t, ctx, "contains([\"a\",\"b\"], \"b\")")
	if v.K != rts.VBool || v.B != true {
		t.Fatalf("expected true")
	}
	v = evalExprCtx(t, ctx, "match(\"a.*\", \"abc\")")
	if v.K != rts.VBool || v.B != true {
		t.Fatalf("expected true")
	}
}

func TestContainsMatchesMembershipOperators(t *testing.T) {
	tests := []struct {
		name     string
		operator string
		call     string
	}{
		{name: "list", operator: `2 in [1, 2]`, call: `contains([1, 2], 2)`},
		{name: "string", operator: `"ell" in "hello"`, call: `contains("hello", "ell")`},
		{name: "dict", operator: `"a" in {a: 1}`, call: `contains({a: 1}, "a")`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := rts.NewCtx(t.Context(), rts.Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
			op := evalExprCtx(t, ctx, tt.operator)
			call := evalExprCtx(t, ctx, tt.call)
			if op.K != rts.VBool || call.K != rts.VBool || op.B != call.B {
				t.Fatalf("operator=%+v call=%+v, want matching bools", op, call)
			}
		})
	}
}

func TestContainsListValueIsNotStringified(t *testing.T) {
	mod := `fn f() { return 1 }`
	for _, expr := range []string{
		`f in [f]`,
		`contains([f], f)`,
		`rts.contains([f], f)`,
		`stdlib.contains([f], f)`,
	} {
		t.Run(expr, func(t *testing.T) {
			ctx := rts.NewCtx(t.Context(), rts.Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
			v := evalExprMod(t, ctx, mod, expr)
			if v.K != rts.VBool || v.B {
				t.Fatalf("%s = %+v, want false", expr, v)
			}
		})
	}
}

func TestContainsAndMembershipRejectSameContainer(t *testing.T) {
	for _, expr := range []string{`1 in 2`, `contains(2, 1)`} {
		t.Run(expr, func(t *testing.T) {
			ctx := rts.NewCtx(t.Context(), rts.Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
			err := evalExprErrCtx(t, ctx, expr)
			if !strings.Contains(err.Error(), "membership container must be string, list, or dict") {
				t.Fatalf("error = %v, want unsupported membership container", err)
			}
		})
	}
}

// the default(a, b) helper was removed in favour of ??. Dropping it from
// coreSpec has to clear all three spellings at once, because the prelude and
// both namespace objects are built from that one map
func TestStdlibDoesNotExposeDefault(t *testing.T) {
	env := New()
	if _, ok := env["default"]; ok {
		t.Fatalf("default is still in the top level prelude")
	}
	for _, ns := range []string{"rts", "stdlib"} {
		v, ok := env[ns]
		if !ok {
			t.Fatalf("missing %s namespace", ns)
		}
		if v.K != rts.VObj {
			t.Fatalf("%s: expected an object, got kind %v", ns, v.K)
		}
		if _, ok, err := v.O.Member(nil, rts.Pos{}, "default"); err != nil || ok {
			t.Fatalf("%s.default is still exposed", ns)
		}
		if _, ok, err := v.O.Member(nil, rts.Pos{}, "str"); err != nil || !ok {
			t.Fatalf("%s.str went missing, the namespace lookup is not proving anything", ns)
		}
	}
}

func TestStdlibCrypto(t *testing.T) {
	ctx := rts.NewCtx(context.Background(), rts.Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, "crypto.sha256(\"abc\")")
	if v.K != rts.VStr ||
		v.S != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("expected sha256")
	}
	v = evalExprCtx(t, ctx, "crypto.hmacSha256(\"key\", \"data\")")
	if v.K != rts.VStr ||
		v.S != "5031fe3d989c6d1537a013fa6e739da23463fdaec3b70137d828e36ace221bd0" {
		t.Fatalf("expected hmac sha256")
	}
	v = evalExprCtx(t, ctx, "crypto.sha512(\"abc\")")
	if v.K != rts.VStr ||
		v.S != "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f" {
		t.Fatalf("expected sha512")
	}
	v = evalExprCtx(t, ctx, "crypto.hmacSha512(\"key\", \"data\")")
	if v.K != rts.VStr ||
		v.S != "3c5953a18f7303ec653ba170ae334fafa08e3846f2efe317b87efce82376253cb52a8c31ddcde5a3a2eee183c2b34cb91f85e64ddbc325f7692b199473579c58" {
		t.Fatalf("expected hmac sha512")
	}
}

func TestStdlibEncoding(t *testing.T) {
	ctx := rts.NewCtx(context.Background(), rts.Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, "encoding.hex.encode(\"hi\")")
	if v.K != rts.VStr || v.S != "6869" {
		t.Fatalf("expected hex encode")
	}
	v = evalExprCtx(t, ctx, "encoding.hex.decode(\"6869\")")
	if v.K != rts.VStr || v.S != "hi" {
		t.Fatalf("expected hex decode")
	}
	v = evalExprCtx(t, ctx, "encoding.base64url.encode(\"hi\")")
	if v.K != rts.VStr || v.S != "aGk" {
		t.Fatalf("expected base64url encode")
	}
	v = evalExprCtx(t, ctx, "encoding.base64url.decode(\"aGk=\")")
	if v.K != rts.VStr || v.S != "hi" {
		t.Fatalf("expected base64url decode")
	}
}

func TestStdlibTypes(t *testing.T) {
	ctx := rts.NewCtx(context.Background(), rts.Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, "num(\"1.5\")")
	if v.K != rts.VNum || v.N != 1.5 {
		t.Fatalf("expected num 1.5")
	}
	v = evalExprCtx(t, ctx, "num(true)")
	if v.K != rts.VNum || v.N != 1 {
		t.Fatalf("expected num true")
	}
	v = evalExprCtx(t, ctx, "num(\"bad\", 2)")
	if v.K != rts.VNum || v.N != 2 {
		t.Fatalf("expected num default")
	}
	v = evalExprCtx(t, ctx, "int(3.0)")
	if v.K != rts.VNum || v.N != 3 {
		t.Fatalf("expected int 3")
	}
	v = evalExprCtx(t, ctx, "int(\"7\")")
	if v.K != rts.VNum || v.N != 7 {
		t.Fatalf("expected int 7")
	}
	v = evalExprCtx(t, ctx, "int(3.2, 4)")
	if v.K != rts.VNum || v.N != 4 {
		t.Fatalf("expected int default")
	}
	v = evalExprCtx(t, ctx, "bool(\"true\")")
	if v.K != rts.VBool || v.B != true {
		t.Fatalf("expected bool true")
	}
	v = evalExprCtx(t, ctx, "bool(\"0\")")
	if v.K != rts.VBool || v.B != false {
		t.Fatalf("expected bool false")
	}
	v = evalExprCtx(t, ctx, "bool(\"\", true)")
	if v.K != rts.VBool || v.B != true {
		t.Fatalf("expected bool default")
	}
	v = evalExprCtx(t, ctx, "typeof(rts)")
	if v.K != rts.VStr || v.S != "rts" {
		t.Fatalf("expected typeof rts")
	}
	v = evalExprCtx(t, ctx, "typeof(stdlib)")
	if v.K != rts.VStr || v.S != "stdlib" {
		t.Fatalf("expected typeof stdlib alias")
	}
}

func TestStdlibTimeExtras(t *testing.T) {
	ctx := rts.NewCtx(context.Background(), rts.Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	base := time.Date(2024, 1, 2, 3, 4, 5, 600*int(time.Millisecond), time.UTC)
	ctx.Now = func() time.Time { return base }

	v := evalExprCtx(t, ctx, "time.nowUnix()")
	if v.K != rts.VNum || v.N != float64(base.Unix()) {
		t.Fatalf("expected nowUnix")
	}
	v = evalExprCtx(t, ctx, "time.nowUnixString()")
	if v.K != rts.VStr || v.S != "1704164645" {
		t.Fatalf("expected nowUnixString")
	}
	v = evalExprCtx(t, ctx, "\"\" + time.nowUnixString()")
	if v.K != rts.VStr || v.S != "1704164645" {
		t.Fatalf("expected nowUnixString concat")
	}
	v = evalExprCtx(t, ctx, "time.nowUnixMs()")
	ms := base.UnixNano() / int64(time.Millisecond)
	if v.K != rts.VNum || v.N != float64(ms) {
		t.Fatalf("expected nowUnixMs")
	}
	v = evalExprCtx(t, ctx, "time.format(\"2006-01-02\")")
	if v.K != rts.VStr || v.S != "2024-01-02" {
		t.Fatalf("expected time.format")
	}
	v = evalExprCtx(t, ctx, "time.parse(\"2006-01-02T15:04:05Z07:00\", \"1970-01-01T00:00:01Z\")")
	if v.K != rts.VNum || v.N != 1 {
		t.Fatalf("expected time.parse unix")
	}
	v = evalExprCtx(
		t,
		ctx,
		"time.parse(\"2006-01-02T15:04:05.000Z07:00\", \"1970-01-01T00:00:01.250Z\")",
	)
	if v.K != rts.VNum || v.N != 1.25 {
		t.Fatalf("expected time.parse fractional")
	}
	v = evalExprCtx(t, ctx, "time.formatUnix(0, \"2006-01-02\")")
	if v.K != rts.VStr || v.S != "1970-01-01" {
		t.Fatalf("expected time.formatUnix")
	}
	v = evalExprCtx(t, ctx, "time.formatUnix(1.25, \"2006-01-02T15:04:05.000Z07:00\")")
	if v.K != rts.VStr || v.S != "1970-01-01T00:00:01.250Z" {
		t.Fatalf("expected time.formatUnix fractional")
	}
	v = evalExprCtx(t, ctx, "time.addUnix(1.5, 2.25)")
	if v.K != rts.VNum || v.N != 3.75 {
		t.Fatalf("expected time.addUnix")
	}
	v = evalExprCtx(t, ctx, "time.addUnix(1.5, \"2.25s\")")
	if v.K != rts.VNum || v.N != 3.75 {
		t.Fatalf("expected time.addUnix duration string")
	}
	v = evalExprCtx(t, ctx, "time.duration(\"1h30m\")")
	if v.K != rts.VNum || v.N != 5400 {
		t.Fatalf("expected time.duration")
	}
}

func TestStdlibJSONFile(t *testing.T) {
	ctx := rts.NewCtx(context.Background(), rts.Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	ctx.ReadFile = func(path string) ([]byte, error) {
		return []byte("[{\"id\":1}]"), nil
	}
	v := evalExprCtx(t, ctx, "json.file(\"x.json\")")
	if v.K != rts.VList || len(v.L) != 1 {
		t.Fatalf("expected list")
	}
	if v.L[0].K != rts.VDict {
		t.Fatalf("expected dict")
	}
}

func TestStdlibJSONParseStringify(t *testing.T) {
	ctx := rts.NewCtx(context.Background(), rts.Limits{MaxStr: 4096, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, "json.parse(\"{\\\"a\\\":1}\").a")
	if v.K != rts.VNum || v.N != 1 {
		t.Fatalf("expected 1")
	}
	v = evalExprCtx(t, ctx, "json.stringify({a:1})")
	if v.K != rts.VStr || v.S != "{\"a\":1}" {
		t.Fatalf("unexpected json: %q", v.S)
	}
	v = evalExprCtx(t, ctx, "json.stringify({a:1}, 2)")
	if v.K != rts.VStr || !strings.Contains(v.S, "\n") || !strings.Contains(v.S, "\"a\": 1") {
		t.Fatalf("expected indented json")
	}
}

func marshalStringify(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (string, error) {
	raw, err := jsonIface(ctx, pos, args[0])
	if err != nil {
		return "", err
	}
	var indent string
	if len(args) > 1 {
		if indent, err = jsonIndent(ctx, pos, args[1]); err != nil {
			return "", err
		}
	}
	var data []byte
	if indent == "" {
		data, err = json.Marshal(raw)
	} else {
		data, err = json.MarshalIndent(raw, "", indent)
	}
	if err != nil {
		return "", rts.Errf(ctx, pos, "json stringify failed")
	}
	if err := rts.CheckStr(ctx, pos, string(data)); err != nil {
		return "", err
	}
	return string(data), nil
}

func TestStdlibJSONStringifyMatchesMarshal(t *testing.T) {
	ctx := rts.NewCtx(t.Context(), rts.Limits{MaxStr: 64, MaxList: 4, MaxDict: 4})
	pos := rts.Pos{Path: "test", Line: 1, Col: 1}
	ok := evalExprCtx(t, ctx, `try [1, {a: "<b>"}]`)
	failed := evalExprCtx(t, ctx, "try (1 / 0)")
	nested := rts.Dict(map[string]rts.Value{
		"b":  rts.List([]rts.Value{rts.Num(1.5), rts.Dict(nil), rts.List(nil), rts.Null()}),
		"a<": rts.Dict(map[string]rts.Value{"z": rts.Bool(false), "y": rts.Str("é")}),
	})
	values := []rts.Value{
		rts.Null(),
		rts.Bool(true),
		rts.Num(1e21),
		rts.Num(1e-7),
		rts.Num(math.Copysign(0, -1)),
		rts.Num(9007199254740993),
		rts.Str("<a&b> \u2028\u2029 \xff \"q\" \\ \x00\n\t"),
		rts.List(nil),
		rts.Dict(nil),
		nested,
		ok,
		failed,
		rts.List([]rts.Value{ok, failed}),
		rts.Num(math.NaN()),
		rts.List([]rts.Value{rts.Num(math.Inf(1))}),
		rts.List([]rts.Value{rts.Fn(&rts.Func{})}),
		rts.Dict(map[string]rts.Value{"a": rts.Native(nil)}),
		rts.List([]rts.Value{rts.Obj(&objMap{name: "x"})}),
		rts.List(make([]rts.Value, 5)),
		rts.Dict(map[string]rts.Value{"a": {}, "b": {}, "c": {}, "d": {}, "e": {}}),
		rts.Str(strings.Repeat("a", 62)),
		rts.Str(strings.Repeat("a", 63)),
		rts.List([]rts.Value{rts.Str(strings.Repeat("a", 40))}),
	}
	indents := [][]rts.Value{
		nil,
		{rts.Num(0)},
		{rts.Num(2)},
		{rts.Num(1)},
		{rts.Num(32)},
		{rts.Str("\t")},
		{rts.Str("")},
		{rts.Str("ab")},
		{rts.Str(" \t")},
	}
	for i, v := range values {
		for j, indent := range indents {
			args := append([]rts.Value{v}, indent...)
			want, werr := marshalStringify(ctx, pos, args)
			got, err := jsonStringify(ctx, pos, args)
			if fmt.Sprint(err) != fmt.Sprint(werr) || err == nil && got.S != want {
				t.Fatalf("value %d, indent %d: json.stringify = %q, %v\nwant %q, %v", i, j, got.S, err, want, werr)
			}
		}
	}
	for i, indent := range []rts.Value{rts.Num(-1), rts.Num(33), rts.Bool(true)} {
		args := []rts.Value{nested, indent}
		_, werr := marshalStringify(ctx, pos, args)
		if _, err := jsonStringify(ctx, pos, args); err == nil || err.Error() != werr.Error() {
			t.Fatalf("bad indent %d: json.stringify error = %v, want %v", i, err, werr)
		}
	}
}

// Shared children expand to 2^60 lists unless encoding stops at the size limit.
func TestStdlibJSONStringifyStopsAtStringLimit(t *testing.T) {
	v := rts.List([]rts.Value{rts.Num(0)})
	for range 60 {
		v = rts.List([]rts.Value{v, v})
	}
	ctx := rts.NewCtx(t.Context(), rts.Limits{MaxStr: 65536, MaxList: 2000, MaxDict: 2000})
	for _, args := range [][]rts.Value{{v}, {v, rts.Num(2)}} {
		_, err := jsonStringify(ctx, rts.Pos{}, args)
		if err == nil || !strings.Contains(err.Error(), "string too long") {
			t.Fatalf("json.stringify error = %v, want string too long", err)
		}
	}
}

// Indentation grows quadratically with depth; it must count toward the size limit.
func TestStdlibJSONStringifyStopsDeepIndentAtStringLimit(t *testing.T) {
	ctx := rts.NewCtx(t.Context(), rts.Limits{MaxStr: 65536, MaxList: 2000, MaxDict: 2000})
	for _, depth := range []int{9998, 25000} {
		v := rts.List(nil)
		for range depth {
			v = rts.List([]rts.Value{v})
		}
		_, err := jsonStringify(ctx, rts.Pos{}, []rts.Value{v, rts.Num(32)})
		if err == nil || !strings.Contains(err.Error(), "string too long") {
			t.Fatalf("depth %d: json.stringify error = %v, want string too long", depth, err)
		}
	}
}

func TestStdlibJSONGet(t *testing.T) {
	ctx := rts.NewCtx(context.Background(), rts.Limits{MaxStr: 4096, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, "json.get({a:{b:[1,2]}}, \"a.b[1]\")")
	if v.K != rts.VNum || v.N != 2 {
		t.Fatalf("expected json.get path 2")
	}
	v = evalExprCtx(t, ctx, "json.get({a:{b:[1,2]}}, \"$.a.b[0]\")")
	if v.K != rts.VNum || v.N != 1 {
		t.Fatalf("expected json.get $ path 1")
	}
	v = evalExprCtx(t, ctx, "json.get({\"a.b\":1}, \"[\\\"a.b\\\"]\")")
	if v.K != rts.VNum || v.N != 1 {
		t.Fatalf("expected json.get quoted key")
	}
	v = evalExprCtx(t, ctx, "json.get({\"a.b\":2}, \"['a.b']\")")
	if v.K != rts.VNum || v.N != 2 {
		t.Fatalf("expected json.get quoted key single")
	}
	v = evalExprCtx(t, ctx, "json.get({a:1}, \"missing\")")
	if v.K != rts.VNull {
		t.Fatalf("expected json.get missing null")
	}
	v = evalExprCtx(t, ctx, "json.get({a:1}).a")
	if v.K != rts.VNum || v.N != 1 {
		t.Fatalf("expected json.get value a=1")
	}
	v = evalExprCtx(t, ctx, "json.has({a:null}, \"a\")")
	if v.K != rts.VBool || v.B != true {
		t.Fatalf("expected json.has true for null")
	}
	v = evalExprCtx(t, ctx, "json.has({a:1}, \"b\")")
	if v.K != rts.VBool || v.B != false {
		t.Fatalf("expected json.has false for missing")
	}
	v = evalExprCtx(t, ctx, "json.has({\"a.b\":1}, \"[\\\"a.b\\\"]\")")
	if v.K != rts.VBool || v.B != true {
		t.Fatalf("expected json.has quoted")
	}
}

func TestStdlibJSONEqual(t *testing.T) {
	ctx := testCtx()
	tests := []struct {
		src  string
		want bool
	}{
		{"json.equal(null, null)", true},
		{"json.equal(true, true)", true},
		{"json.equal(true, false)", false},
		{"json.equal(null, false)", false},
		{"json.equal(1, 1.0)", true},
		{"json.equal(1, 2)", false},
		{`json.equal("a", "a")`, true},
		{`json.equal(1, "1")`, false},
		{"json.equal([1, [2, {a: 3}]], [1, [2, {a: 3}]])", true},
		{"json.equal([1, 2], [2, 1])", false},
		{"json.equal([1, 2], [1, 2, 3])", false},
		{"json.equal({a: {b: [1]}}, {a: {b: [1]}})", true},
		{"json.equal({a: {b: [1]}}, {a: {b: [2]}})", false},
		{"json.equal({a: 1, b: 2}, {b: 2, a: 1})", true},
		{"json.equal({a: 1}, {a: 1, b: 2})", false},
		{"json.equal({a: null}, {})", false},
		{"json.equal({a: null}, {b: null})", false},
		{"json.equal([], {})", false},
		{"json.equal([], [])", true},
		{"json.equal({}, {})", true},
		{"rts.json.equal([1], [1])", true},
		{"stdlib.json.equal([1], [1])", true},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			v := evalExprCtx(t, ctx, tt.src)
			if v.K != rts.VBool || v.B != tt.want {
				t.Fatalf("%s = %+v, want %t", tt.src, v, tt.want)
			}
		})
	}
}

func TestStdlibJSONDiff(t *testing.T) {
	ctx := testCtx()
	diffs := func(n int) string {
		return `json.diff(rts.text.split("` + strings.Repeat("x,", n-1) + `x", ","), [])`
	}
	unexpected := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf(`$[%d]: unexpected "x"`, i)
		}
		return out
	}
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{"equal", `json.diff({a: [1, {b: null}], c: "x"}, {c: "x", a: [1, {b: null}]})`, []string{}},
		{"value", "json.diff({user: {id: 7}}, {user: {id: 8}})", []string{"$.user.id: got 7, want 8"}},
		{"kind", `json.diff({a: "1"}, {a: 1})`, []string{`$.a: got "1", want 1`}},
		{"missing", `json.diff({user: {}}, {user: {email: "a@b.c"}})`, []string{`$.user.email: missing, want "a@b.c"`}},
		{"unexpected", "json.diff({user: {extra: true}}, {user: {}})", []string{"$.user.extra: unexpected true"}},
		{"null is not missing", "json.diff({a: null}, {})", []string{"$.a: unexpected null"}},
		{
			"got shorter",
			"json.diff({items: [1]}, {items: [1, {id: 2}, 3]})",
			[]string{`$.items[1]: missing, want {"id":2}`, "$.items[2]: missing, want 3"},
		},
		{"got longer", "json.diff([1, 2, [3]], [1])", []string{"$[1]: unexpected 2", "$[2]: unexpected [3]"}},
		{
			"nested",
			`json.diff({items: [{tags: ["a"]}]}, {items: [{tags: ["b"]}]})`,
			[]string{`$.items[0].tags[0]: got "a", want "b"`},
		},
		{
			"quoted keys",
			`json.diff({"display.name": 1, "0": 2, "a b": {"x-y": 3}}, {"display.name": 2, "0": 2, "a b": {"x-y": 4}})`,
			[]string{`$["a b"]["x-y"]: got 3, want 4`, `$["display.name"]: got 1, want 2`},
		},
		{"root kind", "json.diff([], {})", []string{"$: got [], want {}"}},
		{"root collection", "json.diff([1, 2], {a: [1]})", []string{`$: got [1,2], want {"a":[1]}`}},
		{"root value", `json.diff("200", 200)`, []string{`$: got "200", want 200`}},
		{"no html escape", `json.diff("<i> & é", 1)`, []string{`$: got "<i> & é", want 1`}},
		{"compact", "json.diff({b: [1, 2.5, null]}, 1)", []string{`$: got {"b":[1,2.5,null]}, want 1`}},
		{"nested kind", "json.diff({a: {b: 1}}, {a: [1]})", []string{`$.a: got {"b":1}, want [1]`}},
		{
			"order",
			"json.diff({b: 1, a: [1, 2], c: {z: 1, y: 2}}, {c: {z: 2, y: 3}, b: 2, a: [1, 3], d: 0})",
			[]string{
				"$.a[1]: got 2, want 3",
				"$.b: got 1, want 2",
				"$.c.y: got 2, want 3",
				"$.c.z: got 1, want 2",
				"$.d: missing, want 0",
			},
		},
		{
			"fits",
			`json.diff("` + strings.Repeat("a", 118) + `", 1)`,
			[]string{`$: got "` + strings.Repeat("a", 118) + `", want 1`},
		},
		{
			"cut",
			`json.diff("` + strings.Repeat("a", 119) + `", 1)`,
			[]string{`$: got "` + strings.Repeat("a", 119) + `..., want 1`},
		},
		{
			"cut at rune",
			`json.diff(1, "` + strings.Repeat("é", 100) + `")`,
			[]string{`$: got 1, want "` + strings.Repeat("é", 59) + "..."},
		},
		{"cap", diffs(100), unexpected(100)},
		{"one more", diffs(101), append(unexpected(100), "and 1 more difference")},
		{"more", diffs(150), append(unexpected(100), "and 50 more differences")},
		{"rts alias", "rts.json.diff(1, 2)", []string{"$: got 1, want 2"}},
		{"stdlib alias", "stdlib.json.diff(1, 2)", []string{"$: got 1, want 2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for range 8 {
				v := evalExprCtx(t, ctx, tt.src)
				if got := strList(t, v); !slices.Equal(got, tt.want) {
					t.Fatalf("%s =\n%s\nwant\n%s", tt.src, strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
				}
			}
		})
	}
}

func TestStdlibJSONDiffPathsWorkWithGet(t *testing.T) {
	ctx := testCtx()
	pos := rts.Pos{Path: "test", Line: 1, Col: 1}
	for _, key := range []string{"id", "0", "", "display.name", "a b", `say "hi"`, `back\slash`, "line\nbreak", "café"} {
		got := rts.Dict(map[string]rts.Value{
			"x": rts.List([]rts.Value{rts.Null(), rts.Dict(map[string]rts.Value{key: rts.Num(1)})}),
		})
		want := rts.Dict(map[string]rts.Value{
			"x": rts.List([]rts.Value{rts.Null(), rts.Dict(map[string]rts.Value{key: rts.Num(2)})}),
		})
		v, err := jsonDiff(ctx, pos, []rts.Value{got, want})
		if err != nil {
			t.Fatalf("json.diff with key %q: %v", key, err)
		}
		msgs := strList(t, v)
		if len(msgs) != 1 || !strings.HasSuffix(msgs[0], ": got 1, want 2") {
			t.Fatalf("json.diff with key %q = %q", key, msgs)
		}
		path := strings.TrimSuffix(msgs[0], ": got 1, want 2")
		v, err = jsonGet(ctx, pos, []rts.Value{got, rts.Str(path)})
		if err != nil || v.K != rts.VNum || v.N != 1 {
			t.Fatalf("json.get(%s) = %+v, %v, want 1", path, v, err)
		}
	}
}

func TestStdlibJSONEqualAndDiffRejectNonJSON(t *testing.T) {
	ctx := testCtx()
	pos := rts.Pos{Path: "test", Line: 1, Col: 1}
	tests := []struct {
		src  string
		want string
	}{
		{"json.equal(f, 1)", "json.equal(a, b) cannot compare function"},
		{"json.equal(1, [{a: [f]}])", "json.equal(a, b) cannot compare function"},
		{"json.equal([1], [1, f])", "json.equal(a, b) cannot compare function"},
		{"json.equal(json.get, json.get)", "json.equal(a, b) cannot compare native"},
		{"json.equal({a: rts}, {a: rts})", "json.equal(a, b) cannot compare rts"},
		{"json.equal({b: try 1, a: f}, 1)", "json.equal(a, b) cannot compare function"},
		{"json.diff([try 1], [])", "json.diff(got, want) cannot compare result"},
		{"json.diff({a: 1}, {b: f})", "json.diff(got, want) cannot compare function"},
		{"json.equal(1)", "json.equal(a, b) expects 2 args"},
		{"json.equal(1, 2, 3)", "json.equal(a, b) expects 2 args"},
		{"json.diff()", "json.diff(got, want) expects 2 args"},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			mod, err := rts.ParseModule("test", []byte("fn f() {\n  return 1\n}\nexport let __v = "+tt.src))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			for range 8 {
				if _, err := rts.Exec(ctx, mod, New()); err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("%s: error = %v, want %q", tt.src, err, tt.want)
				}
			}
		})
	}

	inf := rts.Num(math.Inf(1))
	if _, err := jsonEqual(ctx, pos, []rts.Value{inf, inf}); err == nil ||
		!strings.Contains(err.Error(), "json.equal(a, b) cannot compare +Inf") {
		t.Fatalf("json.equal(+Inf, +Inf): error = %v", err)
	}
	nan := rts.List([]rts.Value{rts.Num(math.NaN())})
	if _, err := jsonDiff(ctx, pos, []rts.Value{rts.Num(1), nan}); err == nil ||
		!strings.Contains(err.Error(), "json.diff(got, want) cannot compare NaN") {
		t.Fatalf("json.diff(1, [NaN]): error = %v", err)
	}
}

// Shared children expand to 2^27 lists; validation must stop at the step limit.
func TestStdlibJSONEqualAndDiffStopAtStepLimit(t *testing.T) {
	const build = `
let v = [0]
for let i = 0; i < 27; i = i + 1 {
  v = [v, v]
}
`
	for _, expr := range []string{"json.equal(v, v)", "try json.equal(v, v)", "json.diff(v, v)", "try json.diff(v, v)"} {
		mod, err := rts.ParseModule("test", []byte(build+"export let __v = "+expr))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		ctx := rts.NewCtx(t.Context(), rts.Limits{MaxSteps: 10000, MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
		_, err = rts.Exec(ctx, mod, New())
		if !rts.IsAbort(err) || !strings.Contains(err.Error(), "step limit exceeded") {
			t.Fatalf("%s error = %v, want the step limit abort", expr, err)
		}
	}
}

// Three containers in each argument use six steps, even when both arguments share v.
func TestStdlibJSONEqualAndDiffStepCost(t *testing.T) {
	const steps = 6
	v := rts.List([]rts.Value{rts.Dict(map[string]rts.Value{"a": rts.List(nil)}), rts.Num(1)})
	for name, f := range map[string]rts.NativeFunc{"json.equal": jsonEqual, "json.diff": jsonDiff} {
		for _, limit := range []int{steps, steps - 1} {
			ctx := rts.NewCtx(t.Context(), rts.Limits{MaxSteps: limit, MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
			_, err := f(ctx, rts.Pos{}, []rts.Value{v, v})
			if rts.IsAbort(err) != (limit < steps) {
				t.Fatalf("%s with %d steps: error = %v", name, limit, err)
			}
		}
	}
}

func TestStdlibJSONEqualAndDiffStopWhenCanceled(t *testing.T) {
	cx, cancel := context.WithCancel(t.Context())
	cancel()
	ctx := rts.NewCtx(cx, rts.Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	v := rts.List([]rts.Value{rts.Num(1)})
	for name, f := range map[string]rts.NativeFunc{"json.equal": jsonEqual, "json.diff": jsonDiff} {
		_, err := f(ctx, rts.Pos{}, []rts.Value{v, v})
		if !rts.IsAbort(err) || !strings.Contains(err.Error(), "canceled: context canceled") {
			t.Fatalf("%s error = %v, want the canceled abort", name, err)
		}
	}
}

func TestStdlibJSONEqualAndDiffFitResponseBudget(t *testing.T) {
	for _, nested := range []bool{false, true} {
		resp := func() rts.Value {
			items := make([]rts.Value, 2000)
			for i := range items {
				item := map[string]rts.Value{}
				for f := range 10 {
					item["field"+strconv.Itoa(f)] = rts.Num(float64(i*10 + f))
				}
				if nested {
					item["field9"] = rts.Dict(map[string]rts.Value{"id": rts.Num(float64(i))})
				}
				items[i] = rts.Dict(item)
			}
			return rts.List(items)
		}
		want := resp()
		want.L[1500].M["field3"] = rts.Str("changed")
		cfg := rts.EvalConfig{Bindings: []rts.Extensions{
			rts.Extension("got", resp()),
			rts.Extension("same", resp()),
			rts.Extension("want", want),
		}}
		eng := rts.NewEng(New)
		v, err := eng.Eval(t.Context(), cfg, "json.equal(got, same)", rts.Pos{})
		if err != nil || v.K != rts.VBool || !v.B {
			t.Fatalf("nested %t: json.equal = %+v, %v, want true", nested, v, err)
		}
		v, err = eng.Eval(t.Context(), cfg, "json.diff(got, want)", rts.Pos{})
		if err != nil {
			t.Fatalf("nested %t: json.diff: %v", nested, err)
		}
		if got := strList(t, v); !slices.Equal(got, []string{`$[1500].field3: got 15003, want "changed"`}) {
			t.Fatalf("nested %t: json.diff = %q", nested, got)
		}
	}
}

// Shared children expand to 2^60 dicts unless encoding stops at the display limit.
func TestStdlibJSONDiffTextStopsAtCut(t *testing.T) {
	v := rts.Num(0)
	for range 60 {
		v = rts.Dict(map[string]rts.Value{"a": v, "b": v})
	}
	const key = `{"a":`
	if got, want := diffText(v), strings.Repeat(key, maxDiffText/len(key))+"..."; got != want {
		t.Fatalf("diffText = %q, want %q", got, want)
	}
}

func strList(t *testing.T, v rts.Value) []string {
	t.Helper()
	if v.K != rts.VList {
		t.Fatalf("got %+v, want a list", v)
	}
	out := make([]string, len(v.L))
	for i, it := range v.L {
		if it.K != rts.VStr {
			t.Fatalf("item %d is %+v, want a string", i, it)
		}
		out[i] = it.S
	}
	return out
}

func TestStdlibHeadersHelpers(t *testing.T) {
	ctx := rts.NewCtx(context.Background(), rts.Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, `headers.get(headers.normalize({"X-Test":"ok"}), "x-test")`)
	if v.K != rts.VStr || v.S != "ok" {
		t.Fatalf("expected header value")
	}
	v = evalExprCtx(t, ctx, `len(headers.merge({"a":"1","b":"2"}, {"b":null}))`)
	if v.K != rts.VNum || v.N != 1 {
		t.Fatalf("expected merged headers length 1")
	}
	v = evalExprCtx(t, ctx, "headers.has({\"A\": [\"1\",\"2\"]}, \"a\")")
	if v.K != rts.VBool || v.B != true {
		t.Fatalf("expected headers.has true")
	}
}

func TestStdlibQueryHelpers(t *testing.T) {
	ctx := rts.NewCtx(context.Background(), rts.Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, `len(query.fromURL("https://x.test?p=1&p=2").p)`)
	if v.K != rts.VNum || v.N != 2 {
		t.Fatalf("expected 2 query values")
	}
	v = evalExprCtx(t, ctx, `query.parse(query.encode({a:"1", b:["x","y"]})).a`)
	if v.K != rts.VStr || v.S != "1" {
		t.Fatalf("expected query value")
	}
	v = evalExprCtx(t, ctx, `len(query.fromURL(query.merge("https://x.test?p=1&q=2", {q:[]})))`)
	if v.K != rts.VNum || v.N != 1 {
		t.Fatalf("expected query length 1")
	}
	v = evalExprCtx(t, ctx, `query.merge("{{host}}/path?keep=1", {q: "x"})`)
	if v.K != rts.VStr {
		t.Fatalf("expected query.merge to return string")
	}
	if !strings.Contains(v.S, "{{host}}") {
		t.Fatalf("expected templated host preserved, got %q", v.S)
	}
	if strings.Contains(v.S, "%7B%7B") || strings.Contains(v.S, "%7D%7D") {
		t.Fatalf("expected template braces to remain unescaped, got %q", v.S)
	}
	if !strings.Contains(v.S, "keep=1") || !strings.Contains(v.S, "q=x") {
		t.Fatalf("expected merged query params, got %q", v.S)
	}
}

func TestStdlibTextHelpers(t *testing.T) {
	ctx := rts.NewCtx(context.Background(), rts.Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, "rts.text.lower(\"AbC\")")
	if v.K != rts.VStr || v.S != "abc" {
		t.Fatalf("expected lower abc")
	}
	v = evalExprCtx(t, ctx, "rts.text.upper(\"AbC\")")
	if v.K != rts.VStr || v.S != "ABC" {
		t.Fatalf("expected upper ABC")
	}
	v = evalExprCtx(t, ctx, "rts.text.trim(\"  a \\t\")")
	if v.K != rts.VStr || v.S != "a" {
		t.Fatalf("expected trim a")
	}
	v = evalExprCtx(t, ctx, "rts.text.split(\"a,b\", \",\")[1]")
	if v.K != rts.VStr || v.S != "b" {
		t.Fatalf("expected split b")
	}
	v = evalExprCtx(t, ctx, "rts.text.join([\"a\",\"b\"], \"-\")")
	if v.K != rts.VStr || v.S != "a-b" {
		t.Fatalf("expected join a-b")
	}
	v = evalExprCtx(t, ctx, "rts.text.replace(\"a-b\", \"-\", \":\")")
	if v.K != rts.VStr || v.S != "a:b" {
		t.Fatalf("expected replace a:b")
	}
	v = evalExprCtx(t, ctx, "rts.text.startsWith(\"hello\", \"he\")")
	if v.K != rts.VBool || v.B != true {
		t.Fatalf("expected startsWith true")
	}
	v = evalExprCtx(t, ctx, "rts.text.endsWith(\"hello\", \"lo\")")
	if v.K != rts.VBool || v.B != true {
		t.Fatalf("expected endsWith true")
	}
	v = evalExprCtx(t, ctx, "stdlib.text.lower(\"XY\")")
	if v.K != rts.VStr || v.S != "xy" {
		t.Fatalf("expected stdlib alias text lower")
	}
}

func TestStdlibListDictHelpers(t *testing.T) {
	ctx := rts.NewCtx(context.Background(), rts.Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, "rts.list.append([1,2], 3)[2]")
	if v.K != rts.VNum || v.N != 3 {
		t.Fatalf("expected list.append to add 3")
	}
	v = evalExprCtx(t, ctx, "len(rts.list.concat([1], [2,3]))")
	if v.K != rts.VNum || v.N != 3 {
		t.Fatalf("expected list.concat length 3")
	}
	v = evalExprCtx(t, ctx, "rts.list.sort([3,1,2])[0]")
	if v.K != rts.VNum || v.N != 1 {
		t.Fatalf("expected list.sort numbers")
	}
	v = evalExprCtx(t, ctx, "rts.list.sort([\"b\",\"a\"])[0]")
	if v.K != rts.VStr || v.S != "a" {
		t.Fatalf("expected list.sort strings")
	}
	v = evalExprCtx(t, ctx, "rts.dict.keys({b:1, a:2})[0]")
	if v.K != rts.VStr || v.S != "a" {
		t.Fatalf("expected dict.keys sorted")
	}
	v = evalExprCtx(t, ctx, "rts.dict.values({b:1, a:2})[0]")
	if v.K != rts.VNum || v.N != 2 {
		t.Fatalf("expected dict.values sorted")
	}
	v = evalExprCtx(t, ctx, "rts.dict.items({a:1})[0].key")
	if v.K != rts.VStr || v.S != "a" {
		t.Fatalf("expected dict.items key")
	}
	v = evalExprCtx(t, ctx, "rts.dict.items({a:1})[0].value")
	if v.K != rts.VNum || v.N != 1 {
		t.Fatalf("expected dict.items value")
	}
	v = evalExprCtx(t, ctx, "rts.dict.set({a:1}, \"b\", 2).b")
	if v.K != rts.VNum || v.N != 2 {
		t.Fatalf("expected dict.set b=2")
	}
	v = evalExprCtx(t, ctx, "rts.dict.merge({a:1}, {a:2, b:3}).a")
	if v.K != rts.VNum || v.N != 2 {
		t.Fatalf("expected dict.merge to override")
	}
	v = evalExprCtx(t, ctx, "rts.dict.remove({a:1,b:2}, \"a\").a")
	if v.K != rts.VNull {
		t.Fatalf("expected dict.remove to drop key")
	}
}

func TestStdlibListDictExtras(t *testing.T) {
	ctx := rts.NewCtx(context.Background(), rts.Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	mod := "fn add1(x) { return x + 1 }\nfn even(x) { return x % 2 == 0 }\nfn gt3(x) { return x > 3 }\n"
	v := evalExprMod(t, ctx, mod, "rts.list.map([1,2], add1)[1]")
	if v.K != rts.VNum || v.N != 3 {
		t.Fatalf("expected list.map")
	}
	v = evalExprMod(t, ctx, mod, "len(rts.list.filter([1,2,3,4], even))")
	if v.K != rts.VNum || v.N != 2 {
		t.Fatalf("expected list.filter length 2")
	}
	v = evalExprMod(t, ctx, mod, "rts.list.any([1,2,3,4], gt3)")
	if v.K != rts.VBool || v.B != true {
		t.Fatalf("expected list.any true")
	}
	v = evalExprMod(t, ctx, mod, "rts.list.any([], gt3)")
	if v.K != rts.VBool || v.B != false {
		t.Fatalf("expected list.any empty false")
	}
	v = evalExprMod(t, ctx, mod, "rts.list.all([2,4], even)")
	if v.K != rts.VBool || v.B != true {
		t.Fatalf("expected list.all true")
	}
	v = evalExprMod(t, ctx, mod, "rts.list.all([], even)")
	if v.K != rts.VBool || v.B != true {
		t.Fatalf("expected list.all empty true")
	}
	v = evalExprCtx(t, ctx, "rts.list.slice([1,2,3,4], 1, 3)[0]")
	if v.K != rts.VNum || v.N != 2 {
		t.Fatalf("expected list.slice 2")
	}
	v = evalExprCtx(t, ctx, "rts.list.slice([1,2,3,4], -2)[0]")
	if v.K != rts.VNum || v.N != 3 {
		t.Fatalf("expected list.slice -2")
	}
	v = evalExprCtx(t, ctx, "rts.list.unique([1,1,2,1])[1]")
	if v.K != rts.VNum || v.N != 2 {
		t.Fatalf("expected list.unique")
	}
	v = evalExprCtx(t, ctx, "rts.dict.get({a:1}, \"a\")")
	if v.K != rts.VNum || v.N != 1 {
		t.Fatalf("expected dict.get a")
	}
	v = evalExprCtx(t, ctx, "rts.dict.get({a:1}, \"b\", 2)")
	if v.K != rts.VNum || v.N != 2 {
		t.Fatalf("expected dict.get default")
	}
	v = evalExprCtx(t, ctx, "rts.dict.has({a:1}, \"a\")")
	if v.K != rts.VBool || v.B != true {
		t.Fatalf("expected dict.has true")
	}
	v = evalExprCtx(t, ctx, "rts.dict.pick({a:1,b:2,c:3}, [\"a\",\"c\"]).b")
	if v.K != rts.VNull {
		t.Fatalf("expected dict.pick b null")
	}
	v = evalExprCtx(t, ctx, "rts.dict.pick({a:1}, \"a\").a")
	if v.K != rts.VNum || v.N != 1 {
		t.Fatalf("expected dict.pick string key")
	}
	v = evalExprCtx(t, ctx, "rts.dict.omit({a:1,b:2,c:3}, [\"b\",\"c\"]).a")
	if v.K != rts.VNum || v.N != 1 {
		t.Fatalf("expected dict.omit a")
	}
	v = evalExprCtx(t, ctx, "rts.dict.omit({a:1,b:2,c:3}, [\"b\",\"c\"]).b")
	if v.K != rts.VNull {
		t.Fatalf("expected dict.omit b null")
	}
}

func TestStdlibMathHelpers(t *testing.T) {
	ctx := rts.NewCtx(context.Background(), rts.Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	v := evalExprCtx(t, ctx, "rts.math.abs(-2)")
	if v.K != rts.VNum || v.N != 2 {
		t.Fatalf("expected math.abs 2")
	}
	v = evalExprCtx(t, ctx, "rts.math.min(2, -1)")
	if v.K != rts.VNum || v.N != -1 {
		t.Fatalf("expected math.min -1")
	}
	v = evalExprCtx(t, ctx, "rts.math.max(2, -1)")
	if v.K != rts.VNum || v.N != 2 {
		t.Fatalf("expected math.max 2")
	}
	v = evalExprCtx(t, ctx, "rts.math.clamp(5, 1, 3)")
	if v.K != rts.VNum || v.N != 3 {
		t.Fatalf("expected math.clamp high 3")
	}
	v = evalExprCtx(t, ctx, "rts.math.clamp(2, 1, 3)")
	if v.K != rts.VNum || v.N != 2 {
		t.Fatalf("expected math.clamp mid 2")
	}
	v = evalExprCtx(t, ctx, "rts.math.floor(1.8)")
	if v.K != rts.VNum || v.N != 1 {
		t.Fatalf("expected math.floor 1")
	}
	v = evalExprCtx(t, ctx, "rts.math.ceil(1.2)")
	if v.K != rts.VNum || v.N != 2 {
		t.Fatalf("expected math.ceil 2")
	}
	v = evalExprCtx(t, ctx, "rts.math.round(1.5)")
	if v.K != rts.VNum || v.N != 2 {
		t.Fatalf("expected math.round 2")
	}
}

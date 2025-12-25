package rts

import (
	"context"
	"strings"
	"testing"
)

func evalExpr(t *testing.T, src string) Value {
	ex, err := ParseExpr("test", 1, 1, src)
	if err != nil {
		t.Fatalf("parse expr: %v", err)
	}
	ctx := NewCtx(context.Background(), Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
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

func execModule(t *testing.T, src string) *Comp {
	m, err := ParseModule("test", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ctx := NewCtx(
		context.Background(),
		Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024, MaxSteps: 10000},
	)
	comp, err := Exec(ctx, m, Stdlib())
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	return comp
}

func TestEvalBasic(t *testing.T) {
	v := evalExpr(t, "1 + 2 * 3")
	if v.K != VNum || v.N != 7 {
		t.Fatalf("expected 7, got %+v", v)
	}

	v = evalExpr(t, "\"a\" + 1")
	if v.K != VStr || v.S != "a1" {
		t.Fatalf("expected a1, got %+v", v)
	}
}

func TestEvalLogic(t *testing.T) {
	v := evalExpr(t, "true and false")
	if v.K != VBool || v.B != false {
		t.Fatalf("expected false")
	}
	v = evalExpr(t, "null ?? 3")
	if v.K != VNum || v.N != 3 {
		t.Fatalf("expected 3")
	}
}

func TestEvalFnCall(t *testing.T) {
	src := "fn add(a, b){ return a + b }"
	m, err := ParseModule("test", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ctx := NewCtx(context.Background(), Limits{})
	comp, err := Exec(ctx, m, Stdlib())
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	ex, err := ParseExpr("test", 1, 1, "add(1,2)")
	if err != nil {
		t.Fatalf("parse expr: %v", err)
	}
	vm := &VM{ctx: ctx}
	v, err := vm.eval(comp.Env, ex)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if v.K != VNum || v.N != 3 {
		t.Fatalf("expected 3")
	}
}

func TestTryExprSwallowsError(t *testing.T) {
	ex, err := ParseExpr("test", 1, 1, "try missing")
	if err != nil {
		t.Fatalf("parse expr: %v", err)
	}
	ctx := NewCtx(context.Background(), Limits{})
	vm := &VM{ctx: ctx}
	env := NewEnv(nil)
	for k, v := range Stdlib() {
		env.DefConst(k, v)
	}
	v, err := vm.eval(env, ex)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if v.K != VObj {
		t.Fatalf("expected object, got %+v", v)
	}
	ok, has := v.O.GetMember("ok")
	if !has {
		t.Fatalf("expected ok member")
	}
	if ok.K != VBool || ok.B {
		t.Fatalf("expected ok=false, got %+v", ok)
	}
	val, has := v.O.GetMember("value")
	if !has {
		t.Fatalf("expected value member")
	}
	if val.K != VNull {
		t.Fatalf("expected null value, got %+v", val)
	}
	errVal, has := v.O.GetMember("error")
	if !has {
		t.Fatalf("expected error member")
	}
	if errVal.K != VStr || !strings.Contains(errVal.S, "undefined name") {
		t.Fatalf("expected error string, got %+v", errVal)
	}
}

func TestTryExprTruthy(t *testing.T) {
	src := `
let out = 0
if try missing {
  out = 1
} else {
  out = 2
}
`
	comp := execModule(t, src)
	out, ok := comp.Env.Get("out")
	if !ok || out.K != VNum || out.N != 2 {
		t.Fatalf("expected out=2, got %+v (ok=%v)", out, ok)
	}
}

func TestTryExprDoesNotSwallowAbort(t *testing.T) {
	ex, err := ParseExpr("test", 1, 1, "try (1 + 2)")
	if err != nil {
		t.Fatalf("parse expr: %v", err)
	}
	ctx := NewCtx(context.Background(), Limits{MaxSteps: 1})
	vm := &VM{ctx: ctx}
	env := NewEnv(nil)
	_, err = vm.eval(env, ex)
	if err == nil || !strings.Contains(err.Error(), "step limit exceeded") {
		t.Fatalf("expected step limit error, got %v", err)
	}
}

func TestFnParamAssignable(t *testing.T) {
	src := `
fn dec(n) {
  n = n - 1
  return n
}
`
	m, err := ParseModule("test", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ctx := NewCtx(context.Background(), Limits{})
	comp, err := Exec(ctx, m, Stdlib())
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	ex, err := ParseExpr("test", 1, 1, "dec(5)")
	if err != nil {
		t.Fatalf("parse expr: %v", err)
	}
	vm := &VM{ctx: ctx}
	v, err := vm.eval(comp.Env, ex)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if v.K != VNum || v.N != 4 {
		t.Fatalf("expected 4, got %+v", v)
	}
}

func TestForLoopCond(t *testing.T) {
	src := `
let i = 0
let sum = 0
for i < 3 {
  sum = sum + i
  i = i + 1
}
`
	comp := execModule(t, src)
	sum, ok := comp.Env.Get("sum")
	if !ok || sum.K != VNum || sum.N != 3 {
		t.Fatalf("expected sum=3, got %+v (ok=%v)", sum, ok)
	}
}

func TestForLoopClassicBreakContinue(t *testing.T) {
	src := `
let sum = 0
for let i = 0; i < 5; i = i + 1 {
  if i == 2 { continue }
  if i == 4 { break }
  sum = sum + i
}
`
	comp := execModule(t, src)
	sum, ok := comp.Env.Get("sum")
	if !ok || sum.K != VNum || sum.N != 4 {
		t.Fatalf("expected sum=4, got %+v (ok=%v)", sum, ok)
	}
	if _, ok := comp.Env.Get("i"); ok {
		t.Fatalf("expected loop var to be scoped to loop")
	}
}

func TestForLoopInfiniteBreak(t *testing.T) {
	src := `
let i = 0
for {
  if i == 3 { break }
  i = i + 1
}
`
	comp := execModule(t, src)
	i, ok := comp.Env.Get("i")
	if !ok || i.K != VNum || i.N != 3 {
		t.Fatalf("expected i=3, got %+v (ok=%v)", i, ok)
	}
}

func TestRangeList(t *testing.T) {
	src := `
let out = ""
for let i, v range ["a", "b"] {
  out = out + str(i) + v
}
`
	comp := execModule(t, src)
	out, ok := comp.Env.Get("out")
	if !ok || out.K != VStr || out.S != "0a1b" {
		t.Fatalf("expected out=0a1b, got %+v (ok=%v)", out, ok)
	}
}

func TestRangeDictDeterministic(t *testing.T) {
	src := `
let out = ""
for let k range {b: 2, a: 1} {
  out = out + k
}
`
	comp := execModule(t, src)
	out, ok := comp.Env.Get("out")
	if !ok || out.K != VStr || out.S != "ab" {
		t.Fatalf("expected out=ab, got %+v (ok=%v)", out, ok)
	}
}

func TestRangeString(t *testing.T) {
	src := `
let out = ""
for let i, ch range "ab" {
  if i == 1 { out = out + ch }
  if i == 0 { out = out + ch }
}
`
	comp := execModule(t, src)
	out, ok := comp.Env.Get("out")
	if !ok || out.K != VStr || out.S != "ab" {
		t.Fatalf("expected out=ab, got %+v (ok=%v)", out, ok)
	}
}

func TestConstImmutable(t *testing.T) {
	src := `
const x = 1
x = 2
`
	m, err := ParseModule("test", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ctx := NewCtx(context.Background(), Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	_, err = Exec(ctx, m, Stdlib())
	if err == nil || !strings.Contains(err.Error(), "const") {
		t.Fatalf("expected const assignment error, got %v", err)
	}
}

func TestBuiltinRedeclareRejected(t *testing.T) {
	src := `
let len = 1
`
	m, err := ParseModule("test", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ctx := NewCtx(context.Background(), Limits{})
	_, err = Exec(ctx, m, Stdlib())
	if err == nil || !strings.Contains(err.Error(), "name already defined") {
		t.Fatalf("expected name already defined error, got %v", err)
	}
}

func TestPreludeRedeclareRejected(t *testing.T) {
	src := `
let env = 1
`
	m, err := ParseModule("test", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ctx := NewCtx(context.Background(), Limits{})
	pre := Stdlib()
	pre["env"] = Obj(newMapObj("env", map[string]string{}))
	_, err = Exec(ctx, m, pre)
	if err == nil || !strings.Contains(err.Error(), "name already defined") {
		t.Fatalf("expected name already defined error, got %v", err)
	}
}

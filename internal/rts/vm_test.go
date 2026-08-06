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
	for k, v := range testStdlib() {
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
	comp, err := Exec(ctx, m, testStdlib())
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	return comp
}

func execModuleErr(t *testing.T, src string, lim Limits) error {
	t.Helper()
	m, err := ParseModule("test", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := Exec(NewCtx(context.Background(), lim), m, testStdlib()); err != nil {
		return err
	}
	t.Fatalf("expected exec error")
	return nil
}

func wantStr(t *testing.T, comp *Comp, name, want string) {
	t.Helper()
	v, ok := comp.Env.Get(name)
	if !ok || v.K != VStr || v.S != want {
		t.Fatalf("expected %s=%q, got %+v (ok=%v)", name, want, v, ok)
	}
}

func wantNum(t *testing.T, comp *Comp, name string, want float64) {
	t.Helper()
	v, ok := comp.Env.Get(name)
	if !ok || v.K != VNum || v.N != want {
		t.Fatalf("expected %s=%v, got %+v (ok=%v)", name, want, v, ok)
	}
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

func TestListIndexRequiresInteger(t *testing.T) {
	ex, err := ParseExpr("test", 1, 1, "[10,20][1.2]")
	if err != nil {
		t.Fatalf("parse expr: %v", err)
	}
	ctx := NewCtx(context.Background(), Limits{MaxStr: 1024, MaxList: 1024, MaxDict: 1024})
	vm := &VM{ctx: ctx}
	env := NewEnv(nil)
	_, err = vm.eval(env, ex)
	if err == nil || !strings.Contains(err.Error(), "list index must be integer") {
		t.Fatalf("expected integer index error, got %v", err)
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

// ?? must not evaluate its right side unless the left side is null, which is
// what separates it from a default(a, b) call
func TestCoalesceIsLazy(t *testing.T) {
	src := `
let calls = 0
fn side(v) {
  calls = calls + 1
  return v
}
let kept = "ok" ?? side("fallback")
let taken = null ?? side("fallback")
let unresolved = "ok" ?? missingName
`
	comp := execModule(t, src)
	wantStr(t, comp, "kept", "ok")
	wantStr(t, comp, "taken", "fallback")
	wantStr(t, comp, "unresolved", "ok")
	wantNum(t, comp, "calls", 1)
}

func TestCoalesceRejectsUndefinedLeftSide(t *testing.T) {
	err := execModuleErr(t, "let v = missingName ?? \"fallback\"\n", Limits{})
	if !strings.Contains(err.Error(), `undefined name "missingName"`) {
		t.Fatalf("expected undefined name error, got %v", err)
	}
}

// ?? answers "is it null", not "is it falsey". Ternary is the truthiness form
func TestCoalesceFallsBackOnNullOnly(t *testing.T) {
	src := `
let zero = 0 ?? 5
let empty = "" ?? "x"
let no = false ?? true
let list = [] ?? [1]
let dict = {} ?? {a: 1}
let nul = null ?? 5
let truthy = 0 ? 0 : 5
`
	comp := execModule(t, src)
	wantNum(t, comp, "zero", 0)
	wantStr(t, comp, "empty", "")
	wantNum(t, comp, "nul", 5)
	wantNum(t, comp, "truthy", 5)

	no, _ := comp.Env.Get("no")
	if no.K != VBool || no.B {
		t.Fatalf("expected no=false, got %+v", no)
	}
	list, _ := comp.Env.Get("list")
	if list.K != VList || len(list.L) != 0 {
		t.Fatalf("expected empty list, got %+v", list)
	}
	dict, _ := comp.Env.Get("dict")
	if dict.K != VDict || len(dict.M) != 0 {
		t.Fatalf("expected empty dict, got %+v", dict)
	}
}

func TestEvalFnCall(t *testing.T) {
	src := "fn add(a, b){ return a + b }"
	m, err := ParseModule("test", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ctx := NewCtx(context.Background(), Limits{})
	comp, err := Exec(ctx, m, testStdlib())
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
	for k, v := range testStdlib() {
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
	comp, err := Exec(ctx, m, testStdlib())
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

func TestSwitchEvaluatesTagOnce(t *testing.T) {
	src := `
let calls = 0
fn tag() {
  calls = calls + 1
  return 3
}
let out = ""
switch tag() {
case 1:
  out = "one"
case 2:
  out = "two"
case 3:
  out = "three"
}
`
	comp := execModule(t, src)
	wantNum(t, comp, "calls", 1)
	wantStr(t, comp, "out", "three")
}

func TestSwitchCaseEvaluationOrderStops(t *testing.T) {
	src := `
let log = ""
fn probe(name, v) {
  log = log + name
  return v
}
let out = ""
switch 2 {
case probe("a", 1), probe("b", 2), probe("c", 2):
  out = "hit"
case probe("d", 2):
  out = "miss"
}
`
	comp := execModule(t, src)
	wantStr(t, comp, "log", "ab")
	wantStr(t, comp, "out", "hit")
}

func TestSwitchFirstMatchOnlyNoFallthrough(t *testing.T) {
	src := `
let out = ""
switch 1 {
case 1:
  out = out + "a"
case 1:
  out = out + "b"
default:
  out = out + "d"
}
`
	comp := execModule(t, src)
	wantStr(t, comp, "out", "a")
}

func TestSwitchDefault(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"runs when nothing matches",
			"switch 9 {\ncase 1:\n  out = \"a\"\ndefault:\n  out = \"d\"\n}",
			"d",
		},
		{
			"skipped when a case matches",
			"switch 1 {\ncase 1:\n  out = \"a\"\ndefault:\n  out = \"d\"\n}",
			"a",
		},
		{
			"reachable from any position",
			"switch 9 {\ndefault:\n  out = \"d\"\ncase 1:\n  out = \"a\"\n}",
			"d",
		},
		{
			"absent leaves state untouched",
			"switch 9 {\ncase 1:\n  out = \"a\"\n}",
			"none",
		},
		{
			"empty body runs nothing",
			"switch 9 {\ncase 1:\n  out = \"a\"\ndefault:\n}",
			"none",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			comp := execModule(t, "let out = \"none\"\n"+tc.src+"\n")
			wantStr(t, comp, "out", tc.want)
		})
	}
}

func TestSwitchEqualityIsTypeSensitive(t *testing.T) {
	cases := []struct {
		name string
		tag  string
		want string
	}{
		{"number matches number", "1", "num"},
		{"string is not number", "\"1\"", "str"},
		{"bool is not number", "true", "bool"},
		{"zero is not false", "0", "d"},
		{"null matches null", "null", "null"},
		{"lists never match", "[1]", "d"},
		{"dicts never match", "({a: 1})", "d"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "let out = \"\"\nswitch " + tc.tag + " {\n" +
				"case 1:\n  out = \"num\"\n" +
				"case \"1\":\n  out = \"str\"\n" +
				"case null:\n  out = \"null\"\n" +
				"case true:\n  out = \"bool\"\n" +
				"case [1]:\n  out = \"list\"\n" +
				"case {a: 1}:\n  out = \"dict\"\n" +
				"default:\n  out = \"d\"\n}\n"
			comp := execModule(t, src)
			wantStr(t, comp, "out", tc.want)
		})
	}
}

func TestSwitchTaglessUsesTruthiness(t *testing.T) {
	src := `
let out = ""
switch {
case "":
  out = "empty string"
case 0:
  out = "zero"
case []:
  out = "empty list"
case "x":
  out = "truthy"
default:
  out = "d"
}
`
	comp := execModule(t, src)
	wantStr(t, comp, "out", "truthy")
}

func TestSwitchTaglessFallsToDefault(t *testing.T) {
	src := `
let out = ""
switch {
case null:
  out = "null"
case 0:
  out = "zero"
default:
  out = "d"
}
`
	comp := execModule(t, src)
	wantStr(t, comp, "out", "d")
}

func TestSwitchClauseScope(t *testing.T) {
	src := `
let out = "outer"
let shadow = 1
switch 1 {
case 1:
  let inner = "clause"
  let shadow = 2
  out = inner
}
`
	comp := execModule(t, src)
	wantStr(t, comp, "out", "clause")
	wantNum(t, comp, "shadow", 1)
	if _, ok := comp.Env.Get("inner"); ok {
		t.Fatalf("expected clause declaration to stay in the clause")
	}
}

func TestSwitchBreakStopsAtSwitch(t *testing.T) {
	src := `
let out = ""
switch 1 {
case 1:
  out = out + "a"
  break
  out = out + "b"
default:
  out = out + "d"
}
out = out + "after"
`
	comp := execModule(t, src)
	wantStr(t, comp, "out", "aafter")
}

func TestSwitchNestedBreakLeavesOuterRunning(t *testing.T) {
	src := `
let out = ""
switch 1 {
case 1:
  switch 2 {
  case 2:
    out = out + "inner"
    break
  }
  out = out + "outer"
}
`
	comp := execModule(t, src)
	wantStr(t, comp, "out", "innerouter")
}

func TestSwitchBreakInLoopTargetsSwitch(t *testing.T) {
	src := `
let out = ""
for let i = 0; i < 4; i = i + 1 {
  switch i {
  case 2:
    break
  }
  out = out + str(i)
}
`
	comp := execModule(t, src)
	wantStr(t, comp, "out", "0123")
}

func TestSwitchContinueTargetsLoop(t *testing.T) {
	src := `
let out = ""
for let i = 0; i < 4; i = i + 1 {
  switch i {
  case 1:
    continue
  case 2:
    out = out + "two"
    continue
  }
  out = out + str(i)
}
`
	comp := execModule(t, src)
	wantStr(t, comp, "out", "0two3")
}

func TestLabeledBreakEscapesLoopFromSwitch(t *testing.T) {
	src := `
let out = ""
outer: for let i = 0; i < 5; i = i + 1 {
  switch i {
  case 3:
    break outer
  }
  out = out + str(i)
}
`
	comp := execModule(t, src)
	wantStr(t, comp, "out", "012")
}

func TestLabeledBreakPropagatesThroughNesting(t *testing.T) {
	src := `
let out = ""
outer: for let i = 0; i < 3; i = i + 1 {
  for let j = 0; j < 3; j = j + 1 {
    switch j {
    case 1:
      switch i {
      case 1:
        break outer
      }
    }
    out = out + str(i) + str(j)
  }
}
`
	comp := execModule(t, src)
	wantStr(t, comp, "out", "00010210")
}

// a labeled continue runs the target loop's post once and skips the post of
// every loop it travels through
func TestLabeledContinueRunsTargetPostOnce(t *testing.T) {
	src := `
let posts = 0
let inner = 0
fn bumpOuter(i) {
  posts = posts + 1
  return i + 1
}
fn bumpInner(j) {
  inner = inner + 1
  return j + 1
}
let out = ""
outer: for let i = 0; i < 3; i = bumpOuter(i) {
  for let j = 0; j < 3; j = bumpInner(j) {
    continue outer
  }
  out = out + "unreachable"
}
`
	comp := execModule(t, src)
	wantNum(t, comp, "posts", 3)
	wantNum(t, comp, "inner", 0)
	wantStr(t, comp, "out", "")
}

func TestLabeledRangeBreakAndContinue(t *testing.T) {
	src := `
let out = ""
outer: for let i, row range [[1, 2], [3, 9], [5, 6]] {
  for let j, v range row {
    switch v {
    case 9:
      continue outer
    case 5:
      break outer
    }
    out = out + str(v)
  }
  out = out + "|"
}
`
	comp := execModule(t, src)
	wantStr(t, comp, "out", "12|3")
}

func TestLabeledBreakLeavesOuterSwitch(t *testing.T) {
	src := `
let out = ""
outer: switch 1 {
case 1:
  out = out + "a"
  switch 2 {
  case 2:
    out = out + "b"
    break outer
  }
  out = out + "c"
}
out = out + "d"
`
	comp := execModule(t, src)
	wantStr(t, comp, "out", "abd")
}

// a labeled break travels past the loop it is written in when the label names
// an enclosing switch
func TestLabeledBreakSkipsLoopToReachSwitch(t *testing.T) {
	src := `
let out = ""
sw: switch 1 {
case 1:
  for let i = 0; i < 3; i = i + 1 {
    switch i {
    case 1:
      break sw
    }
    out = out + str(i)
  }
  out = out + "tail"
}
out = out + "end"
`
	comp := execModule(t, src)
	wantStr(t, comp, "out", "0end")
}

func TestLabeledSignalsInsideFunction(t *testing.T) {
	src := `
fn firstMissing(rows) {
  outer: for let i, row range rows {
    for let j, v range row {
      switch v {
      case null:
        continue outer
      }
    }
    return i
  }
  return -1
}
let a = firstMissing([[1, null], [2, 3]])
let b = firstMissing([[null], [null]])
`
	comp := execModule(t, src)
	wantNum(t, comp, "a", 1)
	wantNum(t, comp, "b", -1)
}

func TestSwitchReturnPropagates(t *testing.T) {
	src := `
fn grade(score) {
  switch {
  case score >= 90:
    return "A"
  case score >= 80:
    return "B"
  default:
    return "C"
  }
  return "unreachable"
}
let a = grade(95)
let b = grade(85)
let c = grade(10)
`
	comp := execModule(t, src)
	wantStr(t, comp, "a", "A")
	wantStr(t, comp, "b", "B")
	wantStr(t, comp, "c", "C")
}

func TestSwitchErrorsKeepPosition(t *testing.T) {
	cases := []struct {
		name string
		src  string
		msg  string
		line int
		col  int
	}{
		{"tag", "switch missing {\ncase 1:\n}\n", "undefined name \"missing\"", 1, 8},
		{"case expression", "switch 1 {\ncase missing:\n}\n", "undefined name \"missing\"", 2, 6},
		{"clause body", "switch 1 {\ncase 1:\n  let x = missing\n}\n",
			"undefined name \"missing\"", 3, 11},
		{"default body", "switch 9 {\ndefault:\n  let x = missing\n}\n",
			"undefined name \"missing\"", 3, 11},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := execModuleErr(t, tc.src, Limits{})
			se, ok := err.(*StackError)
			if !ok {
				t.Fatalf("expected *StackError, got %T", err)
			}
			re, ok := se.Err.(*RuntimeError)
			if !ok {
				t.Fatalf("expected *RuntimeError, got %T", se.Err)
			}
			if re.Msg != tc.msg {
				t.Fatalf("msg: got %q, want %q", re.Msg, tc.msg)
			}
			if re.Pos.Line != tc.line || re.Pos.Col != tc.col {
				t.Fatalf("pos: got %d:%d, want %d:%d", re.Pos.Line, re.Pos.Col, tc.line, tc.col)
			}
		})
	}
}

func TestSwitchCaseErrorKeepsCallStack(t *testing.T) {
	src := `
fn boom() {
  return missing
}
switch 1 {
case boom():
  let x = 1
}
`
	err := execModuleErr(t, src, Limits{})
	se, ok := err.(*StackError)
	if !ok {
		t.Fatalf("expected *StackError, got %T", err)
	}
	if len(se.Frames) != 1 || se.Frames[0].Name != "boom" {
		t.Fatalf("expected boom frame, got %+v", se.Frames)
	}
}

func TestSwitchStepLimitPropagates(t *testing.T) {
	src := `
let out = 0
switch 1 {
case 1:
  out = 1
}
`
	err := execModuleErr(t, src, Limits{MaxSteps: 6})
	if !isAbort(err) {
		t.Fatalf("expected abort, got %T (%v)", err, err)
	}
	if !strings.Contains(err.Error(), "step limit exceeded") {
		t.Fatalf("expected step limit error, got %v", err)
	}
}

func TestSwitchHardAbortNotSwallowed(t *testing.T) {
	src := `
let out = 0
switch 1 {
case 1:
  for {
    out = out + 1
  }
}
`
	err := execModuleErr(t, src, Limits{MaxSteps: 200})
	if !isAbort(err) {
		t.Fatalf("expected abort, got %T (%v)", err, err)
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
	_, err = Exec(ctx, m, testStdlib())
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
	_, err = Exec(ctx, m, testStdlib())
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
	pre := testStdlib()
	pre["env"] = Obj(newMapObj("env", map[string]string{}))
	_, err = Exec(ctx, m, pre)
	if err == nil || !strings.Contains(err.Error(), "name already defined") {
		t.Fatalf("expected name already defined error, got %v", err)
	}
}

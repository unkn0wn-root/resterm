package rts

import "math"

// Args validates and converts arguments passed to native RTS functions.
type Args struct {
	ctx  *Ctx
	pos  Pos
	args []Value
	sig  string
}

// NewArgs creates an argument validator for a native RTS function call.
func NewArgs(ctx *Ctx, pos Pos, args []Value, sig string) Args {
	return Args{ctx: ctx, pos: pos, args: args, sig: sig}
}

func (a Args) Count(want int) error {
	return ArgCount(a.ctx, a.pos, a.args, want, a.sig)
}

func (a Args) None() error {
	return a.Count(0)
}

func (a Args) CountRange(min, max int) error {
	return ArgCountRange(a.ctx, a.pos, a.args, min, max, a.sig)
}

func (a Args) Len() int {
	return len(a.args)
}

func (a Args) Has(i int) bool {
	return i >= 0 && i < len(a.args)
}

func (a Args) Arg(i int) Value {
	return a.args[i]
}

func (a Args) Str(i int) (string, error) {
	return StrArg(a.ctx, a.pos, a.args[i], a.sig)
}

func (a Args) ToStr(i int) (string, error) {
	return ToStr(a.ctx, a.pos, a.args[i])
}

func (a Args) Num(i int) (float64, error) {
	return NumArg(a.ctx, a.pos, a.args[i], a.sig)
}

func (a Args) FiniteNum(i int) (float64, error) {
	n, err := a.Num(i)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, Errf(a.ctx, a.pos, "%s expects finite number", a.sig)
	}
	return n, nil
}

func (a Args) Bool(i int) (bool, error) {
	v := a.args[i]
	if v.K != VBool {
		return false, Errf(a.ctx, a.pos, "%s expects bool", a.sig)
	}
	return v.B, nil
}

func (a Args) ScalarStr(i int) (string, error) {
	return ScalarStr(a.ctx, a.pos, a.args[i], a.sig)
}

func (a Args) List(i int) ([]Value, error) {
	return ListArg(a.ctx, a.pos, a.args[i], a.sig)
}

func (a Args) Dict(i int) (map[string]Value, error) {
	return DictArg(a.ctx, a.pos, a.args[i], a.sig)
}

func (a Args) Key(i int) (string, error) {
	return KeyArg(a.ctx, a.pos, a.args[i], a.sig)
}

func (a Args) MapKey(key string) (string, error) {
	return MapKey(a.ctx, a.pos, key, a.sig)
}

func (a Args) Fn(i int) (Value, error) {
	v := a.args[i]
	if err := CheckFunc(a.ctx, a.pos, v, a.sig); err != nil {
		return Null(), err
	}
	return v, nil
}

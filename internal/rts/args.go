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

func argCount(ctx *Ctx, pos Pos, args []Value, want int, sig string) error {
	if len(args) != want {
		return Errf(ctx, pos, "%s expects %d args", sig, want)
	}
	return nil
}

func argCountRange(ctx *Ctx, pos Pos, args []Value, min, max int, sig string) error {
	if len(args) < min || len(args) > max {
		return Errf(ctx, pos, "%s expects %d-%d args", sig, min, max)
	}
	return nil
}

func strArg(ctx *Ctx, pos Pos, v Value, sig string) (string, error) {
	s, err := ToStr(ctx, pos, v)
	if err != nil {
		return "", err
	}
	if err := CheckStr(ctx, pos, s); err != nil {
		return "", err
	}
	return s, nil
}

func numArg(ctx *Ctx, pos Pos, v Value, sig string) (float64, error) {
	if v.K != VNum {
		return 0, Errf(ctx, pos, "%s expects number", sig)
	}
	return v.N, nil
}

func listArg(ctx *Ctx, pos Pos, v Value, sig string) ([]Value, error) {
	if v.K == VNull {
		return nil, nil
	}

	if v.K != VList {
		return nil, Errf(ctx, pos, "%s expects list", sig)
	}

	if err := CheckList(ctx, pos, len(v.L)); err != nil {
		return nil, err
	}
	return v.L, nil
}

func dictArg(ctx *Ctx, pos Pos, v Value, sig string) (map[string]Value, error) {
	if v.K == VNull {
		return nil, nil
	}
	if v.K != VDict {
		return nil, Errf(ctx, pos, "%s expects dict", sig)
	}
	return v.M, nil
}

func (a Args) Count(want int) error {
	return argCount(a.ctx, a.pos, a.args, want, a.sig)
}

func (a Args) None() error {
	return a.Count(0)
}

func (a Args) CountRange(min, max int) error {
	return argCountRange(a.ctx, a.pos, a.args, min, max, a.sig)
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
	return strArg(a.ctx, a.pos, a.args[i], a.sig)
}

func (a Args) ToStr(i int) (string, error) {
	return ToStr(a.ctx, a.pos, a.args[i])
}

func (a Args) Num(i int) (float64, error) {
	return numArg(a.ctx, a.pos, a.args[i], a.sig)
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
	return listArg(a.ctx, a.pos, a.args[i], a.sig)
}

func (a Args) Dict(i int) (map[string]Value, error) {
	return dictArg(a.ctx, a.pos, a.args[i], a.sig)
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

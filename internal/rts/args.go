package rts

import "math"

// Key validates that v is a string key and returns its raw value.
func Key(pos Pos, v Value) (string, error) {
	if v.K != VStr {
		return "", Errf(nil, pos, "expected string key")
	}
	return v.S, nil
}

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
	if len(a.args) != want {
		return Errf(a.ctx, a.pos, "%s expects %d argument%s, got %d", a.sig, want, plural(want), len(a.args))
	}
	return nil
}

func (a Args) None() error {
	return a.Count(0)
}

// CountOptional checks for n arguments followed by one optional argument.
func (a Args) CountOptional(n int) error {
	if got := len(a.args); got != n && got != n+1 {
		return Errf(a.ctx, a.pos, "%s expects %d or %d argument%s, got %d", a.sig, n, n+1, plural(n+1), got)
	}
	return nil
}

func (a Args) CountMin(n int) error {
	if len(a.args) < n {
		return Errf(a.ctx, a.pos, "%s expects at least %d argument%s, got %d", a.sig, n, plural(n), len(a.args))
	}
	return nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
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

// SearchStr is like Str for an argument that is searched for, such as a
// prefix or a pattern, but null is an error instead of the empty string.
// name is the argument as the error calls it, for example "prefix".
func (a Args) SearchStr(i int, name string) (string, error) {
	if a.args[i].K == VNull {
		return "", Errf(a.ctx, a.pos, "cannot search for a null %s", name)
	}
	return a.Str(i)
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
	return a.KeyOf(a.args[i])
}

// KeyOf validates a string key that is not itself an argument, such as an
// element of a list an argument holds.
func (a Args) KeyOf(v Value) (string, error) {
	k, err := Key(a.pos, v)
	if err != nil {
		return "", WrapErr(a.ctx, err)
	}
	return k, nil
}

// Sig is the signature these arguments are checked against.
func (a Args) Sig() string { return a.sig }

// In names one argument, so a helper that takes more than one dictionary can
// report which of them an error came from.
func (a Args) In(label string) Args {
	a.sig += ": " + label
	return a
}

func (a Args) Fn(i int) (Value, error) {
	v := a.args[i]
	if err := CheckFunc(a.ctx, a.pos, v, a.sig); err != nil {
		return Null(), err
	}
	return v, nil
}

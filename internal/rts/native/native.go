// Package native adapts typed Go functions to the raw RTS native-function ABI.
package native

import (
	"math"

	"github.com/unkn0wn-root/resterm/internal/rts"
)

// Call is the source-aware context of one native call.
type Call struct {
	Ctx *rts.Ctx
	Pos rts.Pos
	Sig string
}

// Errorf returns an RTS runtime error at the call site.
func (c Call) Errorf(format string, args ...any) error {
	return rts.Errf(c.Ctx, c.Pos, format, args...)
}

// Decoder converts one RTS value to the Go type required by a function.
type Decoder[T any] func(Call, rts.Value) (T, error)

// Optional carries an optional decoded argument without using a sentinel value.
type Optional[T any] struct {
	Value T
	Set   bool
}

// Def is a fully named native function definition. Its constructors enforce
// arity before any decoder or implementation runs.
type Def struct {
	name string
	fn   rts.NativeFunc
}

// Name returns the stack-frame name of the definition.
func (d Def) Name() string { return d.name }

// Func returns the raw ABI implementation for registries that store functions.
func (d Def) Func() rts.NativeFunc { return d.fn }

// Value returns the definition as an RTS value with named stack frames.
func (d Def) Value() rts.Value { return rts.NativeNamed(d.name, d.fn) }

// Raw defines a variable-arity function that performs its own argument
// decoding. Prefer Fn0 through Fn3 for new fixed-arity functions.
func Raw(name string, fn rts.NativeFunc) Def {
	return Def{name: name, fn: fn}
}

// Fn0 defines a function with no arguments.
func Fn0(name, sig string, fn func(Call) (rts.Value, error)) Def {
	return Def{name: name, fn: func(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
		call := Call{Ctx: ctx, Pos: pos, Sig: sig}
		if err := count(call, len(args), 0); err != nil {
			return rts.Null(), err
		}
		return fn(call)
	}}
}

// FnOptional defines a function with one optional argument.
func FnOptional[A any](
	name, sig string,
	a Decoder[A],
	fn func(Call, Optional[A]) (rts.Value, error),
) Def {
	return Def{name: name, fn: func(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
		call := Call{Ctx: ctx, Pos: pos, Sig: sig}
		if err := countRange(call, len(args), 0, 1); err != nil {
			return rts.Null(), err
		}
		var av Optional[A]
		if len(args) == 1 {
			var err error
			av.Value, err = a(call, args[0])
			if err != nil {
				return rts.Null(), err
			}
			av.Set = true
		}
		return fn(call, av)
	}}
}

// Fn1 defines a function with one decoded argument.
func Fn1[A any](
	name, sig string,
	a Decoder[A],
	fn func(Call, A) (rts.Value, error),
) Def {
	return Def{name: name, fn: func(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
		call := Call{Ctx: ctx, Pos: pos, Sig: sig}
		if err := count(call, len(args), 1); err != nil {
			return rts.Null(), err
		}
		av, err := a(call, args[0])
		if err != nil {
			return rts.Null(), err
		}
		return fn(call, av)
	}}
}

// Fn2 defines a function with two decoded arguments.
func Fn2[A, B any](
	name, sig string,
	a Decoder[A], b Decoder[B],
	fn func(Call, A, B) (rts.Value, error),
) Def {
	return Def{name: name, fn: func(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
		call := Call{Ctx: ctx, Pos: pos, Sig: sig}
		if err := count(call, len(args), 2); err != nil {
			return rts.Null(), err
		}
		av, err := a(call, args[0])
		if err != nil {
			return rts.Null(), err
		}
		bv, err := b(call, args[1])
		if err != nil {
			return rts.Null(), err
		}
		return fn(call, av, bv)
	}}
}

// Fn3 defines a function with three decoded arguments.
func Fn3[A, B, C any](
	name, sig string,
	a Decoder[A], b Decoder[B], c Decoder[C],
	fn func(Call, A, B, C) (rts.Value, error),
) Def {
	return Def{name: name, fn: func(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
		call := Call{Ctx: ctx, Pos: pos, Sig: sig}
		if err := count(call, len(args), 3); err != nil {
			return rts.Null(), err
		}
		av, err := a(call, args[0])
		if err != nil {
			return rts.Null(), err
		}
		bv, err := b(call, args[1])
		if err != nil {
			return rts.Null(), err
		}
		cv, err := c(call, args[2])
		if err != nil {
			return rts.Null(), err
		}
		return fn(call, av, bv, cv)
	}}
}

// Fn1Optional defines a function with one required and one optional argument.
func Fn1Optional[A, B any](
	name, sig string,
	a Decoder[A], b Decoder[B],
	fn func(Call, A, Optional[B]) (rts.Value, error),
) Def {
	return Def{name: name, fn: func(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
		call := Call{Ctx: ctx, Pos: pos, Sig: sig}
		if err := countRange(call, len(args), 1, 2); err != nil {
			return rts.Null(), err
		}
		av, err := a(call, args[0])
		if err != nil {
			return rts.Null(), err
		}
		var bv Optional[B]
		if len(args) == 2 {
			bv.Value, err = b(call, args[1])
			if err != nil {
				return rts.Null(), err
			}
			bv.Set = true
		}
		return fn(call, av, bv)
	}}
}

// Fn2Optional defines a function with two required and one optional argument.
func Fn2Optional[A, B, C any](
	name, sig string,
	a Decoder[A], b Decoder[B], c Decoder[C],
	fn func(Call, A, B, Optional[C]) (rts.Value, error),
) Def {
	return Def{name: name, fn: func(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
		call := Call{Ctx: ctx, Pos: pos, Sig: sig}
		if err := countRange(call, len(args), 2, 3); err != nil {
			return rts.Null(), err
		}
		av, err := a(call, args[0])
		if err != nil {
			return rts.Null(), err
		}
		bv, err := b(call, args[1])
		if err != nil {
			return rts.Null(), err
		}
		var cv Optional[C]
		if len(args) == 3 {
			cv.Value, err = c(call, args[2])
			if err != nil {
				return rts.Null(), err
			}
			cv.Set = true
		}
		return fn(call, av, bv, cv)
	}}
}

func count(call Call, got, want int) error {
	if got == want {
		return nil
	}
	word := "arguments"
	if want == 1 {
		word = "argument"
	}
	return call.Errorf("%s expects %d %s, got %d", call.Sig, want, word, got)
}

func countRange(call Call, got, min, max int) error {
	if got >= min && got <= max {
		return nil
	}
	word := "arguments"
	if max == 1 {
		word = "argument"
	}
	return call.Errorf("%s expects %d or %d %s, got %d", call.Sig, min, max, word, got)
}

// Value leaves an argument in the RTS value domain.
func Value(_ Call, v rts.Value) (rts.Value, error) { return v, nil }

// String decodes only an RTS string. It never invokes implicit conversion.
func String(call Call, v rts.Value) (string, error) {
	return decodeString(call, v, "string")
}

func decodeString(call Call, v rts.Value, want string) (string, error) {
	if v.K != rts.VStr {
		return "", call.Errorf("%s expects %s", call.Sig, want)
	}
	if err := rts.CheckStr(call.Ctx, call.Pos, v.S); err != nil {
		return "", err
	}
	return v.S, nil
}

// Bool decodes only an RTS boolean.
func Bool(call Call, v rts.Value) (bool, error) {
	if v.K != rts.VBool {
		return false, call.Errorf("%s expects bool", call.Sig)
	}
	return v.B, nil
}

// Number decodes only an RTS number.
func Number(call Call, v rts.Value) (float64, error) {
	if v.K != rts.VNum {
		return 0, call.Errorf("%s expects number", call.Sig)
	}
	return v.N, nil
}

// FiniteNumber decodes a finite RTS number.
func FiniteNumber(call Call, v rts.Value) (float64, error) {
	n, err := Number(call, v)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, call.Errorf("%s expects finite number", call.Sig)
	}
	return n, nil
}

// List decodes only an RTS list. It does not treat null as an empty list.
func List(call Call, v rts.Value) ([]rts.Value, error) {
	if v.K != rts.VList {
		return nil, call.Errorf("%s expects list", call.Sig)
	}
	if err := rts.CheckList(call.Ctx, call.Pos, len(v.L)); err != nil {
		return nil, err
	}
	return v.L, nil
}

// Dict decodes only an RTS dictionary. It does not treat null as an empty
// dictionary.
func Dict(call Call, v rts.Value) (map[string]rts.Value, error) {
	if v.K != rts.VDict {
		return nil, call.Errorf("%s expects dict", call.Sig)
	}
	if err := rts.CheckDict(call.Ctx, call.Pos, len(v.M)); err != nil {
		return nil, err
	}
	return v.M, nil
}

// StringList decodes a list whose elements are all strings and returns a copy.
// The outer list is validated before any element, so a list over the limit
// reports that rather than whatever its first element happens to be.
func StringList(call Call, v rts.Value) ([]string, error) {
	vals, err := List(call, v)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(vals))
	for i, val := range vals {
		out[i], err = decodeString(call, val, "list<string>")
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

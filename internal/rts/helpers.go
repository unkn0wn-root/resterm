package rts

import (
	"fmt"
	"strings"
)

func ArgCount(ctx *Ctx, pos Pos, args []Value, want int, sig string) error {
	if len(args) != want {
		return Errf(ctx, pos, "%s expects %d args", sig, want)
	}
	return nil
}

func ArgCountRange(ctx *Ctx, pos Pos, args []Value, min, max int, sig string) error {
	if len(args) < min || len(args) > max {
		return Errf(ctx, pos, "%s expects %d-%d args", sig, min, max)
	}
	return nil
}

func DictArg(ctx *Ctx, pos Pos, v Value, sig string) (map[string]Value, error) {
	if v.K == VNull {
		return nil, nil
	}
	if v.K != VDict {
		return nil, Errf(ctx, pos, "%s expects dict", sig)
	}
	return v.M, nil
}

func KeyArg(ctx *Ctx, pos Pos, v Value, sig string) (string, error) {
	k, err := Key(pos, v)
	if err != nil {
		return "", WrapErr(ctx, err)
	}

	k = strings.TrimSpace(k)
	if k == "" {
		return "", Errf(ctx, pos, "%s expects non-empty key", sig)
	}
	return k, nil
}

func MapKey(ctx *Ctx, pos Pos, key, sig string) (string, error) {
	k := strings.TrimSpace(key)
	if k == "" {
		return "", Errf(ctx, pos, "%s expects non-empty key", sig)
	}
	return k, nil
}

func ScalarStr(ctx *Ctx, pos Pos, v Value, sig string) (string, error) {
	switch v.K {
	case VStr, VNum, VBool:
		return ToStr(ctx, pos, v)
	default:
		return "", Errf(ctx, pos, "%s expects string/number/bool", sig)
	}
}

func StrArg(ctx *Ctx, pos Pos, v Value, sig string) (string, error) {
	s, err := ToStr(ctx, pos, v)
	if err != nil {
		return "", err
	}
	if err := CheckStr(ctx, pos, s); err != nil {
		return "", err
	}
	return s, nil
}

func NumArg(ctx *Ctx, pos Pos, v Value, sig string) (float64, error) {
	if v.K != VNum {
		return 0, Errf(ctx, pos, "%s expects number", sig)
	}
	return v.N, nil
}

func ListArg(ctx *Ctx, pos Pos, v Value, sig string) ([]Value, error) {
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

func CheckStr(ctx *Ctx, pos Pos, s string) error {
	if ctx == nil || ctx.Lim.MaxStr <= 0 {
		return nil
	}
	if len(s) > ctx.Lim.MaxStr {
		return Errf(ctx, pos, "string too long")
	}
	return nil
}

func CheckList(ctx *Ctx, pos Pos, n int) error {
	if ctx == nil || ctx.Lim.MaxList <= 0 {
		return nil
	}
	if n > ctx.Lim.MaxList {
		return Errf(ctx, pos, "list too large")
	}
	return nil
}

func CheckDict(ctx *Ctx, pos Pos, n int) error {
	if ctx == nil || ctx.Lim.MaxDict <= 0 {
		return nil
	}
	if n > ctx.Lim.MaxDict {
		return Errf(ctx, pos, "dict too large")
	}
	return nil
}

func lowerKey(key string) string {
	return strings.ToLower(key)
}

func reqMsg(ctx *Ctx, pos Pos, args []Value) (string, error) {
	if len(args) < 2 {
		return "", nil
	}

	s, err := ToStr(ctx, pos, args[1])
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(s), nil
}

func reqErr(ctx *Ctx, pos Pos, obj, key string, args []Value) error {
	msg, err := reqMsg(ctx, pos, args)
	if err != nil {
		return err
	}
	if msg == "" {
		msg = fmt.Sprintf("missing required %s: %s", obj, key)
	}
	return Errf(ctx, pos, "%s", msg)
}

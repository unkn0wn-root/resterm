package rts

import (
	"fmt"
	"strings"
)

func argCount(ctx *Ctx, pos Pos, args []Value, want int, sig string) error {
	if len(args) != want {
		return rtErr(ctx, pos, "%s expects %d args", sig, want)
	}
	return nil
}

func argCountRange(ctx *Ctx, pos Pos, args []Value, min, max int, sig string) error {
	if len(args) < min || len(args) > max {
		return rtErr(ctx, pos, "%s expects %d-%d args", sig, min, max)
	}
	return nil
}

func dictArg(ctx *Ctx, pos Pos, v Value, sig string) (map[string]Value, error) {
	if v.K == VNull {
		return nil, nil
	}
	if v.K != VDict {
		return nil, rtErr(ctx, pos, "%s expects dict", sig)
	}
	return v.M, nil
}

func cloneDict(in map[string]Value) map[string]Value {
	return cloneMap(in)
}

func keyArg(ctx *Ctx, pos Pos, v Value, sig string) (string, error) {
	k, err := toKey(pos, v)
	if err != nil {
		return "", wrapErr(ctx, err)
	}

	k = strings.TrimSpace(k)
	if k == "" {
		return "", rtErr(ctx, pos, "%s expects non-empty key", sig)
	}
	return k, nil
}

func mapKey(ctx *Ctx, pos Pos, key, sig string) (string, error) {
	k := strings.TrimSpace(key)
	if k == "" {
		return "", rtErr(ctx, pos, "%s expects non-empty key", sig)
	}
	return k, nil
}

func lowerKey(key string) string {
	return strings.ToLower(key)
}

func scalarStr(ctx *Ctx, pos Pos, v Value, sig string) (string, error) {
	switch v.K {
	case VStr, VNum, VBool:
		return toStr(ctx, pos, v)
	default:
		return "", rtErr(ctx, pos, "%s expects string/number/bool", sig)
	}
}

func strArg(ctx *Ctx, pos Pos, v Value, sig string) (string, error) {
	s, err := toStr(ctx, pos, v)
	if err != nil {
		return "", err
	}
	if err := chkStr(ctx, pos, s); err != nil {
		return "", err
	}
	return s, nil
}

func numArg(ctx *Ctx, pos Pos, v Value, sig string) (float64, error) {
	if v.K != VNum {
		return 0, rtErr(ctx, pos, "%s expects number", sig)
	}
	return v.N, nil
}

func listArg(ctx *Ctx, pos Pos, v Value, sig string) ([]Value, error) {
	if v.K == VNull {
		return nil, nil
	}

	if v.K != VList {
		return nil, rtErr(ctx, pos, "%s expects list", sig)
	}

	if err := chkList(ctx, pos, len(v.L)); err != nil {
		return nil, err
	}
	return v.L, nil
}

func reqMsg(ctx *Ctx, pos Pos, args []Value) (string, error) {
	if len(args) < 2 {
		return "", nil
	}

	s, err := toStr(ctx, pos, args[1])
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
	return rtErr(ctx, pos, "%s", msg)
}

func chkStr(ctx *Ctx, pos Pos, s string) error {
	if ctx == nil || ctx.Lim.MaxStr <= 0 {
		return nil
	}
	if len(s) > ctx.Lim.MaxStr {
		return rtErr(ctx, pos, "string too long")
	}
	return nil
}

func chkList(ctx *Ctx, pos Pos, n int) error {
	if ctx == nil || ctx.Lim.MaxList <= 0 {
		return nil
	}
	if n > ctx.Lim.MaxList {
		return rtErr(ctx, pos, "list too large")
	}
	return nil
}

func chkDict(ctx *Ctx, pos Pos, n int) error {
	if ctx == nil || ctx.Lim.MaxDict <= 0 {
		return nil
	}
	if n > ctx.Lim.MaxDict {
		return rtErr(ctx, pos, "dict too large")
	}
	return nil
}

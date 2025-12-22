package rts

import "strings"

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
	if len(in) == 0 {
		return map[string]Value{}
	}
	out := make(map[string]Value, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
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

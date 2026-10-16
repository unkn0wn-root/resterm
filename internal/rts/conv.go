package rts

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
)

// ToStr returns v as text. A string is returned as is and null is empty.
// Bools, numbers, lists, dicts and objects with a JSON form are written as
// JSON, and NaN and infinities as NaN, +Inf and -Inf. Other values are an
// error.
func ToStr(ctx *Ctx, pos Pos, v Value) (string, error) {
	switch v.K {
	case VStr:
		return v.S, nil
	case VNum:
		if b, err := json.Marshal(v.N); err == nil {
			return string(b), nil
		}
		// JSON has no NaN or infinities.
		return strconv.FormatFloat(v.N, 'g', -1, 64), nil
	case VBool:
		if v.B {
			return "true", nil
		}
		return "false", nil
	case VNull:
		return "", nil
	case VList, VDict:
		return jsonStr(ctx, pos, v)
	case VObj:
		if _, ok := v.O.(InterfaceValuer); ok {
			return jsonStr(ctx, pos, v)
		}
		return "", Errf(ctx, pos, "cannot stringify %v", v.K)
	default:
		return "", Errf(ctx, pos, "cannot stringify %v", v.K)
	}
}

func jsonStr(ctx *Ctx, pos Pos, v Value) (string, error) {
	o := JSONOptions{EscapeHTML: true}
	if ctx != nil {
		o.Max = ctx.Lim.MaxStr
	}
	s, err := EncodeJSON(v, o)
	if err != nil {
		return "", Errf(ctx, pos, "json encode failed")
	}
	if err := CheckStr(ctx, pos, s); err != nil {
		return "", err
	}
	return s, nil
}

// ScalarStr converts string, number, and bool values to strings.
func ScalarStr(ctx *Ctx, pos Pos, v Value, sig string) (string, error) {
	switch v.K {
	case VStr, VNum, VBool:
		return ToStr(ctx, pos, v)
	default:
		return "", Errf(ctx, pos, "%s expects string/number/bool", sig)
	}
}

func ValueString(ctx *Ctx, pos Pos, v Value) (string, error) {
	return ToStr(ctx, pos, v)
}

func ToIface(v Value) any {
	switch v.K {
	case VNull:
		return nil
	case VBool:
		return v.B
	case VNum:
		return v.N
	case VStr:
		return v.S
	case VList:
		out := make([]any, 0, len(v.L))
		for _, it := range v.L {
			out = append(out, ToIface(it))
		}
		return out
	case VDict:
		out := make(map[string]any, len(v.M))
		for k, it := range v.M {
			out[k] = ToIface(it)
		}
		return out
	case VObj:
		if v.O != nil {
			if t, ok := v.O.(InterfaceValuer); ok {
				return t.ToInterface()
			}
		}
		return fmt.Sprintf("<%v>", v.K)
	default:
		return fmt.Sprintf("<%v>", v.K)
	}
}

func FromIface(ctx *Ctx, pos Pos, v any) (Value, error) {
	switch t := v.(type) {
	case nil:
		return Null(), nil
	case bool:
		return Bool(t), nil
	case float64:
		return Num(t), nil
	case string:
		if ctx != nil && ctx.Lim.MaxStr > 0 && len(t) > ctx.Lim.MaxStr {
			return Null(), Errf(ctx, pos, "string too long")
		}
		return Str(t), nil
	case []any:
		if ctx != nil && ctx.Lim.MaxList > 0 && len(t) > ctx.Lim.MaxList {
			return Null(), Errf(ctx, pos, "list too large")
		}
		out := make([]Value, 0, len(t))
		for _, it := range t {
			v2, err := FromIface(ctx, pos, it)
			if err != nil {
				return Null(), err
			}
			out = append(out, v2)
		}
		return List(out), nil
	case map[string]any:
		if err := CheckDict(ctx, pos, len(t)); err != nil {
			return Null(), err
		}
		out := make(map[string]Value, len(t))
		for _, k := range slices.Sorted(maps.Keys(t)) {
			if err := CheckStr(ctx, pos, k); err != nil {
				return Null(), Errf(ctx, pos, "dictionary key %q: string too long", k)
			}
			v2, err := FromIface(ctx, pos, t[k])
			if err != nil {
				return Null(), err
			}
			out[k] = v2
		}
		return Dict(out), nil
	default:
		return Null(), Errf(ctx, pos, "unsupported json value")
	}
}

func toNum(pos Pos, v Value) (float64, error) {
	if v.K != VNum {
		return 0, Errf(nil, pos, "expected number")
	}
	return v.N, nil
}

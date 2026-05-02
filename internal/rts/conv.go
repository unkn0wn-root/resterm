package rts

import (
	"encoding/json"
	"fmt"
	"strconv"
)

func Key(pos Pos, v Value) (string, error) {
	if v.K != VStr {
		return "", Errf(nil, pos, "expected string key")
	}
	return v.S, nil
}

func ToStr(ctx *Ctx, pos Pos, v Value) (string, error) {
	switch v.K {
	case VStr:
		return v.S, nil
	case VNum:
		return strconv.FormatFloat(v.N, 'g', -1, 64), nil
	case VBool:
		if v.B {
			return "true", nil
		}
		return "false", nil
	case VNull:
		return "", nil
	case VList, VDict:
		data, err := json.Marshal(ToIface(v))
		if err != nil {
			return "", Errf(ctx, pos, "json encode failed")
		}
		if ctx != nil && ctx.Lim.MaxStr > 0 && len(data) > ctx.Lim.MaxStr {
			return "", Errf(ctx, pos, "string too long")
		}
		return string(data), nil
	case VObj:
		if v.O != nil {
			if _, ok := v.O.(InterfaceValuer); ok {
				data, err := json.Marshal(ToIface(v))
				if err != nil {
					return "", Errf(ctx, pos, "json encode failed")
				}
				if ctx != nil && ctx.Lim.MaxStr > 0 && len(data) > ctx.Lim.MaxStr {
					return "", Errf(ctx, pos, "string too long")
				}
				return string(data), nil
			}
		}
		return "", Errf(ctx, pos, "cannot stringify %v", v.K)
	default:
		return "", Errf(ctx, pos, "cannot stringify %v", v.K)
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
		if ctx != nil && ctx.Lim.MaxDict > 0 && len(t) > ctx.Lim.MaxDict {
			return Null(), Errf(ctx, pos, "dict too large")
		}
		out := make(map[string]Value, len(t))
		for k, it := range t {
			v2, err := FromIface(ctx, pos, it)
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

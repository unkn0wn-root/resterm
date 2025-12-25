package rts

import (
	"encoding/json"
	"fmt"
	"strconv"
)

func toNum(pos Pos, v Value) (float64, error) {
	if v.K != VNum {
		return 0, rtErr(nil, pos, "expected number")
	}
	return v.N, nil
}

func toKey(pos Pos, v Value) (string, error) {
	if v.K != VStr {
		return "", rtErr(nil, pos, "expected string key")
	}
	return v.S, nil
}

func toStr(ctx *Ctx, pos Pos, v Value) (string, error) {
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
		data, err := json.Marshal(toIface(v))
		if err != nil {
			return "", rtErr(ctx, pos, "json encode failed")
		}
		if ctx != nil && ctx.Lim.MaxStr > 0 && len(data) > ctx.Lim.MaxStr {
			return "", rtErr(ctx, pos, "string too long")
		}
		return string(data), nil
	case VObj:
		if v.O != nil {
			if _, ok := v.O.(interface{ ToInterface() any }); ok {
				data, err := json.Marshal(toIface(v))
				if err != nil {
					return "", rtErr(ctx, pos, "json encode failed")
				}
				if ctx != nil && ctx.Lim.MaxStr > 0 && len(data) > ctx.Lim.MaxStr {
					return "", rtErr(ctx, pos, "string too long")
				}
				return string(data), nil
			}
		}
		return "", rtErr(ctx, pos, "cannot stringify %v", v.K)
	default:
		return "", rtErr(ctx, pos, "cannot stringify %v", v.K)
	}
}

func ValueString(ctx *Ctx, pos Pos, v Value) (string, error) {
	return toStr(ctx, pos, v)
}

func toIface(v Value) any {
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
			out = append(out, toIface(it))
		}
		return out
	case VDict:
		out := make(map[string]any, len(v.M))
		for k, it := range v.M {
			out[k] = toIface(it)
		}
		return out
	case VObj:
		if v.O != nil {
			if t, ok := v.O.(interface{ ToInterface() any }); ok {
				return t.ToInterface()
			}
		}
		return fmt.Sprintf("<%v>", v.K)
	default:
		return fmt.Sprintf("<%v>", v.K)
	}
}

func fromIface(ctx *Ctx, pos Pos, v any) (Value, error) {
	switch t := v.(type) {
	case nil:
		return Null(), nil
	case bool:
		return Bool(t), nil
	case float64:
		return Num(t), nil
	case string:
		if ctx != nil && ctx.Lim.MaxStr > 0 && len(t) > ctx.Lim.MaxStr {
			return Null(), rtErr(ctx, pos, "string too long")
		}
		return Str(t), nil
	case []any:
		if ctx != nil && ctx.Lim.MaxList > 0 && len(t) > ctx.Lim.MaxList {
			return Null(), rtErr(ctx, pos, "list too large")
		}
		out := make([]Value, 0, len(t))
		for _, it := range t {
			v2, err := fromIface(ctx, pos, it)
			if err != nil {
				return Null(), err
			}
			out = append(out, v2)
		}
		return List(out), nil
	case map[string]any:
		if ctx != nil && ctx.Lim.MaxDict > 0 && len(t) > ctx.Lim.MaxDict {
			return Null(), rtErr(ctx, pos, "dict too large")
		}
		out := make(map[string]Value, len(t))
		for k, it := range t {
			v2, err := fromIface(ctx, pos, it)
			if err != nil {
				return Null(), err
			}
			out[k] = v2
		}
		return Dict(out), nil
	default:
		return Null(), rtErr(ctx, pos, "unsupported json value")
	}
}

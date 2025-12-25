package rts

import (
	"encoding/json"
	"strings"
)

func stdlibJSONParse(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, "json.parse(text)"); err != nil {
		return Null(), err
	}

	if args[0].K != VStr {
		return Null(), rtErr(ctx, pos, "json.parse(text) expects string")
	}

	txt := args[0].S
	if ctx != nil && ctx.Lim.MaxStr > 0 && len(txt) > ctx.Lim.MaxStr {
		return Null(), rtErr(ctx, pos, "text too long")
	}

	var raw any
	if err := json.Unmarshal([]byte(txt), &raw); err != nil {
		return Null(), rtErr(ctx, pos, "invalid json")
	}
	return fromIface(ctx, pos, raw)
}

func stdlibJSONStringify(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCountRange(ctx, pos, args, 1, 2, "json.stringify(value[, indent])"); err != nil {
		return Null(), err
	}

	raw, err := jsonIface(ctx, pos, args[0])
	if err != nil {
		return Null(), err
	}

	var (
		data   []byte
		indent string
	)
	if len(args) == 2 {
		indent, err = jsonIndent(ctx, pos, args[1])
		if err != nil {
			return Null(), err
		}
	}
	if indent == "" {
		data, err = json.Marshal(raw)
	} else {
		data, err = json.MarshalIndent(raw, "", indent)
	}
	if err != nil {
		return Null(), rtErr(ctx, pos, "json stringify failed")
	}
	if ctx != nil && ctx.Lim.MaxStr > 0 && len(data) > ctx.Lim.MaxStr {
		return Null(), rtErr(ctx, pos, "string too long")
	}
	return Str(string(data)), nil
}

func stdlibJSONGet(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCountRange(ctx, pos, args, 1, 2, "json.get(value[, path])"); err != nil {
		return Null(), err
	}
	raw, err := jsonIface(ctx, pos, args[0])
	if err != nil {
		return Null(), err
	}
	path := ""
	if len(args) == 2 {
		p, err := strArg(ctx, pos, args[1], "json.get(value[, path])")
		if err != nil {
			return Null(), err
		}
		path = p
	}
	if path == "" {
		return fromIface(ctx, pos, raw)
	}
	val, ok := jsonGet(raw, path)
	if !ok {
		return Null(), nil
	}
	return fromIface(ctx, pos, val)
}

func jsonIndent(ctx *Ctx, pos Pos, v Value) (string, error) {
	switch v.K {
	case VStr:
		return v.S, nil
	case VNum:
		n := int(v.N)
		if n < 0 {
			return "", rtErr(ctx, pos, "indent must be >= 0")
		}
		if n == 0 {
			return "", nil
		}
		if n > 32 {
			return "", rtErr(ctx, pos, "indent too large")
		}
		return strings.Repeat(" ", n), nil
	default:
		return "", rtErr(ctx, pos, "indent must be string or number")
	}
}

func jsonIface(ctx *Ctx, pos Pos, v Value) (any, error) {
	switch v.K {
	case VNull:
		return nil, nil
	case VBool:
		return v.B, nil
	case VNum:
		return v.N, nil
	case VStr:
		return v.S, nil
	case VList:
		if ctx != nil && ctx.Lim.MaxList > 0 && len(v.L) > ctx.Lim.MaxList {
			return nil, rtErr(ctx, pos, "list too large")
		}
		out := make([]any, 0, len(v.L))
		for _, it := range v.L {
			val, err := jsonIface(ctx, pos, it)
			if err != nil {
				return nil, err
			}
			out = append(out, val)
		}
		return out, nil
	case VDict:
		if ctx != nil && ctx.Lim.MaxDict > 0 && len(v.M) > ctx.Lim.MaxDict {
			return nil, rtErr(ctx, pos, "dict too large")
		}
		out := make(map[string]any, len(v.M))
		for k, it := range v.M {
			val, err := jsonIface(ctx, pos, it)
			if err != nil {
				return nil, err
			}
			out[k] = val
		}
		return out, nil
	case VObj:
		if v.O != nil {
			if t, ok := v.O.(interface{ ToInterface() any }); ok {
				return t.ToInterface(), nil
			}
		}
		return nil, rtErr(ctx, pos, "json stringify unsupported object")
	default:
		return nil, rtErr(ctx, pos, "json stringify unsupported type")
	}
}

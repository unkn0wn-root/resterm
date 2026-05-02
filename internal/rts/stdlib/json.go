package stdlib

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/rts"
)

const (
	sigJSONFile      = "json.file(path)"
	sigJSONParse     = "json.parse(text)"
	sigJSONStringify = "json.stringify(value[, indent])"
	sigJSONGet       = "json.get(value[, path])"
	sigJSONHas       = "json.has(value, path)"
)

var jsonSpec = nsSpec{name: "json", top: true, fns: map[string]rts.NativeFunc{
	"file":      jsonFile,
	"parse":     jsonParse,
	"stringify": jsonStringify,
	"get":       jsonGet,
	"has":       jsonHas,
}}

func jsonFile(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigJSONFile)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	if ctx == nil || ctx.ReadFile == nil {
		return rts.Null(), rts.Errf(ctx, pos, "file access not available")
	}

	p, err := na.ToStr(0)
	if err != nil {
		return rts.Null(), err
	}

	path := p
	if !filepath.IsAbs(path) && ctx.BaseDir != "" {
		path = filepath.Join(ctx.BaseDir, path)
	}

	data, err := ctx.ReadFile(path)
	if err != nil {
		return rts.Null(), rts.Errf(ctx, pos, "file read failed")
	}

	if ctx.Lim.MaxStr > 0 && len(data) > ctx.Lim.MaxStr {
		return rts.Null(), rts.Errf(ctx, pos, "file too large")
	}

	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return rts.Null(), rts.Errf(ctx, pos, "invalid json")
	}

	return rts.FromIface(ctx, pos, raw)
}

func jsonParse(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigJSONParse)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	if na.Arg(0).K != rts.VStr {
		return rts.Null(), rts.Errf(ctx, pos, "%s expects string", sigJSONParse)
	}

	txt := na.Arg(0).S
	if ctx != nil && ctx.Lim.MaxStr > 0 && len(txt) > ctx.Lim.MaxStr {
		return rts.Null(), rts.Errf(ctx, pos, "text too long")
	}

	var raw any
	if err := json.Unmarshal([]byte(txt), &raw); err != nil {
		return rts.Null(), rts.Errf(ctx, pos, "invalid json")
	}
	return rts.FromIface(ctx, pos, raw)
}

func jsonStringify(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigJSONStringify)
	if err := na.CountRange(1, 2); err != nil {
		return rts.Null(), err
	}

	raw, err := jsonIface(ctx, pos, na.Arg(0))
	if err != nil {
		return rts.Null(), err
	}

	var (
		data   []byte
		indent string
	)
	if na.Has(1) {
		indent, err = jsonIndent(ctx, pos, na.Arg(1))
		if err != nil {
			return rts.Null(), err
		}
	}

	if indent == "" {
		data, err = json.Marshal(raw)
	} else {
		data, err = json.MarshalIndent(raw, "", indent)
	}

	if err != nil {
		return rts.Null(), rts.Errf(ctx, pos, "json stringify failed")
	}

	if ctx != nil && ctx.Lim.MaxStr > 0 && len(data) > ctx.Lim.MaxStr {
		return rts.Null(), rts.Errf(ctx, pos, "string too long")
	}

	return rts.Str(string(data)), nil
}

func jsonGet(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigJSONGet)
	if err := na.CountRange(1, 2); err != nil {
		return rts.Null(), err
	}

	raw, err := jsonIface(ctx, pos, na.Arg(0))
	if err != nil {
		return rts.Null(), err
	}

	path := ""
	if na.Has(1) {
		p, err := na.Str(1)
		if err != nil {
			return rts.Null(), err
		}
		path = p
	}
	if path == "" {
		return rts.FromIface(ctx, pos, raw)
	}

	val, ok := rts.JSONPathGet(raw, path)
	if !ok {
		return rts.Null(), nil
	}

	return rts.FromIface(ctx, pos, val)
}

func jsonHas(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigJSONHas)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	raw, err := jsonIface(ctx, pos, na.Arg(0))
	if err != nil {
		return rts.Null(), err
	}

	path, err := na.Str(1)
	if err != nil {
		return rts.Null(), err
	}

	_, ok := rts.JSONPathGet(raw, path)
	return rts.Bool(ok), nil
}

func jsonIndent(ctx *rts.Ctx, pos rts.Pos, v rts.Value) (string, error) {
	switch v.K {
	case rts.VStr:
		return v.S, nil
	case rts.VNum:
		n := int(v.N)
		if n < 0 {
			return "", rts.Errf(ctx, pos, "indent must be >= 0")
		}
		if n == 0 {
			return "", nil
		}
		if n > 32 {
			return "", rts.Errf(ctx, pos, "indent too large")
		}
		return strings.Repeat(" ", n), nil
	default:
		return "", rts.Errf(ctx, pos, "indent must be string or number")
	}
}

func jsonIface(ctx *rts.Ctx, pos rts.Pos, v rts.Value) (any, error) {
	switch v.K {
	case rts.VNull:
		return nil, nil
	case rts.VBool:
		return v.B, nil
	case rts.VNum:
		return v.N, nil
	case rts.VStr:
		return v.S, nil
	case rts.VList:
		if ctx != nil && ctx.Lim.MaxList > 0 && len(v.L) > ctx.Lim.MaxList {
			return nil, rts.Errf(ctx, pos, "list too large")
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
	case rts.VDict:
		if ctx != nil && ctx.Lim.MaxDict > 0 && len(v.M) > ctx.Lim.MaxDict {
			return nil, rts.Errf(ctx, pos, "dict too large")
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
	case rts.VObj:
		if v.O != nil {
			if t, ok := v.O.(rts.InterfaceValuer); ok {
				return t.ToInterface(), nil
			}
		}
		return nil, rts.Errf(ctx, pos, "json stringify unsupported object")
	default:
		return nil, rts.Errf(ctx, pos, "json stringify unsupported type")
	}
}

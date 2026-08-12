package stdlib

import (
	"github.com/unkn0wn-root/resterm/internal/httpheader"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rts/native"
)

const (
	sigHeadersNormalize = "headers.normalize(headers)"
	sigHeadersGet       = "headers.get(headers, name)"
	sigHeadersHas       = "headers.has(headers, name)"
	sigHeadersSet       = "headers.set(headers, name, value)"
	sigHeadersRemove    = "headers.remove(headers, name)"
	sigHeadersMerge     = "headers.merge(a, b)"
)

var (
	headersNormalizeDef = native.Fn1(
		"headers.normalize", sigHeadersNormalize, headerBlockArg,
		func(call native.Call, h httpheader.Values) (rts.Value, error) {
			return headerBlockValue(call, h)
		},
	)
	headersGetDef = native.Fn2(
		"headers.get", sigHeadersGet, headerBlockArg, headerNameArg,
		func(_ native.Call, h httpheader.Values, name httpheader.Name) (rts.Value, error) {
			if len(h[name.Key()]) == 0 {
				return rts.Null(), nil
			}
			return rts.Str(h[name.Key()][0]), nil
		},
	)
	headersHasDef = native.Fn2(
		"headers.has", sigHeadersHas, headerBlockArg, headerNameArg,
		func(_ native.Call, h httpheader.Values, name httpheader.Name) (rts.Value, error) {
			return rts.Bool(len(h[name.Key()]) > 0), nil
		},
	)
	headersSetDef = native.Fn3(
		"headers.set", sigHeadersSet, headerBlockArg, headerNameArg, native.StringValues,
		func(call native.Call, h httpheader.Values, name httpheader.Name, vals []string) (rts.Value, error) {
			h[name.Key()] = vals
			return headerBlockValue(call, h)
		},
	)
	headersRemoveDef = native.Fn2(
		"headers.remove", sigHeadersRemove, headerBlockArg, headerNameArg,
		func(call native.Call, h httpheader.Values, name httpheader.Name) (rts.Value, error) {
			delete(h, name.Key())
			return headerBlockValue(call, h)
		},
	)
	headersMergeDef = native.Fn2(
		"headers.merge", sigHeadersMerge, headerBlockArg, headerPatchArg,
		func(call native.Call, h httpheader.Values, patch headerPatch) (rts.Value, error) {
			for name, edit := range patch {
				if edit.del {
					delete(h, name)
					continue
				}
				h[name] = edit.vals
			}
			return headerBlockValue(call, h)
		},
	)
)

var headersSpec = nsSpec{name: "headers", top: true, fns: map[string]rts.NativeFunc{
	"get":       headersGetDef.Func(),
	"has":       headersHasDef.Func(),
	"set":       headersSetDef.Func(),
	"remove":    headersRemoveDef.Func(),
	"merge":     headersMergeDef.Func(),
	"normalize": headersNormalizeDef.Func(),
}}

type headerEdit struct {
	vals []string
	del  bool
}

type headerPatch map[string]headerEdit

func headerBlockArg(call native.Call, v rts.Value) (httpheader.Values, error) {
	m, err := native.Dict(call, v)
	if err != nil {
		return nil, err
	}
	keys, err := headerKeys(call, m)
	if err != nil {
		return nil, err
	}
	out := make(httpheader.Values, len(m))
	for _, k := range keys {
		vals, err := native.StringValues(call, m[k.Source])
		if err != nil {
			return nil, err
		}
		out[k.Name.Key()] = vals
	}
	return out, nil
}

func headerPatchArg(call native.Call, v rts.Value) (headerPatch, error) {
	m, err := native.Dict(call, v)
	if err != nil {
		return nil, err
	}
	keys, err := headerKeys(call, m)
	if err != nil {
		return nil, err
	}
	out := make(headerPatch, len(m))
	for _, k := range keys {
		v := m[k.Source]
		if v.K == rts.VNull {
			out[k.Name.Key()] = headerEdit{del: true}
			continue
		}
		vals, err := native.StringValues(call, v)
		if err != nil {
			return nil, err
		}
		out[k.Name.Key()] = headerEdit{vals: vals}
	}
	return out, nil
}

func headerKeys(call native.Call, m map[string]rts.Value) ([]httpheader.Named, error) {
	keys, err := httpheader.Keys(m)
	if err != nil {
		return nil, call.Errorf("%s: %v", call.Sig, err)
	}
	for _, k := range keys {
		if err := rts.CheckStr(call.Ctx, call.Pos, k.Source); err != nil {
			return nil, err
		}
	}
	return keys, nil
}

func headerNameArg(call native.Call, v rts.Value) (httpheader.Name, error) {
	name, err := native.String(call, v)
	if err != nil {
		return httpheader.Name{}, err
	}
	n, err := httpheader.Parse(name)
	if err != nil {
		return httpheader.Name{}, call.Errorf("%s expects an HTTP header name, got %q", call.Sig, name)
	}
	return n, nil
}

func headerBlockValue(call native.Call, h httpheader.Values) (rts.Value, error) {
	return native.StringValuesDict(call.Ctx, call.Pos, h)
}

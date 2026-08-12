package stdlib

import (
	"maps"
	"slices"

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

type headerKey struct {
	src  string
	name httpheader.Name
}

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
		vals, err := native.StringValues(call, m[k.src])
		if err != nil {
			return nil, err
		}
		out[k.name.Key()] = vals
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
		v := m[k.src]
		if v.K == rts.VNull {
			out[k.name.Key()] = headerEdit{del: true}
			continue
		}
		vals, err := native.StringValues(call, v)
		if err != nil {
			return nil, err
		}
		out[k.name.Key()] = headerEdit{vals: vals}
	}
	return out, nil
}

func headerKeys(call native.Call, m map[string]rts.Value) ([]headerKey, error) {
	names := slices.Sorted(maps.Keys(m))
	out := make([]headerKey, 0, len(names))
	seen := make(map[string]string, len(names))
	for _, name := range names {
		if err := rts.CheckStr(call.Ctx, call.Pos, name); err != nil {
			return nil, err
		}
		n, err := httpheader.Parse(name)
		if err != nil {
			return nil, call.Errorf("%s: %v", call.Sig, err)
		}
		key := n.Key()
		if first, ok := seen[key]; ok {
			return nil, call.Errorf(
				"%s: %q and %q are the same HTTP header",
				call.Sig,
				first,
				name,
			)
		}
		seen[key] = name
		out = append(out, headerKey{src: name, name: n})
	}
	return out, nil
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

package stdlib

import (
	"maps"
	"slices"

	"github.com/unkn0wn-root/resterm/internal/queryparams"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rts/native"
	"github.com/unkn0wn-root/resterm/internal/urltpl"
)

const (
	sigQueryParse   = "query.parse(rawQuery)"
	sigQueryFromURL = "query.fromURL(url)"
	sigQueryEncode  = "query.encode(query)"
	sigQueryMerge   = "query.merge(url, query)"
)

var (
	queryParseDef = native.Fn1("query.parse", sigQueryParse, native.String,
		func(call native.Call, raw string) (rts.Value, error) {
			vals, err := queryparams.Parse(raw)
			if err != nil {
				return rts.Null(), call.Errorf("invalid query: %v", err)
			}
			return queryValue(call, vals)
		},
	)
	queryFromURLDef = native.Fn1("query.fromURL", sigQueryFromURL, native.String,
		func(call native.Call, raw string) (rts.Value, error) {
			vals, err := queryparams.FromURL(raw)
			if err != nil {
				return rts.Null(), call.Errorf("invalid URL: %v", err)
			}
			return queryValue(call, vals)
		},
	)
	queryEncodeDef = native.Fn1("query.encode", sigQueryEncode, queryArg,
		func(call native.Call, vals queryparams.Values) (rts.Value, error) {
			return native.StringValue(call.Ctx, call.Pos, queryparams.Encode(vals))
		},
	)
	queryMergeDef = native.Fn2("query.merge", sigQueryMerge, native.String, queryPatchArg,
		func(call native.Call, raw string, patch queryparams.Values) (rts.Value, error) {
			merged, err := urltpl.MergeQuery(raw, patch)
			if err != nil {
				return rts.Null(), call.Errorf("invalid URL: %v", err)
			}
			return native.StringValue(call.Ctx, call.Pos, merged)
		},
	)
)

var querySpec = nsSpec{name: "query", top: true, fns: map[string]rts.NativeFunc{
	"parse":   queryParseDef.Func(),
	"fromURL": queryFromURLDef.Func(),
	"encode":  queryEncodeDef.Func(),
	"merge":   queryMergeDef.Func(),
}}

func queryArg(call native.Call, v rts.Value) (queryparams.Values, error) {
	m, err := native.Dict(call, v)
	if err != nil {
		return nil, err
	}
	out := make(queryparams.Values, len(m))
	for _, name := range slices.Sorted(maps.Keys(m)) {
		if err := rts.CheckStr(call.Ctx, call.Pos, name); err != nil {
			return nil, err
		}
		vals, err := native.StringValues(call, m[name])
		if err != nil {
			return nil, err
		}
		out[name] = vals
	}
	return out, nil
}

func queryPatchArg(call native.Call, v rts.Value) (queryparams.Values, error) {
	m, err := native.Dict(call, v)
	if err != nil {
		return nil, err
	}
	out := make(queryparams.Values, len(m))
	for _, name := range slices.Sorted(maps.Keys(m)) {
		if err := rts.CheckStr(call.Ctx, call.Pos, name); err != nil {
			return nil, err
		}
		v := m[name]
		if v.K == rts.VNull {
			out[name] = []string{}
			continue
		}
		vals, err := native.StringValues(call, v)
		if err != nil {
			return nil, err
		}
		out[name] = vals
	}
	return out, nil
}

func queryValue(call native.Call, vals queryparams.Values) (rts.Value, error) {
	return native.StringValuesDict(call.Ctx, call.Pos, vals)
}

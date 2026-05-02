package stdlib

import (
	"net/url"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/urltpl"
)

const (
	sigQueryParse  = "query.parse(urlOrQuery)"
	sigQueryEncode = "query.encode(map)"
	sigQueryMerge  = "query.merge(url, map)"
)

var querySpec = nsSpec{name: "query", top: true, fns: map[string]rts.NativeFunc{
	"parse":  queryParse,
	"encode": queryEncode,
	"merge":  queryMerge,
}}

func queryParse(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigQueryParse)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}
	if na.Arg(0).K != rts.VStr {
		return rts.Null(), rts.Errf(ctx, pos, "%s expects string", sigQueryParse)
	}
	txt := strings.TrimSpace(na.Arg(0).S)
	if txt == "" {
		return rts.Dict(map[string]rts.Value{}), nil
	}
	vals, err := rts.ParseQuery(txt)
	if err != nil {
		return rts.Null(), rts.Errf(ctx, pos, "invalid query")
	}
	return rts.Dict(rts.ValuesDict(vals)), nil
}

func queryEncode(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigQueryEncode)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}
	m, err := na.Dict(0)
	if err != nil {
		return rts.Null(), err
	}
	vals := url.Values{}
	for k, v := range m {
		key, err := na.MapKey(k)
		if err != nil {
			return rts.Null(), err
		}
		items, err := queryValues(ctx, pos, v)
		if err != nil {
			return rts.Null(), err
		}
		for _, it := range items {
			vals.Add(key, it)
		}
	}
	return rts.Str(vals.Encode()), nil
}

func queryMerge(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigQueryMerge)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	if na.Arg(0).K != rts.VStr {
		return rts.Null(), rts.Errf(ctx, pos, "%s expects string url", sigQueryMerge)
	}

	m, err := na.Dict(1)
	if err != nil {
		return rts.Null(), err
	}

	raw := strings.TrimSpace(na.Arg(0).S)
	patch := make(map[string][]string, len(m))
	for k, v := range m {
		key, err := na.MapKey(k)
		if err != nil {
			return rts.Null(), err
		}
		if v.K == rts.VNull {
			patch[key] = nil
			continue
		}

		items, err := queryValues(ctx, pos, v)
		if err != nil {
			return rts.Null(), err
		}

		if len(items) == 0 {
			patch[key] = nil
			continue
		}
		patch[key] = items
	}
	merged, err := urltpl.MergeQuery(raw, patch)
	if err != nil {
		return rts.Null(), rts.Errf(ctx, pos, "invalid url")
	}
	return rts.Str(merged), nil
}

func queryValues(ctx *rts.Ctx, pos rts.Pos, v rts.Value) ([]string, error) {
	switch v.K {
	case rts.VNull:
		return nil, nil
	case rts.VStr, rts.VNum, rts.VBool:
		s, err := rts.ScalarStr(ctx, pos, v, "query values")
		if err != nil {
			return nil, err
		}
		return []string{s}, nil
	case rts.VList:
		if len(v.L) == 0 {
			return nil, nil
		}
		out := make([]string, 0, len(v.L))
		for _, it := range v.L {
			if it.K == rts.VNull {
				continue
			}
			s, err := rts.ScalarStr(ctx, pos, it, "query values")
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, rts.Errf(ctx, pos, "query values must be string/number/bool/list")
	}
}

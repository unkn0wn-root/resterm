package rts

import (
	"net/url"
	"strings"
)

func stdlibQueryParse(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, "query.parse(urlOrQuery)"); err != nil {
		return Null(), err
	}
	if args[0].K != VStr {
		return Null(), rtErr(ctx, pos, "query.parse(urlOrQuery) expects string")
	}
	txt := strings.TrimSpace(args[0].S)
	if txt == "" {
		return Dict(map[string]Value{}), nil
	}
	vals, err := parseQuery(txt)
	if err != nil {
		return Null(), rtErr(ctx, pos, "invalid query")
	}
	return Dict(valuesDict(vals)), nil
}

func stdlibQueryEncode(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, "query.encode(map)"); err != nil {
		return Null(), err
	}
	m, err := dictArg(ctx, pos, args[0], "query.encode(map)")
	if err != nil {
		return Null(), err
	}
	vals := url.Values{}
	for k, v := range m {
		key, err := mapKey(ctx, pos, k, "query.encode(map)")
		if err != nil {
			return Null(), err
		}
		items, err := queryValues(ctx, pos, v)
		if err != nil {
			return Null(), err
		}
		for _, it := range items {
			vals.Add(key, it)
		}
	}
	return Str(vals.Encode()), nil
}

func stdlibQueryMerge(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 2, "query.merge(url, map)"); err != nil {
		return Null(), err
	}
	if args[0].K != VStr {
		return Null(), rtErr(ctx, pos, "query.merge(url, map) expects string url")
	}
	m, err := dictArg(ctx, pos, args[1], "query.merge(url, map)")
	if err != nil {
		return Null(), err
	}
	u, err := url.Parse(strings.TrimSpace(args[0].S))
	if err != nil {
		return Null(), rtErr(ctx, pos, "invalid url")
	}
	vals := u.Query()
	for k, v := range m {
		key, err := mapKey(ctx, pos, k, "query.merge(url, map)")
		if err != nil {
			return Null(), err
		}
		if v.K == VNull {
			vals.Del(key)
			continue
		}
		items, err := queryValues(ctx, pos, v)
		if err != nil {
			return Null(), err
		}
		vals.Del(key)
		for _, it := range items {
			vals.Add(key, it)
		}
	}
	u.RawQuery = vals.Encode()
	return Str(u.String()), nil
}

func queryValues(ctx *Ctx, pos Pos, v Value) ([]string, error) {
	switch v.K {
	case VNull:
		return nil, nil
	case VStr, VNum, VBool:
		s, err := scalarStr(ctx, pos, v, "query values")
		if err != nil {
			return nil, err
		}
		return []string{s}, nil
	case VList:
		if len(v.L) == 0 {
			return nil, nil
		}
		out := make([]string, 0, len(v.L))
		for _, it := range v.L {
			if it.K == VNull {
				continue
			}
			s, err := scalarStr(ctx, pos, it, "query values")
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, rtErr(ctx, pos, "query values must be string/number/bool/list")
	}
}

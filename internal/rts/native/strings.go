package native

import (
	"maps"
	"slices"

	"github.com/unkn0wn-root/resterm/internal/rts"
)

// StringValue encodes a Go string after applying RTS resource limits.
func StringValue(ctx *rts.Ctx, pos rts.Pos, s string) (rts.Value, error) {
	if err := rts.CheckStr(ctx, pos, s); err != nil {
		return rts.Null(), err
	}
	return rts.Str(s), nil
}

// StringValues decodes the cardinality-based RTS representation of a string
// collection: one string is scalar, while zero or multiple strings are a list.
func StringValues(call Call, v rts.Value) ([]string, error) {
	if v.K == rts.VStr {
		s, err := String(call, v)
		if err != nil {
			return nil, err
		}
		return []string{s}, nil
	}
	if v.K == rts.VList {
		return StringList(call, v)
	}
	return nil, call.Errorf("%s expects string or list<string>", call.Sig)
}

// encodeStrings encodes strings using the cardinality representation:
// one value is a scalar; zero or multiple values are a list.
func encodeStrings(ctx *rts.Ctx, pos rts.Pos, vals []string) (rts.Value, error) {
	if len(vals) == 1 {
		return StringValue(ctx, pos, vals[0])
	}
	if err := rts.CheckList(ctx, pos, len(vals)); err != nil {
		return rts.Null(), err
	}

	out := make([]rts.Value, len(vals))
	for i, s := range vals {
		v, err := StringValue(ctx, pos, s)
		if err != nil {
			return rts.Null(), err
		}
		out[i] = v
	}
	return rts.List(out), nil
}

// StringValuesDict encodes a map of string collections using the same
// cardinality rule for every value.
func StringValuesDict(
	ctx *rts.Ctx,
	pos rts.Pos,
	vals map[string][]string,
) (rts.Value, error) {
	if err := rts.CheckDict(ctx, pos, len(vals)); err != nil {
		return rts.Null(), err
	}

	// Sorted for the errors, not the result: a map holds no order, so without
	// this the reported name would depend on which key iteration reached first.
	out := make(map[string]rts.Value, len(vals))
	for _, name := range slices.Sorted(maps.Keys(vals)) {
		if err := rts.CheckStr(ctx, pos, name); err != nil {
			return rts.Null(), err
		}
		v, err := encodeStrings(ctx, pos, vals[name])
		if err != nil {
			return rts.Null(), err
		}
		out[name] = v
	}
	return rts.Dict(out), nil
}

package stdlib

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/unkn0wn-root/resterm/internal/jsonpath"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

const (
	sigJSONFile      = "json.file(path)"
	sigJSONParse     = "json.parse(text)"
	sigJSONStringify = "json.stringify(value[, indent])"
	sigJSONGet       = "json.get(value[, path])"
	sigJSONHas       = "json.has(value, path)"
	sigJSONEqual     = "json.equal(a, b)"
	sigJSONDiff      = "json.diff(got, want)"
)

var jsonSpec = nsSpec{name: "json", top: true, fns: map[string]rts.NativeFunc{
	"file":      jsonFile,
	"parse":     jsonParse,
	"stringify": jsonStringify,
	"get":       jsonGet,
	"has":       jsonHas,
	"equal":     jsonEqual,
	"diff":      jsonDiff,
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

	var indent string
	if na.Has(1) {
		in, err := jsonIndent(ctx, pos, na.Arg(1))
		if err != nil {
			return rts.Null(), err
		}
		indent = in
	}

	o := rts.JSONOptions{EscapeHTML: true, Indent: indent, Check: func(v rts.Value) error {
		switch v.K {
		case rts.VList:
			return rts.CheckList(ctx, pos, len(v.L))
		case rts.VDict:
			return rts.CheckDict(ctx, pos, len(v.M))
		case rts.VFunc, rts.VNative:
			return rts.Errf(ctx, pos, "json stringify unsupported type")
		case rts.VObj:
			if _, ok := v.O.(rts.InterfaceValuer); !ok {
				return rts.Errf(ctx, pos, "json stringify unsupported object")
			}
		}
		return nil
	}}
	if ctx != nil {
		o.Max = ctx.Lim.MaxStr
	}
	s, err := rts.EncodeJSON(na.Arg(0), o)
	switch {
	case errors.Is(err, rts.ErrNotJSON):
		return rts.Null(), rts.Errf(ctx, pos, "json stringify failed")
	case err != nil:
		return rts.Null(), err
	}
	if err := rts.CheckStr(ctx, pos, s); err != nil {
		return rts.Null(), err
	}
	return rts.Str(s), nil
}

func jsonGet(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigJSONGet)
	if err := na.CountRange(1, 2); err != nil {
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

	if v, ok := jsonLookup(na.Arg(0), path); ok {
		return v, nil
	}
	return rts.Null(), nil
}

func jsonHas(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigJSONHas)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	path, err := na.Str(1)
	if err != nil {
		return rts.Null(), err
	}

	_, ok := jsonLookup(na.Arg(0), path)
	return rts.Bool(ok), nil
}

func jsonLookup(v rts.Value, path string) (rts.Value, bool) {
	return jsonpath.Walk(v, path, func(v rts.Value, k string) (rts.Value, bool) {
		if v.K != rts.VDict {
			return rts.Null(), false
		}
		it, ok := v.M[k]
		return it, ok
	}, func(v rts.Value, i int) (rts.Value, bool) {
		if v.K != rts.VList || i < 0 || i >= len(v.L) {
			return rts.Null(), false
		}
		return v.L[i], true
	})
}

const (
	maxDiffs    = 100
	maxDiffText = 120
)

func jsonEqual(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigJSONEqual)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}
	for _, v := range args {
		if err := checkJSON(ctx, pos, sigJSONEqual, v); err != nil {
			return rts.Null(), err
		}
	}
	return rts.Bool(deepEqual(args[0], args[1])), nil
}

func jsonDiff(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigJSONDiff)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}
	for _, v := range args {
		if err := checkJSON(ctx, pos, sigJSONDiff, v); err != nil {
			return rts.Null(), err
		}
	}

	var d differ
	d.walk("$", args[0], args[1])
	switch n := d.n - maxDiffs; {
	case n == 1:
		d.out = append(d.out, rts.Str("and 1 more difference"))
	case n > 1:
		d.out = append(d.out, rts.Str(fmt.Sprintf("and %d more differences", n)))
	}
	return rts.List(d.out), nil
}

// Charge a step per container to bound traversal of shared children.
// This also bounds the comparison, which visits at most the same containers.
func checkJSON(ctx *rts.Ctx, pos rts.Pos, sig string, v rts.Value) error {
	switch v.K {
	case rts.VNum:
		if math.IsNaN(v.N) || math.IsInf(v.N, 0) {
			return rts.Errf(ctx, pos, "%s cannot compare %v", sig, v.N)
		}
	case rts.VList:
		if err := rts.Tick(ctx, pos); err != nil {
			return err
		}
		for _, it := range v.L {
			if err := checkJSON(ctx, pos, sig, it); err != nil {
				return err
			}
		}
	case rts.VDict:
		if err := rts.Tick(ctx, pos); err != nil {
			return err
		}
		for _, k := range slices.Sorted(maps.Keys(v.M)) {
			if err := checkJSON(ctx, pos, sig, v.M[k]); err != nil {
				return err
			}
		}
	case rts.VFunc, rts.VNative, rts.VObj:
		return rts.Errf(ctx, pos, "%s cannot compare %s", sig, typeName(v))
	}
	return nil
}

func deepEqual(a, b rts.Value) bool {
	switch {
	case a.K == rts.VList && b.K == rts.VList:
		return slices.EqualFunc(a.L, b.L, deepEqual)
	case a.K == rts.VDict && b.K == rts.VDict:
		return maps.EqualFunc(a.M, b.M, deepEqual)
	}
	return rts.ValueEqual(a, b)
}

type differ struct {
	out []rts.Value
	n   int
}

func (d *differ) walk(path string, got, want rts.Value) {
	switch {
	case got.K == rts.VList && want.K == rts.VList:
		for i := range max(len(got.L), len(want.L)) {
			p := path + "[" + strconv.Itoa(i) + "]"
			switch {
			case i >= len(got.L):
				d.add(p, nil, &want.L[i])
			case i >= len(want.L):
				d.add(p, &got.L[i], nil)
			default:
				d.walk(p, got.L[i], want.L[i])
			}
		}
	case got.K == rts.VDict && want.K == rts.VDict:
		keys := slices.AppendSeq(slices.Collect(maps.Keys(got.M)), maps.Keys(want.M))
		slices.Sort(keys)
		for _, k := range slices.Compact(keys) {
			g, inGot := got.M[k]
			w, inWant := want.M[k]
			p := jsonpath.Key(path, k)
			switch {
			case !inGot:
				d.add(p, nil, &w)
			case !inWant:
				d.add(p, &g, nil)
			default:
				d.walk(p, g, w)
			}
		}
	case !rts.ValueEqual(got, want):
		d.add(path, &got, &want)
	}
}

func (d *differ) add(path string, got, want *rts.Value) {
	d.n++
	if d.n > maxDiffs {
		return
	}
	var msg string
	switch {
	case got == nil:
		msg = path + ": missing, want " + diffText(*want)
	case want == nil:
		msg = path + ": unexpected " + diffText(*got)
	default:
		msg = path + ": got " + diffText(*got) + ", want " + diffText(*want)
	}
	d.out = append(d.out, rts.Str(msg))
}

func diffText(v rts.Value) string {
	// Validation already rejected values that JSON cannot encode.
	s, _ := rts.EncodeJSON(v, rts.JSONOptions{Max: maxDiffText})
	if len(s) <= maxDiffText {
		return s
	}
	i := maxDiffText
	for !utf8.RuneStart(s[i]) {
		i--
	}
	return s[:i] + "..."
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

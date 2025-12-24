package rts

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

type NativeFunc func(ctx *Ctx, pos Pos, args []Value) (Value, error)

type objMap struct {
	name string
	m    map[string]Value
}

func (o *objMap) TypeName() string { return o.name }

func (o *objMap) GetMember(name string) (Value, bool) {
	v, ok := o.m[name]
	return v, ok
}

func (o *objMap) CallMember(name string, args []Value) (Value, error) {
	return Null(), fmt.Errorf("no such member: %s", name)
}

func (o *objMap) Index(key Value) (Value, error) {
	k, err := toKey(Pos{}, key)
	if err != nil {
		return Null(), err
	}

	v, ok := o.m[k]
	if !ok {
		return Null(), nil
	}
	return v, nil
}

type nsSpec struct {
	name string
	top  bool
	fns  map[string]NativeFunc
}

func stdlibCoreSpec() map[string]NativeFunc {
	return map[string]NativeFunc{
		"fail":     builtinFail,
		"len":      builtinLen,
		"contains": builtinContains,
		"match":    builtinMatch,
		"str":      builtinStr,
		"default":  builtinDefault,
		"uuid":     builtinUUID,
	}
}

func stdlibNSpecs() []nsSpec {
	return []nsSpec{
		{name: "base64", top: true, fns: map[string]NativeFunc{
			"encode": builtinB64Enc,
			"decode": builtinB64Dec,
		}},
		{name: "url", top: true, fns: map[string]NativeFunc{
			"encode": builtinURLEnc,
			"decode": builtinURLDec,
		}},
		{name: "time", top: true, fns: map[string]NativeFunc{
			"nowISO": builtinTimeNowISO,
			"format": builtinTimeFormat,
		}},
		{name: "json", top: true, fns: map[string]NativeFunc{
			"file":      builtinJSONFile,
			"parse":     builtinJSONParse,
			"stringify": builtinJSONStringify,
			"get":       builtinJSONGet,
		}},
		{name: "headers", top: true, fns: map[string]NativeFunc{
			"get":       builtinHeadersGet,
			"has":       builtinHeadersHas,
			"set":       builtinHeadersSet,
			"remove":    builtinHeadersRemove,
			"merge":     builtinHeadersMerge,
			"normalize": builtinHeadersNormalize,
		}},
		{name: "query", top: true, fns: map[string]NativeFunc{
			"parse":  builtinQueryParse,
			"encode": builtinQueryEncode,
			"merge":  builtinQueryMerge,
		}},
		{name: "text", fns: map[string]NativeFunc{
			"lower":      builtinTextLower,
			"upper":      builtinTextUpper,
			"trim":       builtinTextTrim,
			"split":      builtinTextSplit,
			"join":       builtinTextJoin,
			"replace":    builtinTextReplace,
			"startsWith": builtinTextStartsWith,
			"endsWith":   builtinTextEndsWith,
		}},
		{name: "list", fns: map[string]NativeFunc{
			"append": builtinListAppend,
			"concat": builtinListConcat,
			"sort":   builtinListSort,
		}},
		{name: "dict", fns: map[string]NativeFunc{
			"keys":   builtinDictKeys,
			"values": builtinDictValues,
			"items":  builtinDictItems,
			"set":    builtinDictSet,
			"merge":  builtinDictMerge,
			"remove": builtinDictRemove,
		}},
		{name: "math", fns: map[string]NativeFunc{
			"abs":   builtinMathAbs,
			"min":   builtinMathMin,
			"max":   builtinMathMax,
			"clamp": builtinMathClamp,
			"floor": builtinMathFloor,
			"ceil":  builtinMathCeil,
			"round": builtinMathRound,
		}},
	}
}

func addVals(dst, src map[string]Value) {
	for k, v := range src {
		dst[k] = v
	}
}

func mkFns(prefix string, fns map[string]NativeFunc) map[string]Value {
	out := make(map[string]Value, len(fns))
	for k, f := range fns {
		name := k
		if prefix != "" {
			name = prefix + "." + k
		}
		out[k] = NativeNamed(name, f)
	}
	return out
}

func mkObj(name string, fns map[string]NativeFunc) *objMap {
	return &objMap{name: name, m: mkFns(name, fns)}
}

func Stdlib() map[string]Value {
	return stdlibWithReq(nil)
}

func stdlibWithReq(req *requestObj) map[string]Value {
	core := mkFns("", stdlibCoreSpec())
	spec := stdlibNSpecs()
	top := 0
	for _, s := range spec {
		if s.top {
			top++
		}
	}
	out := make(map[string]Value, len(core)+top+2)
	addVals(out, core)
	std := &objMap{name: "stdlib", m: make(map[string]Value, len(core)+len(spec))}
	addVals(std.m, core)
	for _, s := range spec {
		o := mkObj(s.name, s.fns)
		if s.top {
			out[s.name] = Obj(o)
		}
		std.m[s.name] = Obj(o)
	}
	out["stdlib"] = Obj(std)
	if req != nil {
		out["request"] = Obj(req)
	}
	return out
}

func NativeNamed(name string, f NativeFunc) Value {
	return Native(func(ctx *Ctx, pos Pos, args []Value) (Value, error) {
		if ctx != nil {
			ctx.push(Frame{Kind: FrameNative, Pos: pos, Name: name})
			defer ctx.pop()
		}
		return f(ctx, pos, args)
	})
}

func builtinFail(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	msg := "fail()"
	if len(args) == 1 {
		s, err := toStr(ctx, pos, args[0])
		if err != nil {
			return Null(), err
		}
		msg = s
	} else if len(args) > 1 {
		msg = fmt.Sprintf("fail(%d args)", len(args))
	}
	return Null(), rtErr(ctx, pos, "%s", msg)
}

func builtinLen(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if len(args) != 1 {
		return Null(), rtErr(ctx, pos, "len(x) expects 1 arg")
	}
	switch args[0].K {
	case VStr:
		return Num(float64(len(args[0].S))), nil
	case VList:
		return Num(float64(len(args[0].L))), nil
	case VDict:
		return Num(float64(len(args[0].M))), nil
	default:
		return Null(), rtErr(ctx, pos, "len(x) unsupported")
	}
}

func builtinContains(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if len(args) != 2 {
		return Null(), rtErr(ctx, pos, "contains(a, b) expects 2 args")
	}

	h := args[0]
	n := args[1]
	s, err := toStr(ctx, pos, n)
	if err != nil {
		return Null(), err
	}

	switch h.K {
	case VStr:
		return Bool(strings.Contains(h.S, s)), nil
	case VList:
		for _, it := range h.L {
			if eq(it, n) {
				return Bool(true), nil
			}
		}
		return Bool(false), nil
	case VDict:
		_, ok := h.M[s]
		return Bool(ok), nil
	default:
		return Null(), rtErr(ctx, pos, "contains unsupported")
	}
}

func builtinMatch(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if len(args) != 2 {
		return Null(), rtErr(ctx, pos, "match(pattern, text) expects 2 args")
	}

	pat, err := toStr(ctx, pos, args[0])
	if err != nil {
		return Null(), err
	}

	txt, err := toStr(ctx, pos, args[1])
	if err != nil {
		return Null(), err
	}

	if ctx != nil && ctx.Lim.MaxStr > 0 && len(pat) > ctx.Lim.MaxStr {
		return Null(), rtErr(ctx, pos, "pattern too long")
	}

	re, err := regexp.Compile(pat)
	if err != nil {
		return Null(), rtErr(ctx, pos, "invalid regex")
	}
	return Bool(re.MatchString(txt)), nil
}

func builtinStr(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if len(args) != 1 {
		return Null(), rtErr(ctx, pos, "str(x) expects 1 arg")
	}

	s, err := toStr(ctx, pos, args[0])
	if err != nil {
		return Null(), err
	}
	return Str(s), nil
}

func builtinDefault(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if len(args) != 2 {
		return Null(), rtErr(ctx, pos, "default(a, b) expects 2 args")
	}
	if args[0].K != VNull {
		return args[0], nil
	}
	return args[1], nil
}

func builtinB64Enc(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if len(args) != 1 {
		return Null(), rtErr(ctx, pos, "base64.encode(x) expects 1 arg")
	}

	s, err := toStr(ctx, pos, args[0])
	if err != nil {
		return Null(), err
	}
	return Str(base64.StdEncoding.EncodeToString([]byte(s))), nil
}

func builtinB64Dec(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if len(args) != 1 {
		return Null(), rtErr(ctx, pos, "base64.decode(x) expects 1 arg")
	}

	s, err := toStr(ctx, pos, args[0])
	if err != nil {
		return Null(), err
	}

	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return Null(), rtErr(ctx, pos, "base64 decode failed")
	}
	return Str(string(b)), nil
}

func builtinURLEnc(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if len(args) != 1 {
		return Null(), rtErr(ctx, pos, "url.encode(x) expects 1 arg")
	}

	s, err := toStr(ctx, pos, args[0])
	if err != nil {
		return Null(), err
	}
	return Str(url.QueryEscape(s)), nil
}

func builtinURLDec(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if len(args) != 1 {
		return Null(), rtErr(ctx, pos, "url.decode(x) expects 1 arg")
	}

	s, err := toStr(ctx, pos, args[0])
	if err != nil {
		return Null(), err
	}

	res, err := url.QueryUnescape(s)
	if err != nil {
		return Null(), rtErr(ctx, pos, "url decode failed")
	}
	return Str(res), nil
}

func builtinTimeNowISO(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if len(args) != 0 {
		return Null(), rtErr(ctx, pos, "time.nowISO() expects 0 args")
	}
	if ctx == nil || ctx.Now == nil {
		return Null(), rtErr(ctx, pos, "time not available")
	}
	return Str(ctx.Now().UTC().Format(time.RFC3339)), nil
}

func builtinTimeFormat(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if len(args) != 1 {
		return Null(), rtErr(ctx, pos, "time.format(layout) expects 1 arg")
	}

	if ctx == nil || ctx.Now == nil {
		return Null(), rtErr(ctx, pos, "time not available")
	}

	layout, err := toStr(ctx, pos, args[0])
	if err != nil {
		return Null(), err
	}
	return Str(ctx.Now().Format(layout)), nil
}

func builtinUUID(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if len(args) != 0 {
		return Null(), rtErr(ctx, pos, "uuid() expects 0 args")
	}

	if ctx != nil && ctx.UUID != nil {
		id, err := ctx.UUID()
		if err != nil {
			return Null(), rtErr(ctx, pos, "uuid failed")
		}
		return Str(id), nil
	}

	if ctx != nil && !ctx.AllowRandom {
		return Null(), rtErr(ctx, pos, "uuid not allowed")
	}

	id, err := randUUID()
	if err != nil {
		return Null(), rtErr(ctx, pos, "uuid failed")
	}
	return Str(id), nil
}

func randUUID() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

func builtinJSONFile(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if len(args) != 1 {
		return Null(), rtErr(ctx, pos, "json.file(path) expects 1 arg")
	}

	if ctx == nil || ctx.ReadFile == nil {
		return Null(), rtErr(ctx, pos, "file access not available")
	}

	p, err := toStr(ctx, pos, args[0])
	if err != nil {
		return Null(), err
	}

	path := p
	if !filepath.IsAbs(path) && ctx.BaseDir != "" {
		path = filepath.Join(ctx.BaseDir, path)
	}

	data, err := ctx.ReadFile(path)
	if err != nil {
		return Null(), rtErr(ctx, pos, "file read failed")
	}

	if ctx.Lim.MaxStr > 0 && len(data) > ctx.Lim.MaxStr {
		return Null(), rtErr(ctx, pos, "file too large")
	}

	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return Null(), rtErr(ctx, pos, "invalid json")
	}

	return fromIface(ctx, pos, raw)
}

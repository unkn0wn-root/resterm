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

func Builtins() map[string]Value {
	return builtinsWithReq(nil)
}

func builtinsWithReq(req *requestObj) map[string]Value {
	b64 := &objMap{name: "base64", m: map[string]Value{}}
	b64.m["encode"] = NativeNamed("base64.encode", builtinB64Enc)
	b64.m["decode"] = NativeNamed("base64.decode", builtinB64Dec)

	u := &objMap{name: "url", m: map[string]Value{}}
	u.m["encode"] = NativeNamed("url.encode", builtinURLEnc)
	u.m["decode"] = NativeNamed("url.decode", builtinURLDec)

	tm := &objMap{name: "time", m: map[string]Value{}}
	tm.m["nowISO"] = NativeNamed("time.nowISO", builtinTimeNowISO)
	tm.m["format"] = NativeNamed("time.format", builtinTimeFormat)

	js := &objMap{name: "json", m: map[string]Value{}}
	js.m["file"] = NativeNamed("json.file", builtinJSONFile)
	js.m["parse"] = NativeNamed("json.parse", builtinJSONParse)
	js.m["stringify"] = NativeNamed("json.stringify", builtinJSONStringify)
	js.m["get"] = NativeNamed("json.get", builtinJSONGet)

	h := &objMap{name: "headers", m: map[string]Value{}}
	h.m["get"] = NativeNamed("headers.get", builtinHeadersGet)
	h.m["has"] = NativeNamed("headers.has", builtinHeadersHas)
	h.m["set"] = NativeNamed("headers.set", builtinHeadersSet)
	h.m["remove"] = NativeNamed("headers.remove", builtinHeadersRemove)
	h.m["merge"] = NativeNamed("headers.merge", builtinHeadersMerge)
	h.m["normalize"] = NativeNamed("headers.normalize", builtinHeadersNormalize)

	q := &objMap{name: "query", m: map[string]Value{}}
	q.m["parse"] = NativeNamed("query.parse", builtinQueryParse)
	q.m["encode"] = NativeNamed("query.encode", builtinQueryEncode)
	q.m["merge"] = NativeNamed("query.merge", builtinQueryMerge)

	txt := &objMap{name: "text", m: map[string]Value{}}
	txt.m["lower"] = NativeNamed("text.lower", builtinTextLower)
	txt.m["upper"] = NativeNamed("text.upper", builtinTextUpper)
	txt.m["trim"] = NativeNamed("text.trim", builtinTextTrim)
	txt.m["split"] = NativeNamed("text.split", builtinTextSplit)
	txt.m["join"] = NativeNamed("text.join", builtinTextJoin)
	txt.m["replace"] = NativeNamed("text.replace", builtinTextReplace)
	txt.m["startsWith"] = NativeNamed("text.startsWith", builtinTextStartsWith)
	txt.m["endsWith"] = NativeNamed("text.endsWith", builtinTextEndsWith)

	lst := &objMap{name: "list", m: map[string]Value{}}
	lst.m["append"] = NativeNamed("list.append", builtinListAppend)
	lst.m["concat"] = NativeNamed("list.concat", builtinListConcat)
	lst.m["sort"] = NativeNamed("list.sort", builtinListSort)

	dic := &objMap{name: "dict", m: map[string]Value{}}
	dic.m["keys"] = NativeNamed("dict.keys", builtinDictKeys)
	dic.m["values"] = NativeNamed("dict.values", builtinDictValues)
	dic.m["items"] = NativeNamed("dict.items", builtinDictItems)
	dic.m["set"] = NativeNamed("dict.set", builtinDictSet)
	dic.m["merge"] = NativeNamed("dict.merge", builtinDictMerge)
	dic.m["remove"] = NativeNamed("dict.remove", builtinDictRemove)

	mt := &objMap{name: "math", m: map[string]Value{}}
	mt.m["abs"] = NativeNamed("math.abs", builtinMathAbs)
	mt.m["min"] = NativeNamed("math.min", builtinMathMin)
	mt.m["max"] = NativeNamed("math.max", builtinMathMax)
	mt.m["clamp"] = NativeNamed("math.clamp", builtinMathClamp)
	mt.m["floor"] = NativeNamed("math.floor", builtinMathFloor)
	mt.m["ceil"] = NativeNamed("math.ceil", builtinMathCeil)
	mt.m["round"] = NativeNamed("math.round", builtinMathRound)

	out := map[string]Value{
		"fail":     NativeNamed("fail", builtinFail),
		"len":      NativeNamed("len", builtinLen),
		"contains": NativeNamed("contains", builtinContains),
		"match":    NativeNamed("match", builtinMatch),
		"str":      NativeNamed("str", builtinStr),
		"default":  NativeNamed("default", builtinDefault),
		"uuid":     NativeNamed("uuid", builtinUUID),
		"base64":   Obj(b64),
		"url":      Obj(u),
		"time":     Obj(tm),
		"json":     Obj(js),
		"headers":  Obj(h),
		"query":    Obj(q),
	}
	std := &objMap{name: "stdlib", m: map[string]Value{}}
	std.m["fail"] = out["fail"]
	std.m["len"] = out["len"]
	std.m["contains"] = out["contains"]
	std.m["match"] = out["match"]
	std.m["str"] = out["str"]
	std.m["default"] = out["default"]
	std.m["uuid"] = out["uuid"]
	std.m["base64"] = Obj(b64)
	std.m["url"] = Obj(u)
	std.m["time"] = Obj(tm)
	std.m["json"] = Obj(js)
	std.m["headers"] = Obj(h)
	std.m["query"] = Obj(q)
	std.m["text"] = Obj(txt)
	std.m["list"] = Obj(lst)
	std.m["dict"] = Obj(dic)
	std.m["math"] = Obj(mt)
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

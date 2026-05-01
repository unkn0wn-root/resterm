package rts

import "net/url"

const (
	sigURLEncode = "url.encode(x)"
	sigURLDecode = "url.decode(x)"
)

var urlSpec = nsSpec{name: "url", top: true, fns: map[string]NativeFunc{
	"encode": urlEncode,
	"decode": urlDecode,
}}

func urlEncode(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigURLEncode)
	if err := na.count(1); err != nil {
		return Null(), err
	}

	s, err := na.toStr(0)
	if err != nil {
		return Null(), err
	}
	return Str(url.QueryEscape(s)), nil
}

func urlDecode(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigURLDecode)
	if err := na.count(1); err != nil {
		return Null(), err
	}

	s, err := na.toStr(0)
	if err != nil {
		return Null(), err
	}

	res, err := url.QueryUnescape(s)
	if err != nil {
		return Null(), rtErr(ctx, pos, "url decode failed")
	}
	return Str(res), nil
}

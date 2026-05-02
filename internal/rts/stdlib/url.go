package stdlib

import (
	"net/url"

	"github.com/unkn0wn-root/resterm/internal/rts"
)

const (
	sigURLEncode = "url.encode(x)"
	sigURLDecode = "url.decode(x)"
)

var urlSpec = nsSpec{name: "url", top: true, fns: map[string]rts.NativeFunc{
	"encode": urlEncode,
	"decode": urlDecode,
}}

func urlEncode(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigURLEncode)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	s, err := na.ToStr(0)
	if err != nil {
		return rts.Null(), err
	}
	return rts.Str(url.QueryEscape(s)), nil
}

func urlDecode(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigURLDecode)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	s, err := na.ToStr(0)
	if err != nil {
		return rts.Null(), err
	}

	res, err := url.QueryUnescape(s)
	if err != nil {
		return rts.Null(), rts.Errf(ctx, pos, "url decode failed")
	}
	return rts.Str(res), nil
}

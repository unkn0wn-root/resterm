package stdlib

import (
	"encoding/base64"

	"github.com/unkn0wn-root/resterm/internal/rts"
)

const (
	sigBase64Encode = "base64.encode(x)"
	sigBase64Decode = "base64.decode(x)"
)

var base64Spec = nsSpec{name: "base64", top: true, fns: map[string]rts.NativeFunc{
	"encode": base64Encode,
	"decode": base64Decode,
}}

func base64Encode(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigBase64Encode)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	s, err := na.ToStr(0)
	if err != nil {
		return rts.Null(), err
	}
	return rts.Str(base64.StdEncoding.EncodeToString([]byte(s))), nil
}

func base64Decode(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigBase64Decode)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	s, err := na.ToStr(0)
	if err != nil {
		return rts.Null(), err
	}

	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return rts.Null(), rts.Errf(ctx, pos, "base64 decode failed")
	}
	return rts.Str(string(b)), nil
}

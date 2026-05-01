package rts

import "encoding/base64"

const (
	sigBase64Encode = "base64.encode(x)"
	sigBase64Decode = "base64.decode(x)"
)

var base64Spec = nsSpec{name: "base64", top: true, fns: map[string]NativeFunc{
	"encode": base64Encode,
	"decode": base64Decode,
}}

func base64Encode(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigBase64Encode)
	if err := na.count(1); err != nil {
		return Null(), err
	}

	s, err := na.toStr(0)
	if err != nil {
		return Null(), err
	}
	return Str(base64.StdEncoding.EncodeToString([]byte(s))), nil
}

func base64Decode(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigBase64Decode)
	if err := na.count(1); err != nil {
		return Null(), err
	}

	s, err := na.toStr(0)
	if err != nil {
		return Null(), err
	}

	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return Null(), rtErr(ctx, pos, "base64 decode failed")
	}
	return Str(string(b)), nil
}

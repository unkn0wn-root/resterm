package rts

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

var cryptoSpec = nsSpec{name: "crypto", top: true, fns: map[string]NativeFunc{
	"sha256":     cryptoSHA256,
	"hmacSha256": cryptoHMACSHA256,
}}

func cryptoSHA256(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "crypto.sha256(text)")
	if err := na.count(1); err != nil {
		return Null(), err
	}

	s, err := na.str(0)
	if err != nil {
		return Null(), err
	}

	sum := sha256.Sum256([]byte(s))
	return hexVal(ctx, pos, sum[:])
}

func cryptoHMACSHA256(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "crypto.hmacSha256(key, text)")
	if err := na.count(2); err != nil {
		return Null(), err
	}

	key, err := na.str(0)
	if err != nil {
		return Null(), err
	}

	msg, err := na.str(1)
	if err != nil {
		return Null(), err
	}

	h := hmac.New(sha256.New, []byte(key))
	_, _ = h.Write([]byte(msg))
	return hexVal(ctx, pos, h.Sum(nil))
}

func hexVal(ctx *Ctx, pos Pos, b []byte) (Value, error) {
	out := hex.EncodeToString(b)
	if err := chkStr(ctx, pos, out); err != nil {
		return Null(), err
	}
	return Str(out), nil
}

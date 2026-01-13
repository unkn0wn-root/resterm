package rts

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

func stdlibSHA256(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	sig := "crypto.sha256(text)"
	if err := argCount(ctx, pos, args, 1, sig); err != nil {
		return Null(), err
	}

	s, err := strArg(ctx, pos, args[0], sig)
	if err != nil {
		return Null(), err
	}

	sum := sha256.Sum256([]byte(s))
	return hexVal(ctx, pos, sum[:])
}

func stdlibHMACSHA256(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	sig := "crypto.hmacSha256(key, text)"
	if err := argCount(ctx, pos, args, 2, sig); err != nil {
		return Null(), err
	}

	key, err := strArg(ctx, pos, args[0], sig)
	if err != nil {
		return Null(), err
	}

	msg, err := strArg(ctx, pos, args[1], sig)
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

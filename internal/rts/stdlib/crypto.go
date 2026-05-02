package stdlib

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"github.com/unkn0wn-root/resterm/internal/rts"
)

const (
	sigCryptoSHA256     = "crypto.sha256(text)"
	sigCryptoHMACSHA256 = "crypto.hmacSha256(key, text)"
)

var cryptoSpec = nsSpec{name: "crypto", top: true, fns: map[string]rts.NativeFunc{
	"sha256":     cryptoSHA256,
	"hmacSha256": cryptoHMACSHA256,
}}

func cryptoSHA256(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigCryptoSHA256)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	s, err := na.Str(0)
	if err != nil {
		return rts.Null(), err
	}

	sum := sha256.Sum256([]byte(s))
	return hexVal(ctx, pos, sum[:])
}

func cryptoHMACSHA256(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigCryptoHMACSHA256)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	key, err := na.Str(0)
	if err != nil {
		return rts.Null(), err
	}

	msg, err := na.Str(1)
	if err != nil {
		return rts.Null(), err
	}

	h := hmac.New(sha256.New, []byte(key))
	_, _ = h.Write([]byte(msg))
	return hexVal(ctx, pos, h.Sum(nil))
}

func hexVal(ctx *rts.Ctx, pos rts.Pos, b []byte) (rts.Value, error) {
	out := hex.EncodeToString(b)
	if err := rts.CheckStr(ctx, pos, out); err != nil {
		return rts.Null(), err
	}
	return rts.Str(out), nil
}

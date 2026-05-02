package stdlib

import (
	"encoding/base64"
	"encoding/hex"

	"github.com/unkn0wn-root/resterm/internal/rts"
)

const (
	sigEncodingHexEncode       = "encoding.hex.encode(text)"
	sigEncodingHexDecode       = "encoding.hex.decode(text)"
	sigEncodingBase64URLEncode = "encoding.base64url.encode(text)"
	sigEncodingBase64URLDecode = "encoding.base64url.decode(text)"
)

func mkEncObj() *objMap {
	hx := mkObj("encoding.hex", map[string]rts.NativeFunc{
		"encode": encodingHexEncode,
		"decode": encodingHexDecode,
	})

	bu := mkObj("encoding.base64url", map[string]rts.NativeFunc{
		"encode": encodingBase64urlEncode,
		"decode": encodingBase64urlDecode,
	})

	return &objMap{
		name: "encoding",
		m: map[string]rts.Value{
			"hex":       rts.Obj(hx),
			"base64url": rts.Obj(bu),
		},
	}
}

func encodingHexEncode(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigEncodingHexEncode)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	s, err := na.Str(0)
	if err != nil {
		return rts.Null(), err
	}
	return hexVal(ctx, pos, []byte(s))
}

func encodingHexDecode(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigEncodingHexDecode)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	s, err := na.Str(0)
	if err != nil {
		return rts.Null(), err
	}

	out, err := hex.DecodeString(s)
	if err != nil {
		return rts.Null(), rts.Errf(ctx, pos, "hex decode failed")
	}

	res := string(out)
	if err := rts.CheckStr(ctx, pos, res); err != nil {
		return rts.Null(), err
	}
	return rts.Str(res), nil
}

func encodingBase64urlEncode(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigEncodingBase64URLEncode)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	s, err := na.Str(0)
	if err != nil {
		return rts.Null(), err
	}

	out := base64.RawURLEncoding.EncodeToString([]byte(s))
	if err := rts.CheckStr(ctx, pos, out); err != nil {
		return rts.Null(), err
	}
	return rts.Str(out), nil
}

func encodingBase64urlDecode(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigEncodingBase64URLDecode)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	s, err := na.Str(0)
	if err != nil {
		return rts.Null(), err
	}

	out, err := b64URLDec(s)
	if err != nil {
		return rts.Null(), rts.Errf(ctx, pos, "base64url decode failed")
	}

	res := string(out)
	if err := rts.CheckStr(ctx, pos, res); err != nil {
		return rts.Null(), err
	}
	return rts.Str(res), nil
}

func b64URLDec(s string) ([]byte, error) {
	out, err := base64.RawURLEncoding.DecodeString(s)
	if err == nil {
		return out, nil
	}
	return base64.URLEncoding.DecodeString(s)
}

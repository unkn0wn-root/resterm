package rts

import (
	"encoding/base64"
	"encoding/hex"
)

const (
	sigEncodingHexEncode       = "encoding.hex.encode(text)"
	sigEncodingHexDecode       = "encoding.hex.decode(text)"
	sigEncodingBase64URLEncode = "encoding.base64url.encode(text)"
	sigEncodingBase64URLDecode = "encoding.base64url.decode(text)"
)

func mkEncObj() *objMap {
	hx := mkObj("encoding.hex", map[string]NativeFunc{
		"encode": encodingHexEncode,
		"decode": encodingHexDecode,
	})

	bu := mkObj("encoding.base64url", map[string]NativeFunc{
		"encode": encodingBase64urlEncode,
		"decode": encodingBase64urlDecode,
	})

	return &objMap{
		name: "encoding",
		m: map[string]Value{
			"hex":       Obj(hx),
			"base64url": Obj(bu),
		},
	}
}

func encodingHexEncode(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigEncodingHexEncode)
	if err := na.count(1); err != nil {
		return Null(), err
	}

	s, err := na.str(0)
	if err != nil {
		return Null(), err
	}
	return hexVal(ctx, pos, []byte(s))
}

func encodingHexDecode(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigEncodingHexDecode)
	if err := na.count(1); err != nil {
		return Null(), err
	}

	s, err := na.str(0)
	if err != nil {
		return Null(), err
	}

	out, err := hex.DecodeString(s)
	if err != nil {
		return Null(), rtErr(ctx, pos, "hex decode failed")
	}

	res := string(out)
	if err := chkStr(ctx, pos, res); err != nil {
		return Null(), err
	}
	return Str(res), nil
}

func encodingBase64urlEncode(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigEncodingBase64URLEncode)
	if err := na.count(1); err != nil {
		return Null(), err
	}

	s, err := na.str(0)
	if err != nil {
		return Null(), err
	}

	out := base64.RawURLEncoding.EncodeToString([]byte(s))
	if err := chkStr(ctx, pos, out); err != nil {
		return Null(), err
	}
	return Str(out), nil
}

func encodingBase64urlDecode(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigEncodingBase64URLDecode)
	if err := na.count(1); err != nil {
		return Null(), err
	}

	s, err := na.str(0)
	if err != nil {
		return Null(), err
	}

	out, err := b64URLDec(s)
	if err != nil {
		return Null(), rtErr(ctx, pos, "base64url decode failed")
	}

	res := string(out)
	if err := chkStr(ctx, pos, res); err != nil {
		return Null(), err
	}
	return Str(res), nil
}

func b64URLDec(s string) ([]byte, error) {
	out, err := base64.RawURLEncoding.DecodeString(s)
	if err == nil {
		return out, nil
	}
	return base64.URLEncoding.DecodeString(s)
}

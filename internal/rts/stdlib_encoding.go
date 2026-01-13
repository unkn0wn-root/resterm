package rts

import (
	"encoding/base64"
	"encoding/hex"
)

func mkEncObj() *objMap {
	hx := mkObj("encoding.hex", map[string]NativeFunc{
		"encode": stdlibHexEnc,
		"decode": stdlibHexDec,
	})

	bu := mkObj("encoding.base64url", map[string]NativeFunc{
		"encode": stdlibB64URLEnc,
		"decode": stdlibB64URLDec,
	})

	return &objMap{
		name: "encoding",
		m: map[string]Value{
			"hex":       Obj(hx),
			"base64url": Obj(bu),
		},
	}
}

func stdlibHexEnc(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	sig := "encoding.hex.encode(text)"
	if err := argCount(ctx, pos, args, 1, sig); err != nil {
		return Null(), err
	}

	s, err := strArg(ctx, pos, args[0], sig)
	if err != nil {
		return Null(), err
	}
	return hexVal(ctx, pos, []byte(s))
}

func stdlibHexDec(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	sig := "encoding.hex.decode(text)"
	if err := argCount(ctx, pos, args, 1, sig); err != nil {
		return Null(), err
	}

	s, err := strArg(ctx, pos, args[0], sig)
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

func stdlibB64URLEnc(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	sig := "encoding.base64url.encode(text)"
	if err := argCount(ctx, pos, args, 1, sig); err != nil {
		return Null(), err
	}

	s, err := strArg(ctx, pos, args[0], sig)
	if err != nil {
		return Null(), err
	}

	out := base64.RawURLEncoding.EncodeToString([]byte(s))
	if err := chkStr(ctx, pos, out); err != nil {
		return Null(), err
	}
	return Str(out), nil
}

func stdlibB64URLDec(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	sig := "encoding.base64url.decode(text)"
	if err := argCount(ctx, pos, args, 1, sig); err != nil {
		return Null(), err
	}

	s, err := strArg(ctx, pos, args[0], sig)
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

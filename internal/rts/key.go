package rts

import "strings"

// Key validates that v is a string key and returns its raw value.
func Key(pos Pos, v Value) (string, error) {
	if v.K != VStr {
		return "", Errf(nil, pos, "expected string key")
	}
	return v.S, nil
}

// KeyArg validates and trims a user-supplied string key argument.
func KeyArg(ctx *Ctx, pos Pos, v Value, sig string) (string, error) {
	k, err := Key(pos, v)
	if err != nil {
		return "", WrapErr(ctx, err)
	}

	k = strings.TrimSpace(k)
	if k == "" {
		return "", Errf(ctx, pos, "%s expects non-empty key", sig)
	}
	return k, nil
}

// MapKey validates and trims an existing dictionary/map key.
func MapKey(ctx *Ctx, pos Pos, key, sig string) (string, error) {
	k := strings.TrimSpace(key)
	if k == "" {
		return "", Errf(ctx, pos, "%s expects non-empty key", sig)
	}
	return k, nil
}

func lookupKey(key string) string {
	return strings.ToLower(key)
}

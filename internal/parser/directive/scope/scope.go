package scope

import "strings"

func Parse[T ~int](token string, request, file, global T) (T, bool) {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "global":
		return global, true
	case "file":
		return file, true
	case "request":
		return request, true
	default:
		var zero T
		return zero, false
	}
}

func ParseToken(token string) (string, bool) {
	tok := strings.ToLower(strings.TrimSpace(token))
	if tok == "" {
		return "", false
	}
	secret := strings.HasSuffix(tok, "-secret")
	if secret {
		tok = strings.TrimSuffix(tok, "-secret")
	}
	return tok, secret
}

func Label[T comparable](scope, request, file, global T) string {
	switch scope {
	case global:
		return "global"
	case file:
		return "file"
	case request:
		return "request"
	default:
		return "request"
	}
}

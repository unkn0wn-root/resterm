package restfile

import "strings"

func CloneAuthSpec(auth *AuthSpec) *AuthSpec {
	if auth == nil {
		return nil
	}
	cp := *auth
	cp.Params = cloneAuthParams(auth.Params)
	return &cp
}

func CloneAuthSpecValue(auth AuthSpec) AuthSpec {
	cp := auth
	cp.Params = cloneAuthParams(auth.Params)
	return cp
}

func AuthScopeLabel(scope AuthScope) string {
	switch scope {
	case AuthScopeRequest:
		return "request"
	case AuthScopeFile:
		return "file"
	case AuthScopeGlobal:
		return "global"
	default:
		return strings.ToLower(strings.TrimSpace(scope.String()))
	}
}

func (s AuthScope) String() string {
	switch s {
	case AuthScopeRequest:
		return "request"
	case AuthScopeFile:
		return "file"
	case AuthScopeGlobal:
		return "global"
	default:
		return "unknown"
	}
}

func cloneAuthParams(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

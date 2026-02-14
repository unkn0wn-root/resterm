package parser

import "strings"

func parseDirectiveScope[T ~int](token string, request, file, global T) (T, bool) {
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

func directiveScopeLabel[T comparable](scope, request, file, global T) string {
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

package target

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

type Kind string

const (
	DefaultNamespace      = "default"
	Pod              Kind = "pod"
	Service          Kind = "service"
	Deployment       Kind = "deployment"
	StatefulSet      Kind = "statefulset"
)

func ParseRef(ref string) (Kind, string, error) {
	if ref == "" {
		return "", "", errors.New("target is required")
	}

	kindRef, name, ok := splitRef(ref)
	if !ok {
		return Pod, ref, nil
	}

	kind := ParseKind(kindRef)
	if kind == "" {
		return "", "", fmt.Errorf("invalid target kind %q", kindRef)
	}
	if name == "" {
		return "", "", errors.New("target name is required")
	}
	return kind, name, nil
}

func ParseKind(raw string) Kind {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(Pod):
		return Pod
	case "svc", string(Service):
		return Service
	case "deploy", string(Deployment):
		return Deployment
	case "sts", string(StatefulSet):
		return StatefulSet
	default:
		return ""
	}
}

func Format(kind Kind, name string) string {
	return strings.TrimSpace(string(kind)) + ":" + strings.TrimSpace(name)
}

func IsValidPortName(raw string) bool {
	val := strings.TrimSpace(raw)
	if val == "" || strings.ContainsAny(val, " \t\r\n") {
		return false
	}
	if strings.Contains(val, "{{") || strings.Contains(val, "}}") {
		return hasBalancedTemplateDelims(val)
	}

	hasAlnum := false
	for _, r := range val {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			hasAlnum = true
			continue
		}
		switch r {
		case '-', '_', '.':
			continue
		default:
			return false
		}
	}
	return hasAlnum
}

func hasBalancedTemplateDelims(val string) bool {
	depth := 0
	for i := 0; i < len(val); {
		switch {
		case strings.HasPrefix(val[i:], "{{"):
			depth++
			i += 2
		case strings.HasPrefix(val[i:], "}}"):
			if depth == 0 {
				return false
			}
			depth--
			i += 2
		default:
			i++
		}
	}
	return depth == 0
}

func splitRef(ref string) (kind, name string, ok bool) {
	for _, sep := range [...]string{":", "/"} {
		lhs, rhs, found := strings.Cut(ref, sep)
		if found {
			return strings.TrimSpace(lhs), strings.TrimSpace(rhs), true
		}
	}
	return "", "", false
}

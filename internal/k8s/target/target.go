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

func ParseRef(raw string) (Kind, string, error) {
	val := strings.TrimSpace(raw)
	if val == "" {
		return "", "", errors.New("k8s: target is required")
	}

	var lhs, rhs string
	if i := strings.Index(val, ":"); i >= 0 {
		lhs = strings.TrimSpace(val[:i])
		rhs = strings.TrimSpace(val[i+1:])
	} else if i := strings.Index(val, "/"); i >= 0 {
		lhs = strings.TrimSpace(val[:i])
		rhs = strings.TrimSpace(val[i+1:])
	} else {
		return Pod, val, nil
	}

	k := ParseKind(lhs)
	if k == "" {
		return "", "", fmt.Errorf("k8s: invalid target kind %q", lhs)
	}
	if rhs == "" {
		return "", "", errors.New("k8s: target name is required")
	}
	return k, rhs, nil
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

// IsValidPortName validates non-numeric port references used in @k8s options.
// It allows template placeholders and otherwise accepts only simple identifier
// characters to reject clearly malformed values early (for example "!!!").
func IsValidPortName(raw string) bool {
	val := strings.TrimSpace(raw)
	if val == "" || strings.ContainsAny(val, " \t\r\n") {
		return false
	}
	if strings.Contains(val, "{{") || strings.Contains(val, "}}") {
		return true
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

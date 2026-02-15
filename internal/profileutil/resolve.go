package profileutil

import (
	"fmt"
	"os"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func SetIf(dst *string, val string) {
	if strings.TrimSpace(val) == "" {
		return
	}
	*dst = val
}

func OptSet[T any](opt restfile.Opt[T], raw string) bool {
	return opt.Set || strings.TrimSpace(raw) != ""
}

func ExpandValue(raw string, resolver *vars.Resolver) (string, error) {
	r := strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(r), "env:") {
		key := strings.TrimSpace(r[4:])
		if key == "" {
			return "", fmt.Errorf("empty env: token")
		}
		if v, ok := os.LookupEnv(key); ok {
			return v, nil
		}
		if v, ok := os.LookupEnv(strings.ToUpper(key)); ok {
			return v, nil
		}
		if resolver != nil {
			if v, ok := resolver.Resolve(key); ok {
				return v, nil
			}
			if v, ok := resolver.Resolve(strings.ToUpper(key)); ok {
				return v, nil
			}
		}
		return "", fmt.Errorf("undefined env variable: %s", key)
	}
	if resolver == nil {
		return r, nil
	}
	return resolver.ExpandTemplates(r)
}

package settings

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/vars"
)

func FromEnv(set vars.EnvironmentSet, envName string) map[string]string {
	values := vars.EnvValues(set, envName)
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string)
	for key, val := range values {
		if key == "" {
			continue
		}
		lower := strings.ToLower(strings.TrimSpace(key))
		const prefix = "settings."
		if strings.HasPrefix(lower, prefix) {
			name := strings.TrimSpace(key[len(prefix):])
			if name != "" {
				out[name] = val
			}
		}
	}
	return out
}

func Merge(scopes ...map[string]string) map[string]string {
	out := make(map[string]string)
	for _, scope := range scopes {
		for k, v := range scope {
			out[k] = v
		}
	}
	return out
}

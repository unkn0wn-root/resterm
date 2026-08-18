package collection

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/vars"
)

func buildEnvTemplate(rootAbs, rootReal string) ([]byte, error) {
	if data, ok, err := readWorkspaceFileIfExists(
		rootAbs,
		rootReal,
		defaultEnvTemplateFile,
	); err != nil {
		return nil, err
	} else if ok {
		return data, nil
	}

	srcs := []string{defaultEnvSourceFile, altEnvSourceFile, "http-client.env.json"}
	for _, src := range srcs {
		raw, ok, err := readWorkspaceFileIfExists(rootAbs, rootReal, src)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		return redactEnv(raw, src)
	}

	return []byte("{}\n"), nil
}

func redactEnv(raw []byte, src string) ([]byte, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("parse env file %s: %w", src, err)
	}
	mask := redactEnvironment(v)
	data, err := json.MarshalIndent(mask, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode env template from %s: %w", src, err)
	}
	return ensureTrailingNewline(data), nil
}

func redactEnvironment(v any) any {
	root, ok := v.(map[string]any)
	if !ok || !hasEnvKey(root, vars.GroupsEnvKey) {
		return redactAny(v)
	}
	out := make(map[string]any, len(root))
	for key, value := range root {
		if envKey(key) == vars.GroupsEnvKey {
			out[key] = redactGroups(value)
			continue
		}
		out[key] = redactAny(value)
	}
	return out
}

func redactGroups(v any) any {
	groups, ok := v.(map[string]any)
	if !ok {
		return redactAny(v)
	}
	out := make(map[string]any, len(groups))
	for name, value := range groups {
		group, ok := value.(map[string]any)
		if !ok {
			out[name] = redactAny(value)
			continue
		}
		profiles := make(map[string]any, len(group))
		for key, profile := range group {
			if envKey(key) == vars.DefaultEnvKey {
				if _, ok := profile.(string); ok {
					profiles[key] = profile
				} else {
					profiles[key] = redactAny(profile)
				}
			} else {
				profiles[key] = redactAny(profile)
			}
		}
		out[name] = profiles
	}
	return out
}

func hasEnvKey(m map[string]any, want string) bool {
	for key := range m {
		if envKey(key) == want {
			return true
		}
	}
	return false
}

func envKey(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func redactAny(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, child := range t {
			out[k] = redactAny(child)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i := range t {
			out[i] = redactAny(t[i])
		}
		return out
	default:
		return envPlaceholder
	}
}

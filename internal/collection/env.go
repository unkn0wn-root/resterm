package collection

import (
	"encoding/json"
	"fmt"
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

	srcs := []string{defaultEnvSourceFile, altEnvSourceFile}
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
	mask := redactAny(v)
	data, err := json.MarshalIndent(mask, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode env template from %s: %w", src, err)
	}
	return ensureTrailingNewline(data), nil
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

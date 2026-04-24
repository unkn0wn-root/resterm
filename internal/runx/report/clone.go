package runfmt

import (
	"maps"
	"time"
)

func CloneDurationMap(src map[string]time.Duration) map[string]time.Duration {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]time.Duration, len(src))
	maps.Copy(out, src)
	return out
}

func CloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, val := range src {
		out[key] = cloneAny(val)
	}
	return out
}

func cloneAny(v any) any {
	switch x := v.(type) {
	case map[string]any:
		return CloneAnyMap(x)
	case []any:
		out := make([]any, 0, len(x))
		for _, item := range x {
			out = append(out, cloneAny(item))
		}
		return out
	default:
		return x
	}
}

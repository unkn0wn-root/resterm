package settings

import (
	"maps"
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/str"
)

const envSettingPrefix = "settings."

func FromValues(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string)
	for _, key := range slices.Sorted(maps.Keys(values)) {
		name, ok := strings.CutPrefix(str.LowerTrim(key), envSettingPrefix)
		if ok && name != "" {
			out[name] = values[key]
		}
	}
	return out
}

// Merge folds scopes together in precedence order and returns canonical keys, so
// later scopes override earlier ones regardless of how each one wrote the key.
// A key also replaces the other names of its option from earlier scopes, so
// two names of one option are left only when one scope wrote both, which
// ApplyAll rejects. Keys are visited in sorted order to keep two forms of the
// same key in one scope from resolving differently between runs.
func Merge(scopes ...map[string]string) map[string]string {
	out := make(map[string]string)
	for _, scope := range scopes {
		// Runs before this scope's own keys, so a pair written in one scope stays.
		for key := range scope {
			if other, ok := sameOption[str.LowerTrim(key)]; ok {
				delete(out, other)
			}
		}
		for _, key := range slices.Sorted(maps.Keys(scope)) {
			if name := str.LowerTrim(key); name != "" {
				out[name] = scope[key]
			}
		}
	}
	return out
}

// Patch returns base with patch layered over it as one more Merge scope. A nil
// value removes its key and the other name of the same option from base. base
// itself is not modified.
func Patch(base map[string]string, patch map[string]*string) map[string]string {
	out := Merge(base)
	set := make(map[string]string, len(patch))
	for k, v := range patch {
		if v != nil {
			set[k] = *v
			continue
		}
		key := str.LowerTrim(k)
		delete(out, key)
		if other, ok := sameOption[key]; ok {
			delete(out, other)
		}
	}
	return Merge(out, set)
}

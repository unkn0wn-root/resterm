package settings

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/util"
)

const envSettingPrefix = "settings."

func FromValues(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string)
	for _, key := range util.SortedKeys(values) {
		name, ok := strings.CutPrefix(util.LowerTrim(key), envSettingPrefix)
		if ok && name != "" {
			out[name] = values[key]
		}
	}
	return out
}

// Merge folds scopes together in precedence order and returns canonical keys, so
// later scopes override earlier ones regardless of how each one wrote the key.
// Keys are visited in sorted order to keep two forms of the same key in one
// scope from resolving differently between runs.
func Merge(scopes ...map[string]string) map[string]string {
	out := make(map[string]string)
	for _, scope := range scopes {
		for _, key := range util.SortedKeys(scope) {
			if name := util.LowerTrim(key); name != "" {
				out[name] = scope[key]
			}
		}
	}
	return out
}

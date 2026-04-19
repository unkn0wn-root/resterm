package vars

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

// SharedEnvKey is the reserved environment name whose variables are merged
// as defaults into every other environment. Environment-specific values win.
const SharedEnvKey = "$shared"

type EnvironmentSet map[string]map[string]string

var environmentFileCandidates = [...]string{
	"rest-client.env.json",
	"resterm.env.json",
}

// IsReservedEnvironment reports whether the name is reserved for
// framework behavior and cannot be selected as a concrete environment.
func IsReservedEnvironment(name string) bool {
	return strings.EqualFold(str.Trim(name), SharedEnvKey)
}

func LoadEnvironmentFile(path string) (EnvironmentSet, error) {
	if IsDotEnvPath(path) {
		return loadDotEnvEnvironment(path)
	}
	return loadJSONEnvironmentFile(path)
}

func loadJSONEnvironmentFile(path string) (EnvironmentSet, error) {
	raw, err := readJSONEnvironment(path)
	if err != nil {
		return nil, err
	}

	envs, err := parseEnvironmentSet(raw)
	if err != nil {
		return nil, err
	}

	hadShared := applyShared(envs)
	if hadShared && len(envs) == 0 {
		return nil, errdef.New(
			errdef.CodeParse,
			`env file %s defines only %q; add at least one concrete environment`,
			path,
			SharedEnvKey,
		)
	}
	return envs, nil
}

func readJSONEnvironment(path string) (raw any, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errdef.Wrap(errdef.CodeFilesystem, err, "open env file %s", path)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = errdef.Wrap(errdef.CodeFilesystem, closeErr, "close env file %s", path)
		}
	}()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, errdef.Wrap(errdef.CodeFilesystem, err, "read env file %s", path)
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, errdef.Wrap(errdef.CodeParse, err, "parse env file %s", path)
	}
	return raw, nil
}

func parseEnvironmentSet(raw any) (EnvironmentSet, error) {
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, errdef.New(errdef.CodeParse, "unsupported env file format: %T", raw)
	}

	envs := make(EnvironmentSet, len(obj))
	for envName, value := range obj {
		envs[envName] = flattenEnv(value)
	}
	return envs, nil
}

// applyShared merges the $shared environment's values as defaults into every
// other environment (environment-specific values take precedence), then removes
// $shared from the set so it never appears as a selectable environment. It
// reports whether the set contained $shared.
func applyShared(envs EnvironmentSet) bool {
	shared, ok := envs[SharedEnvKey]
	if !ok {
		return false
	}
	for name, env := range envs {
		if name == SharedEnvKey {
			continue
		}
		for k, v := range shared {
			if _, exists := env[k]; !exists {
				env[k] = v
			}
		}
	}
	delete(envs, SharedEnvKey)
	return true
}

func flattenEnv(value any) map[string]string {
	result := make(map[string]string)
	flattenEnvValue("", value, result)
	return result
}

// Recursively walks through JSON structure to build dot-notation paths.
// Nested objects become "parent.child" and arrays become "parent[0]".
// Makes deeply nested config accessible via simple string keys.
func flattenEnvValue(prefix string, value any, out map[string]string) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if key == "" {
				continue
			}
			flattenEnvValue(envPath(prefix, key), child, out)
		}
	case []any:
		for idx, item := range v {
			flattenEnvValue(envIndexPath(prefix, idx), item, out)
		}
	default:
		setFlattenedValue(prefix, v, out)
	}
}

func envPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

func envIndexPath(prefix string, idx int) string {
	index := strconv.Itoa(idx)
	if prefix == "" {
		return index
	}
	return prefix + "[" + index + "]"
}

func setFlattenedValue(key string, value any, out map[string]string) {
	if key == "" {
		return
	}

	switch v := value.(type) {
	case string:
		out[key] = v
	case float64:
		out[key] = strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		out[key] = strconv.FormatBool(v)
	case nil:
		out[key] = ""
	}
}

func ResolveEnvironment(paths []string) (EnvironmentSet, string, error) {
	for _, dir := range paths {
		for _, candidate := range environmentFileCandidates {
			p := filepath.Join(dir, candidate)
			if _, err := os.Stat(p); err == nil {
				envs, loadErr := LoadEnvironmentFile(p)
				return envs, p, loadErr
			}
		}
	}
	return nil, "", nil
}

// DefaultEnvironment returns the default concrete environment name.
func DefaultEnvironment(set EnvironmentSet) string {
	if len(set) == 0 {
		return ""
	}
	for _, name := range [...]string{"dev", "default", "local"} {
		if _, ok := set[name]; ok {
			return name
		}
	}
	names := make([]string, 0, len(set))
	for name := range set {
		if str.Trim(name) == "" {
			continue
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return names[0]
}

// SelectEnv returns the effective environment name, preferring the explicit override
// when provided, then falling back to the current selection. Empty strings are ignored.
func SelectEnv(set EnvironmentSet, override, current string) string {
	if trimmed := str.Trim(override); trimmed != "" {
		return trimmed
	}
	if trimmed := str.Trim(current); trimmed != "" {
		return trimmed
	}
	if len(set) == 0 {
		return ""
	}
	for name := range set {
		if str.Trim(name) != "" {
			return name
		}
	}
	return ""
}

// EnvValues returns the flattened key/value map for the requested environment.
func EnvValues(set EnvironmentSet, name string) map[string]string {
	if set == nil {
		return nil
	}
	key := str.Trim(name)
	if key == "" {
		return nil
	}
	if env, ok := set[key]; ok {
		return env
	}
	return nil
}

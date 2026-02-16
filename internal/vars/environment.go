package vars

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/errdef"
)

// SharedEnvKey is the reserved environment name whose variables are merged
// as defaults into every other environment. Environment-specific values win.
const SharedEnvKey = "$shared"

type EnvironmentSet map[string]map[string]string

// IsReservedEnvironment reports whether the name is reserved for
// framework behavior and cannot be selected as a concrete environment.
func IsReservedEnvironment(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), SharedEnvKey)
}

func LoadEnvironmentFile(path string) (EnvironmentSet, error) {
	if IsDotEnvPath(path) {
		return loadDotEnvEnvironment(path)
	}
	return loadJSONEnvironmentFile(path)
}

func loadJSONEnvironmentFile(path string) (envs EnvironmentSet, err error) {
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

	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, errdef.Wrap(errdef.CodeParse, err, "parse env file %s", path)
	}

	envs = make(EnvironmentSet)
	switch v := raw.(type) {
	case map[string]any:
		for envName, value := range v {
			envs[envName] = flattenEnv(value)
		}
	default:
		return nil, errdef.New(errdef.CodeParse, "unsupported env file format: %T", raw)
	}
	hadShared := false
	if _, ok := envs[SharedEnvKey]; ok {
		hadShared = true
	}
	applyShared(envs)
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

// applyShared merges the $shared environment's values as defaults into every
// other environment (environment-specific values take precedence), then removes
// $shared from the set so it never appears as a selectable environment.
func applyShared(envs EnvironmentSet) {
	shared, ok := envs[SharedEnvKey]
	if !ok {
		return
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
			next := key
			if prefix != "" {
				next = prefix + "." + key
			}
			flattenEnvValue(next, child, out)
		}
	case []any:
		for idx, item := range v {
			childKey := strconv.Itoa(idx)
			if prefix != "" {
				childKey = fmt.Sprintf("%s[%d]", prefix, idx)
			}
			flattenEnvValue(childKey, item, out)
		}
	case string:
		if prefix != "" {
			out[prefix] = v
		}
	case float64:
		if prefix != "" {
			out[prefix] = strconv.FormatFloat(v, 'f', -1, 64)
		}
	case bool:
		if prefix != "" {
			out[prefix] = strconv.FormatBool(v)
		}
	case nil:
		if prefix != "" {
			out[prefix] = ""
		}
	default:
		if prefix != "" {
			out[prefix] = fmt.Sprintf("%v", v)
		}
	}
}

func ResolveEnvironment(paths []string) (EnvironmentSet, string, error) {
	candidates := []string{"rest-client.env.json", "resterm.env.json"}
	for _, dir := range paths {
		for _, candidate := range candidates {
			p := filepath.Join(dir, candidate)
			if _, err := os.Stat(p); err == nil {
				envs, loadErr := LoadEnvironmentFile(p)
				return envs, p, loadErr
			}
		}
	}
	return nil, "", nil
}

type EnvironmentProvider struct {
	name    string
	values  map[string]string
	backing string
}

func NewEnvironmentProvider(name string, values map[string]string, backing string) Provider {
	return &EnvironmentProvider{
		name:    name,
		values:  values,
		backing: backing,
	}
}

func (p *EnvironmentProvider) Resolve(name string) (string, bool) {
	value, ok := p.values[name]
	return value, ok
}

func (p *EnvironmentProvider) Label() string {
	if p.backing == "" {
		return fmt.Sprintf("env:%s", p.name)
	}
	return fmt.Sprintf("env:%s (%s)", p.name, filepath.Base(p.backing))
}

// SelectEnv returns the effective environment name, preferring the explicit override
// when provided, then falling back to the current selection. Empty strings are ignored.
func SelectEnv(set EnvironmentSet, override, current string) string {
	if trimmed := strings.TrimSpace(override); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(current); trimmed != "" {
		return trimmed
	}
	if len(set) == 0 {
		return ""
	}
	for name := range set {
		if strings.TrimSpace(name) != "" {
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
	key := strings.TrimSpace(name)
	if key == "" {
		return nil
	}
	if env, ok := set[key]; ok {
		return env
	}
	return nil
}

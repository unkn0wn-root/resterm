package vars

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

type EnvironmentSet map[string]map[string]string

func LoadEnvironmentFile(path string) (EnvironmentSet, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open env file: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read env file: %w", err)
	}

	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse env file: %w", err)
	}

	envs := make(EnvironmentSet)
	switch v := raw.(type) {
	case map[string]interface{}:
		for envName, value := range v {
			envs[envName] = flattenEnv(value)
		}
	default:
		return nil, fmt.Errorf("unsupported env file format: %T", raw)
	}
	return envs, nil
}

func flattenEnv(value interface{}) map[string]string {
	result := make(map[string]string)
	flattenEnvValue("", value, result)
	return result
}

func flattenEnvValue(prefix string, value interface{}, out map[string]string) {
	switch v := value.(type) {
	case map[string]interface{}:
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
	case []interface{}:
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

package telemetry

import (
	"strings"
	"time"
)

const (
	envPrefix      = "RESTERM_TRACE_OTEL_"
	envEndpoint    = envPrefix + "ENDPOINT"
	envInsecure    = envPrefix + "INSECURE"
	envHeaders     = envPrefix + "HEADERS"
	envService     = envPrefix + "SERVICE"
	envDialTimeout = envPrefix + "TIMEOUT"
)

type Config struct {
	Endpoint    string
	Insecure    bool
	Headers     map[string]string
	ServiceName string
	Version     string
	DialTimeout time.Duration
}

func Default() Config {
	return Config{
		ServiceName: "resterm",
		DialTimeout: 5 * time.Second,
	}
}

func (c Config) Enabled() bool {
	return strings.TrimSpace(c.Endpoint) != ""
}

func ConfigFromEnv(getenv func(string) string) Config {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}

	cfg := Default()

	if val := strings.TrimSpace(getenv(envEndpoint)); val != "" {
		cfg.Endpoint = val
	}

	if val := strings.TrimSpace(getenv(envInsecure)); val != "" {
		if parsed, ok := parseBool(val); ok {
			cfg.Insecure = parsed
		}
	}

	if val := strings.TrimSpace(getenv(envService)); val != "" {
		cfg.ServiceName = val
	}

	if val := strings.TrimSpace(getenv(envDialTimeout)); val != "" {
		if dur, err := time.ParseDuration(val); err == nil && dur > 0 {
			cfg.DialTimeout = dur
		}
	}

	if headerSpec := strings.TrimSpace(getenv(envHeaders)); headerSpec != "" {
		if headers, err := ParseHeaders(headerSpec); err == nil {
			cfg.Headers = headers
		}
	}

	return cfg
}

func MergeHeaders(dst map[string]string, src map[string]string) map[string]string {
	if len(src) == 0 {
		if len(dst) == 0 {
			return nil
		}

		cloned := make(map[string]string, len(dst))
		for k, v := range dst {
			cloned[k] = v
		}
		return cloned
	}

	merged := make(map[string]string, len(dst)+len(src))
	for k, v := range dst {
		merged[k] = v
	}

	for k, v := range src {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		merged[key] = strings.TrimSpace(v)
	}
	return merged
}

func ParseHeaders(spec string) (map[string]string, error) {
	if strings.TrimSpace(spec) == "" {
		return nil, nil
	}

	entries := strings.Split(spec, ",")
	headers := make(map[string]string, len(entries))
	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}

		parts := strings.SplitN(trimmed, "=", 2)
		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}

		value := ""
		if len(parts) == 2 {
			value = strings.TrimSpace(parts[1])
		}
		headers[key] = value
	}
	if len(headers) == 0 {
		return nil, nil
	}
	return headers, nil
}

func parseBool(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true, true
	case "0", "false", "no", "n", "off":
		return false, true
	default:
		return false, false
	}
}

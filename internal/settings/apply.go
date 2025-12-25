package settings

import (
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/tlsconfig"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func ApplyGRPCSettings(
	opts *grpcclient.Options,
	settings map[string]string,
	resolver *vars.Resolver,
) error {
	if opts == nil {
		return nil
	}
	tlsCfg := tlsconfig.Files{
		RootCAs:    opts.RootCAs,
		ClientCert: opts.ClientCert,
		ClientKey:  opts.ClientKey,
		Insecure:   opts.Insecure,
		RootMode:   opts.RootMode,
	}
	if err := applyTLSSettings(&tlsCfg, settings, resolver, "grpc"); err != nil {
		return err
	}
	opts.RootCAs = tlsCfg.RootCAs
	opts.ClientCert = tlsCfg.ClientCert
	opts.ClientKey = tlsCfg.ClientKey
	opts.Insecure = tlsCfg.Insecure
	opts.RootMode = tlsCfg.RootMode
	return nil
}

func ApplyHTTPSettings(
	opts *httpclient.Options,
	settings map[string]string,
	resolver *vars.Resolver,
) error {
	if opts == nil {
		return nil
	}
	tlsCfg := tlsconfig.Files{
		RootCAs:    opts.RootCAs,
		ClientCert: opts.ClientCert,
		ClientKey:  opts.ClientKey,
		Insecure:   opts.InsecureSkipVerify,
		RootMode:   opts.RootMode,
	}
	if err := applyTLSSettings(&tlsCfg, settings, resolver, "http"); err != nil {
		return err
	}
	opts.RootCAs = tlsCfg.RootCAs
	opts.ClientCert = tlsCfg.ClientCert
	opts.ClientKey = tlsCfg.ClientKey
	opts.InsecureSkipVerify = tlsCfg.Insecure
	opts.RootMode = tlsCfg.RootMode
	if len(settings) == 0 {
		return nil
	}
	norm := normalize(settings)
	if value, ok := norm["timeout"]; ok {
		if dur, err := time.ParseDuration(value); err == nil {
			opts.Timeout = dur
		}
	}
	if value, ok := norm["proxy"]; ok && strings.TrimSpace(value) != "" {
		opts.ProxyURL = value
	}
	if value, ok := norm["followredirects"]; ok {
		if b, err := strconv.ParseBool(value); err == nil {
			opts.FollowRedirects = b
		}
	}
	if value, ok := norm["insecure"]; ok {
		if b, err := strconv.ParseBool(value); err == nil {
			opts.InsecureSkipVerify = b
		}
	}
	return nil
}

func applyTLSSettings(
	cfg *tlsconfig.Files,
	settings map[string]string,
	resolver *vars.Resolver,
	prefix string,
) error {
	if cfg == nil || len(settings) == 0 {
		return nil
	}
	resolve := func(val string, label string) (string, error) {
		trimmed := strings.TrimSpace(val)
		if trimmed == "" {
			return "", nil
		}
		if resolver == nil {
			return trimmed, nil
		}
		expanded, err := resolver.ExpandTemplates(trimmed)
		if err != nil {
			return "", errdef.Wrap(errdef.CodeHTTP, err, "expand %s", label)
		}
		return strings.TrimSpace(expanded), nil
	}
	prefixLower := strings.ToLower(strings.TrimSpace(prefix))
	norm := normalize(settings)

	if rawMode := firstSetting(norm, prefixLower+"-root-mode"); rawMode != "" {
		mode := strings.ToLower(strings.TrimSpace(rawMode))
		switch mode {
		case string(tlsconfig.RootModeAppend):
			cfg.RootMode = tlsconfig.RootModeAppend
		case string(tlsconfig.RootModeReplace):
			cfg.RootMode = tlsconfig.RootModeReplace
		default:
			return errdef.New(
				errdef.CodeHTTP,
				"invalid %s-root-mode %q (use append or replace)",
				prefixLower,
				rawMode,
			)
		}
	}

	if b, ok := resolveBool(norm, prefixLower+"-insecure"); ok {
		cfg.Insecure = b
	}
	val, err := resolveSetting(
		norm,
		prefixLower+"-client-cert",
		prefixLower+" client cert",
		resolve,
	)
	if err != nil {
		return err
	}
	if val != "" {
		cfg.ClientCert = val
	}
	val, err = resolveSetting(norm, prefixLower+"-client-key", prefixLower+" client key", resolve)
	if err != nil {
		return err
	}
	if val != "" {
		cfg.ClientKey = val
	}

	if raw := firstSetting(norm, prefixLower+"-root-cas", prefixLower+"-root-ca"); raw != "" {
		paths := splitList(raw)
		resolved := make([]string, 0, len(paths))
		for _, p := range paths {
			if p == "" {
				continue
			}
			val, err := resolve(p, prefixLower+" root ca")
			if err != nil {
				return err
			}
			if val != "" {
				resolved = append(resolved, val)
			}
		}
		if len(resolved) > 0 {
			cfg.RootCAs = resolved
		}
	}
	return nil
}

func normalize(settings map[string]string) map[string]string {
	norm := make(map[string]string, len(settings))
	for k, v := range settings {
		norm[strings.ToLower(strings.TrimSpace(k))] = v
	}
	return norm
}

func firstSetting(m map[string]string, keys ...string) string {
	for _, k := range keys {
		if val, ok := m[k]; ok && strings.TrimSpace(val) != "" {
			return val
		}
	}
	return ""
}

func resolveSetting(
	norm map[string]string,
	key, label string,
	expand func(string, string) (string, error),
) (string, error) {
	raw, ok := norm[key]
	if !ok {
		return "", nil
	}
	return expand(raw, label)
}

func resolveBool(norm map[string]string, key string) (bool, bool) {
	raw, ok := norm[key]
	if !ok {
		return false, false
	}
	val, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, false
	}
	return val, true
}

func splitList(raw string) []string {
	seps := func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n'
	}
	return strings.FieldsFunc(raw, seps)
}

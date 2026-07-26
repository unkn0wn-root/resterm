package settings

import (
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/directive"
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
	return httpclient.ApplyOptionSettings(opts, settings)
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
	prefixLower := strings.ToLower(strings.TrimSpace(prefix))
	component := settingsComponent(prefixLower)
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
			return "", diag.WrapAs(
				diag.ClassProtocol,
				err,
				"expand "+label,
				diag.WithComponent(component),
			)
		}
		return strings.TrimSpace(expanded), nil
	}
	norm := normalize(settings)

	if raw := firstSetting(norm, prefixLower+"-root-mode"); raw != "" {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case string(tlsconfig.RootModeAppend):
			cfg.RootMode = tlsconfig.RootModeAppend
		case string(tlsconfig.RootModeReplace):
			cfg.RootMode = tlsconfig.RootModeReplace
		default:
			return invalidSetting(component, prefixLower+"-root-mode", raw, "append or replace")
		}
	}

	key := prefixLower + "-insecure"
	if raw, ok := norm[key]; ok {
		b, valid := directive.ParseBool(raw)
		if !valid {
			return invalidSetting(component, key, raw, "true or false")
		}
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

func settingsComponent(prefix string) diag.Component {
	switch prefix {
	case "grpc":
		return diag.ComponentGRPC
	default:
		return diag.ComponentHTTP
	}
}

// Settings come from a file the user edits, so name the key and what it takes.
// The wording matches the generic HTTP settings in the httpclient package.
func invalidSetting(c diag.Component, key, val, want string) error {
	msg := fmt.Sprintf("invalid %s %q (use %s)", key, val, want)
	if strings.TrimSpace(val) == "" {
		// Nothing was written after the key, which reads as missing rather than
		// invalid. "@setting http-insecure" is the usual way to land here.
		msg = fmt.Sprintf("missing %s value (use %s)", key, want)
	}
	return diag.New(diag.ClassProtocol, msg, diag.WithComponent(c))
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

func splitList(raw string) []string {
	seps := func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n'
	}
	return strings.FieldsFunc(raw, seps)
}

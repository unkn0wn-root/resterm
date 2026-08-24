package settings

import (
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/tlsconfig"
	"github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func ApplyGRPCSettings(
	opts *grpcx.Options,
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
	if err := applyTLSSettings(&tlsCfg, settings, resolver, tlsPrefixGRPC); err != nil {
		return err
	}
	opts.RootCAs = tlsCfg.RootCAs
	opts.ClientCert = tlsCfg.ClientCert
	opts.ClientKey = tlsCfg.ClientKey
	opts.Insecure = tlsCfg.Insecure
	opts.RootMode = tlsCfg.RootMode
	if len(settings) == 0 {
		return nil
	}
	return grpcx.ApplyOptionSettings(opts, settings)
}

func ApplyHTTPSettings(
	opts *httpx.Options,
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
	if err := applyTLSSettings(&tlsCfg, settings, resolver, tlsPrefixHTTP); err != nil {
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
	return httpx.ApplyOptionSettings(opts, settings)
}

// tlsPrefix is the setting family a TLS block belongs to. Both families spell
// their keys the same way, so the prefix picks both the keys and the component
// that errors are reported under.
type tlsPrefix string

const (
	tlsPrefixHTTP tlsPrefix = "http"
	tlsPrefixGRPC tlsPrefix = "grpc"
)

func (p tlsPrefix) key(name string) string { return string(p) + "-" + name }

func (p tlsPrefix) label(name string) string { return string(p) + " " + name }

func (p tlsPrefix) component() diag.Component {
	if p == tlsPrefixGRPC {
		return diag.ComponentGRPC
	}
	return diag.ComponentHTTP
}

func applyTLSSettings(
	cfg *tlsconfig.Files,
	settings map[string]string,
	resolver *vars.Resolver,
	prefix tlsPrefix,
) error {
	if cfg == nil || len(settings) == 0 {
		return nil
	}
	component := prefix.component()
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
	// A single scope, so this only canonicalizes the keys.
	norm := Merge(settings)

	// Read straight from the map. firstSetting skips written-empty values, and
	// those have to reach the missing-value error like every other setting.
	mode := prefix.key("root-mode")
	if raw, ok := norm[mode]; ok {
		switch util.LowerTrim(raw) {
		case string(tlsconfig.RootModeAppend):
			cfg.RootMode = tlsconfig.RootModeAppend
		case string(tlsconfig.RootModeReplace):
			cfg.RootMode = tlsconfig.RootModeReplace
		default:
			return invalidSetting(component, mode, raw, "append or replace")
		}
	}

	key := prefix.key("insecure")
	if raw, ok := norm[key]; ok {
		b, valid := directive.ParseBool(raw)
		if !valid {
			return invalidSetting(component, key, raw, "true or false")
		}
		cfg.Insecure = b
	}
	val, err := resolveSetting(norm, prefix.key("client-cert"), prefix.label("client cert"), resolve)
	if err != nil {
		return err
	}
	if val != "" {
		cfg.ClientCert = val
	}
	val, err = resolveSetting(norm, prefix.key("client-key"), prefix.label("client key"), resolve)
	if err != nil {
		return err
	}
	if val != "" {
		cfg.ClientKey = val
	}

	if raw := firstSetting(norm, prefix.key("root-cas"), prefix.key("root-ca")); raw != "" {
		paths := splitList(raw)
		resolved := make([]string, 0, len(paths))
		for _, p := range paths {
			if p == "" {
				continue
			}
			val, err := resolve(p, prefix.label("root ca"))
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

// Settings come from a file the user edits, so name the key and what it takes.
// The wording matches the generic HTTP settings in protocol/httpx.
func invalidSetting(c diag.Component, key, val, want string) error {
	msg := fmt.Sprintf("invalid %s %q (use %s)", key, val, want)
	if strings.TrimSpace(val) == "" {
		// Nothing was written after the key, which reads as missing rather than
		// invalid. "@setting http-insecure" is the usual way to land here.
		msg = fmt.Sprintf("missing %s value (use %s)", key, want)
	}
	return diag.New(diag.ClassProtocol, msg, diag.WithComponent(c))
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

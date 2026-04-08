package authcmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/duration"
	"github.com/unkn0wn-root/resterm/internal/errdef"
)

var shellNames = map[string]struct{}{
	"bash":       {},
	"cmd":        {},
	"fish":       {},
	"powershell": {},
	"pwsh":       {},
	"sh":         {},
	"zsh":        {},
}

func Parse(params map[string]string, dir string) (Config, error) {
	cfg := Config{Dir: dir}

	var err error
	if trim(params["argv"]) != "" {
		if cfg.Argv, err = parseArgv(params["argv"]); err != nil {
			return cfg, err
		}
	}
	cfg.Format = Format(params["format"])
	cfg.Header = params["header"]
	cfg.Scheme = params["scheme"]
	cfg.TokenPath = params["token_path"]
	cfg.TypePath = params["type_path"]
	cfg.ExpiryPath = params["expiry_path"]
	cfg.ExpiresInPath = params["expires_in_path"]
	cfg.CacheKey = params["cache_key"]

	if cfg.TTL, err = parseDur(params["ttl"], "ttl"); err != nil {
		return cfg, err
	}
	if cfg.Timeout, err = parseDur(params["timeout"], "timeout"); err != nil {
		return cfg, err
	}
	cfg = cfg.normalize()
	if err := validateParsed(cfg); err != nil {
		return cfg, err
	}
	if cfg.usesCache() {
		return cfg, nil
	}
	return Finalize(cfg)
}

func Finalize(cfg Config) (Config, error) {
	cfg = cfg.normalize()
	if err := validate(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func parseArgv(raw string) ([]string, error) {
	src := trim(raw)
	if src == "" {
		return nil, errdef.New(errdef.CodeHTTP, "@auth command requires argv")
	}

	var argv []string
	if err := json.Unmarshal([]byte(src), &argv); err != nil {
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "decode command argv")
	}
	if len(argv) == 0 {
		return nil, errdef.New(errdef.CodeHTTP, "command argv must not be empty")
	}
	return argv, nil
}

func parseDur(raw, name string) (time.Duration, error) {
	value := trim(raw)
	if value == "" {
		return 0, nil
	}
	d, ok := duration.Parse(value)
	if !ok {
		return 0, errdef.New(errdef.CodeHTTP, "invalid %s duration %q", name, raw)
	}
	if d < 0 {
		return 0, errdef.New(errdef.CodeHTTP, "%s must not be negative", name)
	}
	if d == 0 {
		return 0, errdef.New(errdef.CodeHTTP, "%s must be greater than zero", name)
	}
	return d, nil
}

func validate(cfg Config) error {
	if err := validateParsed(cfg); err != nil {
		return err
	}
	if len(cfg.Argv) == 0 {
		return missingArgvError(cfg)
	}
	if effectiveFormat(cfg) == FormatJSON && cfg.TokenPath == "" {
		return errdef.New(errdef.CodeHTTP, "token_path is required for format=json")
	}
	return nil
}

func validateParsed(cfg Config) error {
	if len(cfg.Argv) == 0 && !cfg.usesCache() {
		return missingArgvError(cfg)
	}
	if len(cfg.Argv) > 0 && cfg.Argv[0] == "" {
		return errdef.New(errdef.CodeHTTP, "command argv[0] must not be empty")
	}
	if len(cfg.Argv) > 0 && isShell(cfg.Argv[0]) {
		return errdef.New(
			errdef.CodeHTTP,
			"@auth command does not allow shell front-end %q",
			filepath.Base(cfg.Argv[0]),
		)
	}
	switch effectiveFormat(cfg) {
	case FormatText, FormatJSON:
	default:
		return errdef.New(errdef.CodeHTTP, "unsupported command auth format %q", cfg.Format)
	}
	if cfg.TTL < 0 {
		return errdef.New(errdef.CodeHTTP, "ttl must not be negative")
	}
	if cfg.Timeout < 0 {
		return errdef.New(errdef.CodeHTTP, "timeout must not be negative")
	}
	if cfg.TTL > 0 && !cfg.usesCache() {
		return errdef.New(errdef.CodeHTTP, "ttl requires cache_key")
	}
	return nil
}

func effectiveFormat(cfg Config) Format {
	if cfg.Format == "" {
		return FormatText
	}
	return cfg.Format
}

func missingArgvError(cfg Config) error {
	if cfg.usesCache() {
		return errdef.New(
			errdef.CodeHTTP,
			"@auth command requires argv (include it once per cache_key to seed the cache)",
		)
	}
	return errdef.New(errdef.CodeHTTP, "@auth command requires argv")
}

func isShell(cmd string) bool {
	base := strings.ToLower(filepath.Base(cmd))
	base = strings.TrimSuffix(base, ".exe")
	_, ok := shellNames[base]
	return ok
}

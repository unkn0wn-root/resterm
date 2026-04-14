package oauth

import "strings"

const (
	GrantClientCredentials = "client_credentials"
	GrantPassword          = "password"
	GrantAuthorizationCode = "authorization_code"

	ClientAuthBasic = "basic"
	ClientAuthBody  = "body"

	DefaultHeader = "Authorization"
)

// Normalized canonicalizes user-provided values while preserving whether
// optional fields were omitted, so cached config inheritance still works.
func (cfg Config) Normalized() Config {
	normalizeFields(
		trim,
		&cfg.TokenURL,
		&cfg.AuthURL,
		&cfg.RedirectURL,
		&cfg.ClientID,
		&cfg.ClientSecret,
		&cfg.Scope,
		&cfg.Audience,
		&cfg.Resource,
		&cfg.Username,
		&cfg.Password,
		&cfg.Header,
		&cfg.CacheKey,
		&cfg.Code,
		&cfg.CodeVerifier,
		&cfg.State,
	)
	normalizeFields(
		lowerTrim,
		&cfg.ClientAuth,
		&cfg.GrantType,
		&cfg.CodeMethod,
	)
	cfg.Extra = normalizeExtras(cfg.Extra)
	return cfg
}

// Resolved applies runtime defaults after config inheritance has run.
func (cfg Config) Resolved() Config {
	cfg = cfg.Normalized()
	if cfg.GrantType == "" {
		cfg.GrantType = GrantClientCredentials
	}
	if cfg.Header == "" {
		cfg.Header = DefaultHeader
	}
	return cfg
}

func normalizeFields(fn func(string) string, fields ...*string) {
	for _, field := range fields {
		*field = fn(*field)
	}
}

func trim(value string) string {
	return strings.TrimSpace(value)
}

func lowerTrim(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeExtras(extra map[string]string) map[string]string {
	if len(extra) == 0 {
		return nil
	}
	out := make(map[string]string, len(extra))
	for rawKey, rawValue := range extra {
		key := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(rawKey), "-", "_"))
		value := strings.TrimSpace(rawValue)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

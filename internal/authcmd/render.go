package authcmd

import (
	"strings"
	"time"
)

func (cfg outputConfig) headerName() string {
	if cfg.Header != "" {
		return cfg.Header
	}
	return defaultHeader
}

func effectiveExpiry(cred credential, ttl time.Duration) time.Time {
	if !cred.Expiry.IsZero() {
		return cred.Expiry
	}
	if ttl > 0 && !cred.FetchedAt.IsZero() {
		return cred.FetchedAt.Add(ttl)
	}
	return time.Time{}
}

func validAt(cred credential, ttl time.Duration, now time.Time) bool {
	expiry := effectiveExpiry(cred, ttl)
	return expiry.IsZero() || now.Before(expiry)
}

func buildHeaderValue(cfg outputConfig, header, tok, typ string) (string, string) {
	if cfg.Scheme != "" {
		return cfg.Scheme + " " + tok, cfg.Scheme
	}
	if strings.EqualFold(header, defaultHeader) {
		if typ == "" {
			typ = defaultScheme
		}
		return typ + " " + tok, typ
	}
	return tok, ""
}

func renderResult(cfg outputConfig, cred credential) Result {
	header := cfg.headerName()
	value, typ := buildHeaderValue(cfg, header, cred.Token, cred.Type)
	return Result{
		Header: header,
		Value:  value,
		Token:  cred.Token,
		Type:   typ,
		Expiry: effectiveExpiry(cred, cfg.TTL),
	}
}

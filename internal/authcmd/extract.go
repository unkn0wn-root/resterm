package authcmd

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

func extract(cfg Config, out []byte, now time.Time) (Result, error) {
	cred, err := extractCredential(cfg, out, now)
	if err != nil {
		return Result{Header: cfg.HeaderName()}, err
	}
	return renderResult(cfg, cred), nil
}

func extractCredential(cfg Config, out []byte, now time.Time) (credential, error) {
	cred := credential{}

	var err error
	switch cfg.Format {
	case FormatJSON:
		cred.Token, cred.Type, cred.Expiry, err = extractJSON(cfg, out, now)
	default:
		cred.Token, err = extractText(out)
	}
	if err != nil {
		return cred, err
	}
	return cred, nil
}

func extractText(out []byte) (string, error) {
	src := strings.TrimSpace(string(out))
	if src == "" {
		return "", errdef.New(errdef.CodeHTTP, "command stdout is empty")
	}

	lines := strings.Split(src, "\n")
	vals := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		vals = append(vals, line)
	}
	switch len(vals) {
	case 0:
		return "", errdef.New(errdef.CodeHTTP, "command stdout is empty")
	case 1:
		return vals[0], nil
	default:
		return "", errdef.New(
			errdef.CodeHTTP,
			"command stdout returned multiple values; use format=json",
		)
	}
}

func extractJSON(cfg Config, out []byte, now time.Time) (string, string, time.Time, error) {
	doc, err := decodeJSON(out)
	if err != nil {
		return "", "", time.Time{}, err
	}

	tok, ok, err := scalarAt(doc, cfg.TokenPath, "token_path")
	if err != nil {
		return "", "", time.Time{}, err
	}
	if !ok || tok == "" {
		return "", "", time.Time{}, errdef.New(
			errdef.CodeHTTP,
			"token_path must resolve to a non-empty scalar",
		)
	}

	typ, _, err := scalarAt(doc, cfg.TypePath, "type_path")
	if err != nil {
		return "", "", time.Time{}, err
	}

	exp, err := expiryAt(doc, cfg, now)
	if err != nil {
		return "", "", time.Time{}, err
	}
	return tok, typ, exp, nil
}

func decodeJSON(out []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(out))
	dec.UseNumber()

	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "decode command stdout as json")
	}

	var extra any
	if err := dec.Decode(&extra); err == nil {
		return nil, errdef.New(errdef.CodeHTTP, "command stdout contains multiple JSON values")
	} else if err != io.EOF {
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "decode command stdout as json")
	}
	return doc, nil
}

func scalarAt(doc any, path, name string) (string, bool, error) {
	if path == "" {
		return "", false, nil
	}

	val, ok := rts.JSONPathGet(doc, path)
	if !ok {
		return "", false, nil
	}

	out, ok := scalarString(val)
	if !ok {
		return "", false, errdef.New(errdef.CodeHTTP, "%s must resolve to a scalar", name)
	}
	return out, true, nil
}

func scalarString(v any) (string, bool) {
	switch x := v.(type) {
	case nil:
		return "", false
	case string:
		return strings.TrimSpace(x), true
	case bool:
		return strconv.FormatBool(x), true
	case json.Number:
		return x.String(), true
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), true
	default:
		return "", false
	}
}

func expiryAt(doc any, cfg Config, now time.Time) (time.Time, error) {
	if cfg.ExpiryPath != "" {
		val, ok, err := scalarAt(doc, cfg.ExpiryPath, "expiry_path")
		if err != nil {
			return time.Time{}, err
		}
		if ok && val != "" {
			return parseExpiry(val)
		}
	}

	if cfg.ExpiresInPath != "" {
		val, ok, err := scalarAt(doc, cfg.ExpiresInPath, "expires_in_path")
		if err != nil {
			return time.Time{}, err
		}
		if ok && val != "" {
			sec, err := parseSeconds(val)
			if err != nil {
				return time.Time{}, err
			}
			return now.Add(sec), nil
		}
	}

	return time.Time{}, nil
}

func parseExpiry(raw string) (time.Time, error) {
	src := trim(raw)
	if src == "" {
		return time.Time{}, errdef.New(errdef.CodeHTTP, "expiry value is empty")
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if ts, err := time.Parse(layout, src); err == nil {
			return ts, nil
		}
	}

	if num, err := strconv.ParseInt(src, 10, 64); err == nil {
		if abs64(num) >= 1_000_000_000_000 {
			return time.UnixMilli(num), nil
		}
		return time.Unix(num, 0), nil
	}

	return time.Time{}, errdef.New(errdef.CodeHTTP, "unsupported expiry value %q", raw)
}

func parseSeconds(raw string) (time.Duration, error) {
	src := trim(raw)
	if src == "" {
		return 0, errdef.New(errdef.CodeHTTP, "expires_in value is empty")
	}

	val, err := strconv.ParseFloat(src, 64)
	if err != nil {
		return 0, errdef.New(errdef.CodeHTTP, "invalid expires_in value %q", raw)
	}
	if val <= 0 {
		return 0, errdef.New(errdef.CodeHTTP, "expires_in must be greater than zero")
	}
	return time.Duration(val * float64(time.Second)), nil
}

func buildHeaderValue(cfg Config, header, tok, typ string) (string, string) {
	if cfg.Scheme != "" {
		return cfg.Scheme + " " + tok, cfg.Scheme
	}
	if strings.EqualFold(header, "authorization") {
		if typ == "" {
			typ = defaultScheme
		}
		return typ + " " + tok, typ
	}
	return tok, ""
}

func renderResult(cfg Config, cred credential) Result {
	cfg = cfg.normalize()
	header := cfg.HeaderName()
	value, typ := buildHeaderValue(cfg, header, cred.Token, cred.Type)
	return Result{
		Header: header,
		Value:  value,
		Token:  cred.Token,
		Type:   typ,
		Expiry: effectiveExpiry(cred, cfg),
	}
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

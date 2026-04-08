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

const unixMillisThreshold = int64(1_000_000_000_000)

type credential struct {
	Token     string
	Type      string
	Expiry    time.Time
	FetchedAt time.Time
}

func extractCredential(cfg extractConfig, out []byte, now time.Time) (credential, error) {
	switch cfg.Format {
	case FormatJSON:
		return extractJSONCredential(cfg, out, now)
	default:
		return extractTextCredential(out)
	}
}

func extractTextCredential(out []byte) (credential, error) {
	tok, err := extractText(out)
	if err != nil {
		return credential{}, err
	}
	return credential{Token: tok}, nil
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

func extractJSONCredential(cfg extractConfig, out []byte, now time.Time) (credential, error) {
	doc, err := decodeJSON(out)
	if err != nil {
		return credential{}, err
	}

	tok, ok, err := scalarAt(doc, cfg.TokenPath, "token_path")
	if err != nil {
		return credential{}, err
	}
	if !ok || tok == "" {
		return credential{}, errdef.New(
			errdef.CodeHTTP,
			"token_path must resolve to a non-empty scalar",
		)
	}

	typ, _, err := scalarAt(doc, cfg.TypePath, "type_path")
	if err != nil {
		return credential{}, err
	}

	exp, err := expiryAt(doc, cfg, now)
	if err != nil {
		return credential{}, err
	}
	return credential{
		Token:  tok,
		Type:   typ,
		Expiry: exp,
	}, nil
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

func expiryAt(doc any, cfg extractConfig, now time.Time) (time.Time, error) {
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
		return parseUnixExpiry(num), nil
	}

	return time.Time{}, errdef.New(errdef.CodeHTTP, "unsupported expiry value %q", raw)
}

// expiry_path accepts both Unix seconds and Unix milliseconds.
// Treat 13 digit or larger magnitudes as ms so negative timestamps work too.
func parseUnixExpiry(num int64) time.Time {
	if num <= -unixMillisThreshold || num >= unixMillisThreshold {
		return time.UnixMilli(num)
	}
	return time.Unix(num, 0)
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

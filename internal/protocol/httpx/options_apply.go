package httpx

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/bytesize"
	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/http/origin"
	"github.com/unkn0wn-root/resterm/internal/http/version"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/str"
)

type optionSettingKey string

const (
	optionSettingBaseURL         optionSettingKey = "base-url"
	optionSettingTimeout         optionSettingKey = "timeout"
	optionSettingProxy           optionSettingKey = "proxy"
	optionSettingFollowRedirects optionSettingKey = "followredirects"
	optionSettingInsecure        optionSettingKey = "insecure"
	optionSettingNoCookies       optionSettingKey = "no-cookies"
	optionSettingMaxResponse     optionSettingKey = "max-response-size"
	optionSettingMaxRedirects    optionSettingKey = "max-redirects"
	optionSettingForwardCreds    optionSettingKey = "forward-credentials-on-redirect"
	optionSettingSSELineBytes    optionSettingKey = "sse-max-line-bytes"
	optionSettingSSEEventBytes   optionSettingKey = "sse-max-event-bytes"
	optionSettingWSMessageBytes  optionSettingKey = "ws-max-message-bytes"
	optionSettingWSCompression   optionSettingKey = "ws-compression"
)

// Settings come from a file the user edits, so name the key and what it takes.
func invalidSetting[K ~string](key K, val, want string) error {
	msg := fmt.Sprintf("invalid %s %q (use %s)", key, val, want)
	if strings.TrimSpace(val) == "" {
		// Nothing was written after the key, which reads as missing rather than
		// invalid. "@setting insecure" is the usual way to land here.
		msg = fmt.Sprintf("missing %s value (use %s)", key, want)
	}
	return diag.New(diag.ClassProtocol, msg, diag.WithComponent(diag.ComponentHTTP))
}

func invalidBool(key optionSettingKey, val string) error {
	return invalidSetting(key, val, "true or false")
}

// ApplyOptionSettings applies the generic HTTP settings recognized by the client
// and reports the first value it cannot read.
func ApplyOptionSettings(opts *Options, settings map[string]string) error {
	if opts == nil || len(settings) == 0 {
		return nil
	}

	norm := normalizeSettings(settings)
	if len(norm) == 0 {
		return nil
	}

	if val, ok := settingValue(norm, version.Key); ok {
		v, valid := version.ParseValue(val)
		if !valid {
			return invalidSetting(version.Key, val, "1.1, 2 or HTTP/1.1, HTTP/2")
		}
		opts.HTTPVersion = v
	}

	if val, ok := settingValue(norm, optionSettingTimeout); ok {
		dur, err := time.ParseDuration(val)
		if err != nil {
			return invalidSetting(optionSettingTimeout, val, "a duration such as 30s")
		}
		opts.Timeout = dur
	}

	// A base URL may hold templates and an absolute request target ignores it
	// entirely, so the raw value is kept here and only the request builder
	// expands and validates it, once a relative target needs it.
	if val, ok := settingValue(norm, optionSettingBaseURL); ok {
		base := strings.TrimSpace(val)
		if base == "" {
			return invalidSetting(
				optionSettingBaseURL,
				val,
				"an absolute HTTP URL such as https://api.example.com/v1/",
			)
		}
		opts.BaseURL = base
	}

	// An empty proxy reads as "not set". Anything else has to be a URL the
	// transport can dial, or it fails much later with a bare dial error.
	if val, ok := settingValue(norm, optionSettingProxy); ok && strings.TrimSpace(val) != "" {
		if u, err := url.Parse(val); err != nil || u.Scheme == "" || u.Host == "" {
			return invalidSetting(optionSettingProxy, val, "a URL such as http://host:8080")
		}
		opts.ProxyURL = val
	}

	if val, ok := settingValue(norm, optionSettingFollowRedirects); ok {
		b, valid := directive.ParseBool(val)
		if !valid {
			return invalidBool(optionSettingFollowRedirects, val)
		}
		opts.FollowRedirects = b
	}

	if val, ok := settingValue(norm, optionSettingInsecure); ok {
		b, valid := directive.ParseBool(val)
		if !valid {
			return invalidBool(optionSettingInsecure, val)
		}
		opts.InsecureSkipVerify = b
	}

	if val, ok := settingValue(norm, optionSettingForwardCreds); ok {
		allowed, err := ParseForwardCredentials(val)
		if err != nil {
			return invalidSetting(
				optionSettingForwardCreds,
				val,
				"origins such as https://cdn.example.com, or true to allow any redirect target",
			)
		}
		opts.ForwardCredentials = allowed
	}

	if val, ok := settingValue(norm, optionSettingMaxRedirects); ok {
		limit, err := parseRedirectLimit(val)
		if err != nil {
			return invalidSetting(
				optionSettingMaxRedirects,
				val,
				"a count such as 20, or none to stop at the first redirect",
			)
		}
		opts.MaxRedirects = restfile.OptOf(limit)
	}

	if val, ok := settingValue(norm, optionSettingMaxResponse); ok {
		size, err := bytesize.ParseBudget(val)
		if err != nil {
			return invalidSetting(
				optionSettingMaxResponse,
				val,
				"a size such as 100mb, or none to read without a limit",
			)
		}
		opts.MaxResponseBytes = size
	}

	for _, size := range []struct {
		key optionSettingKey
		dst *int64
	}{
		{optionSettingSSELineBytes, &opts.SSEMaxLineBytes},
		{optionSettingSSEEventBytes, &opts.SSEMaxEventBytes},
		{optionSettingWSMessageBytes, &opts.WSMaxMessageBytes},
	} {
		val, ok := settingValue(norm, size.key)
		if !ok {
			continue
		}
		limit, err := bytesize.Parse(val)
		if err != nil {
			return invalidSetting(size.key, val, "a size such as 8mb")
		}
		*size.dst = limit
	}

	if val, ok := settingValue(norm, optionSettingWSCompression); ok {
		b, valid := directive.ParseBool(val)
		if !valid {
			return invalidBool(optionSettingWSCompression, val)
		}
		opts.WSCompression = restfile.OptOf(b)
	}

	if val, ok := settingValue(norm, optionSettingNoCookies); ok {
		b, valid := directive.ParseBool(val)
		if !valid {
			return invalidBool(optionSettingNoCookies, val)
		}
		if b {
			opts.CookieJar = nil
		}
	}

	return nil
}

func ParseForwardCredentials(val string) (origin.Set, error) {
	word := str.LowerTrim(val)
	if on, ok := directive.ParseBool(word); ok {
		if on {
			return origin.Any(), nil
		}
		return origin.Set{}, nil
	}
	switch word {
	case "all", "any":
		return origin.Any(), nil
	case "none":
		return origin.Set{}, nil
	}

	allowed, err := origin.ParseSet(val)
	if err != nil {
		return origin.Set{}, err
	}
	if allowed.Empty() {
		return origin.Set{}, errors.New("expected an origin, or true or false")
	}
	return allowed, nil
}

func parseRedirectLimit(val string) (int, error) {
	if str.LowerTrim(val) == "none" || directive.IsOff(val) {
		return 0, nil
	}
	limit, err := strconv.Atoi(strings.TrimSpace(val))
	if err != nil {
		return 0, err
	}
	if limit < 0 {
		return 0, errors.New("count must be non-negative")
	}
	return limit, nil
}

func settingValue[K ~string](settings map[string]string, key K) (string, bool) {
	value, ok := settings[string(key)]
	return value, ok
}

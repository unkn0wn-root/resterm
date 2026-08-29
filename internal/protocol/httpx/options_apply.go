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
	"github.com/unkn0wn-root/resterm/internal/util"
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
	return applyOptionSettings(opts, settings, true)
}

// A value that does not parse is an error only while the settings are being
// applied. The send path re-reads values that were validated then, so outside
// strict mode it keeps what it can read and leaves the rest alone.
func applyOptionSettings(opts *Options, settings map[string]string, strict bool) error {
	if opts == nil || len(settings) == 0 {
		return nil
	}

	norm := normalizeSettings(settings)
	if len(norm) == 0 {
		return nil
	}

	if val, ok := settingValue(norm, version.Key); ok {
		switch v, valid := version.ParseValue(val); {
		case valid:
			opts.HTTPVersion = v
		case strict:
			return invalidSetting(version.Key, val, "1.1, 2 or HTTP/1.1, HTTP/2")
		}
	}

	if val, ok := settingValue(norm, optionSettingTimeout); ok {
		switch dur, err := time.ParseDuration(val); {
		case err == nil:
			opts.Timeout = dur
		case strict:
			return invalidSetting(optionSettingTimeout, val, "a duration such as 30s")
		}
	}

	// A base URL may hold templates and an absolute request target ignores it
	// entirely, so the raw value is kept here and only the request builder
	// expands and validates it, once a relative target needs it.
	if val, ok := settingValue(norm, optionSettingBaseURL); ok {
		switch base := strings.TrimSpace(val); {
		case base != "":
			opts.BaseURL = base
		case strict:
			return invalidSetting(
				optionSettingBaseURL,
				val,
				"an absolute HTTP URL such as https://api.example.com/v1/",
			)
		}
	}

	// An empty proxy reads as "not set". Anything else has to be a URL the
	// transport can dial, or it fails much later with a bare dial error.
	if val, ok := settingValue(norm, optionSettingProxy); ok && strings.TrimSpace(val) != "" {
		switch u, err := url.Parse(val); {
		case err == nil && u.Scheme != "" && u.Host != "":
			opts.ProxyURL = val
		case strict:
			return invalidSetting(optionSettingProxy, val, "a URL such as http://host:8080")
		}
	}

	if val, ok := settingValue(norm, optionSettingFollowRedirects); ok {
		switch b, valid := directive.ParseBool(val); {
		case valid:
			opts.FollowRedirects = b
		case strict:
			return invalidBool(optionSettingFollowRedirects, val)
		}
	}

	if val, ok := settingValue(norm, optionSettingInsecure); ok {
		switch b, valid := directive.ParseBool(val); {
		case valid:
			opts.InsecureSkipVerify = b
		case strict:
			return invalidBool(optionSettingInsecure, val)
		}
	}

	if val, ok := settingValue(norm, optionSettingForwardCreds); ok {
		switch allowed, err := ParseForwardCredentials(val); {
		case err == nil:
			opts.ForwardCredentials = allowed
		case strict:
			return invalidSetting(
				optionSettingForwardCreds,
				val,
				"origins such as https://cdn.example.com, or true to allow any redirect target",
			)
		}
	}

	if val, ok := settingValue(norm, optionSettingMaxRedirects); ok {
		switch limit, err := parseRedirectLimit(val); {
		case err == nil:
			opts.MaxRedirects = restfile.OptOf(limit)
		case strict:
			return invalidSetting(
				optionSettingMaxRedirects,
				val,
				"a count such as 20, or none to stop at the first redirect",
			)
		}
	}

	if val, ok := settingValue(norm, optionSettingMaxResponse); ok {
		switch size, err := bytesize.ParseBudget(val); {
		case err == nil:
			opts.MaxResponseBytes = size
		case strict:
			return invalidSetting(
				optionSettingMaxResponse,
				val,
				"a size such as 100mb, or none to read without a limit",
			)
		}
	}

	if val, ok := settingValue(norm, optionSettingNoCookies); ok {
		switch b, valid := directive.ParseBool(val); {
		case !valid && strict:
			return invalidBool(optionSettingNoCookies, val)
		case b:
			opts.CookieJar = nil
		}
	}

	return nil
}

func ParseForwardCredentials(val string) (origin.Set, error) {
	word := util.LowerTrim(val)
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
	if util.LowerTrim(val) == "none" || directive.IsOff(val) {
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

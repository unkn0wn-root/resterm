package httpx

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/httpver"
)

type optionSettingKey string

const (
	optionSettingTimeout         optionSettingKey = "timeout"
	optionSettingProxy           optionSettingKey = "proxy"
	optionSettingFollowRedirects optionSettingKey = "followredirects"
	optionSettingInsecure        optionSettingKey = "insecure"
	optionSettingNoCookies       optionSettingKey = "no-cookies"
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

	if val, ok := settingValue(norm, httpver.Key); ok {
		switch v, valid := httpver.ParseValue(val); {
		case valid:
			opts.HTTPVersion = v
		case strict:
			return invalidSetting(httpver.Key, val, "1.0, 1.1, 2 or HTTP/1.1, HTTP/2")
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

func settingValue[K ~string](settings map[string]string, key K) (string, bool) {
	value, ok := settings[string(key)]
	return value, ok
}

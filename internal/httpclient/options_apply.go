package httpclient

import (
	"strconv"
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

// A typo here used to be dropped in silence, leaving the setting at a default
// the file never asked for.
func settingBool(key optionSettingKey, value string) (bool, error) {
	if b, ok := directive.ParseBool(value); ok {
		return b, nil
	}
	return false, diag.New(
		diag.ClassProtocol,
		"invalid "+string(key)+" "+strconv.Quote(value)+" (use true or false)",
		diag.WithComponent(diag.ComponentHTTP),
	)
}

// ApplyOptionSettings applies the generic HTTP settings recognized by the client.
// It validates http-version and returns an error for invalid values.
func ApplyOptionSettings(opts *Options, settings map[string]string) error {
	return applyOptionSettings(opts, settings, true)
}

func applyOptionSettings(opts *Options, settings map[string]string, strictVersion bool) error {
	if opts == nil || len(settings) == 0 {
		return nil
	}

	norm := normalizeSettings(settings)
	if len(norm) == 0 {
		return nil
	}

	if raw, ok := settingValue(norm, httpver.Key); ok {
		v, ok := httpver.ParseValue(raw)
		if !ok {
			if strictVersion {
				return diag.New(
					diag.ClassProtocol,
					"invalid http-version "+strconv.Quote(
						raw,
					)+" (use 1.0, 1.1, 2 or HTTP/1.1, HTTP/2)",
					diag.WithComponent(diag.ComponentHTTP),
				)
			}
		} else {
			opts.HTTPVersion = v
		}
	}

	if value, ok := settingValue(norm, optionSettingTimeout); ok {
		if dur, err := time.ParseDuration(value); err == nil {
			opts.Timeout = dur
		}
	}

	if value, ok := settingValue(norm, optionSettingProxy); ok && strings.TrimSpace(value) != "" {
		opts.ProxyURL = value
	}

	if value, ok := settingValue(norm, optionSettingFollowRedirects); ok {
		b, err := settingBool(optionSettingFollowRedirects, value)
		if err != nil {
			return err
		}
		opts.FollowRedirects = b
	}

	if value, ok := settingValue(norm, optionSettingInsecure); ok {
		b, err := settingBool(optionSettingInsecure, value)
		if err != nil {
			return err
		}
		opts.InsecureSkipVerify = b
	}

	if value, ok := settingValue(norm, optionSettingNoCookies); ok {
		b, err := settingBool(optionSettingNoCookies, value)
		if err != nil {
			return err
		}
		if b {
			opts.CookieJar = nil
		}
	}

	return nil
}

func settingValue[K ~string](settings map[string]string, key K) (string, bool) {
	value, ok := settings[string(key)]
	return value, ok
}

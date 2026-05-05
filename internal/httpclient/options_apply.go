package httpclient

import (
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/httpver"
)

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

	if raw, ok := norm[httpver.Key]; ok {
		v, ok := httpver.ParseValue(raw)
		if !ok {
			if strictVersion {
				return diag.New(
					diag.ClassProtocol,
					"invalid http-version "+strconv.Quote(raw)+" (use 1.0, 1.1, 2 or HTTP/1.1, HTTP/2)",
					diag.WithComponent(diag.ComponentHTTP),
				)
			}
		} else {
			opts.HTTPVersion = v
		}
	}

	if value, ok := norm["timeout"]; ok {
		if dur, err := time.ParseDuration(value); err == nil {
			opts.Timeout = dur
		}
	}

	if value, ok := norm["proxy"]; ok && strings.TrimSpace(value) != "" {
		opts.ProxyURL = value
	}

	if value, ok := norm["followredirects"]; ok {
		if b, err := strconv.ParseBool(value); err == nil {
			opts.FollowRedirects = b
		}
	}

	if value, ok := norm["insecure"]; ok {
		if b, err := strconv.ParseBool(value); err == nil {
			opts.InsecureSkipVerify = b
		}
	}

	if value, ok := norm["no-cookies"]; ok {
		if b, err := strconv.ParseBool(value); err == nil && b {
			opts.CookieJar = nil
		}
	}

	return nil
}

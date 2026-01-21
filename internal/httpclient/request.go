package httpclient

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/httpver"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (c *Client) prepareHTTPRequest(
	ctx context.Context,
	req *restfile.Request,
	resolver *vars.Resolver,
	opts Options,
) (*http.Request, Options, error) {
	if req == nil {
		return nil, opts, errdef.New(errdef.CodeHTTP, "request is nil")
	}

	bodyReader, err := c.prepareBody(req, resolver, opts)
	if err != nil {
		return nil, opts, err
	}

	expandedURL := strings.TrimSpace(req.URL)
	if expandedURL == "" {
		return nil, opts, errdef.New(errdef.CodeHTTP, "request url is empty")
	}
	if req.Body.GraphQL == nil || !strings.EqualFold(req.Method, "GET") {
		if resolver != nil {
			expandedURL, err = resolver.ExpandTemplates(expandedURL)
			if err != nil {
				return nil, opts, errdef.Wrap(errdef.CodeHTTP, err, "expand url")
			}
		}
	}

	effectiveOpts := applyRequestSettings(opts, req.Settings)

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, expandedURL, bodyReader)
	if err != nil {
		return nil, opts, errdef.Wrap(errdef.CodeHTTP, err, "build request")
	}
	applyHTTPVersion(httpReq, effectiveOpts.HTTPVersion)
	if verErr := checkHTTPVersionRequest(httpReq, effectiveOpts.HTTPVersion); verErr != nil {
		return nil, effectiveOpts, verErr
	}

	if req.Headers != nil {
		for name, values := range req.Headers {
			for _, value := range values {
				finalValue := value
				if resolver != nil {
					if expanded, expandErr := resolver.ExpandTemplates(value); expandErr == nil {
						finalValue = expanded
					}
				}
				httpReq.Header.Add(name, finalValue)
			}
		}
	}

	if req.Body.GraphQL != nil && !strings.EqualFold(req.Method, "GET") {
		if httpReq.Header.Get("Content-Type") == "" {
			httpReq.Header.Set("Content-Type", "application/json")
		}
	}

	c.applyAuthentication(httpReq, resolver, req.Metadata.Auth)
	return httpReq, effectiveOpts, nil
}

func (c *Client) applyAuthentication(
	req *http.Request,
	resolver *vars.Resolver,
	auth *restfile.AuthSpec,
) {
	if auth == nil || len(auth.Params) == 0 {
		return
	}

	expand := func(value string) string {
		if value == "" {
			return ""
		}
		if resolver == nil {
			return value
		}
		if expanded, err := resolver.ExpandTemplates(value); err == nil {
			return expanded
		}
		return value
	}

	switch strings.ToLower(auth.Type) {
	case "basic":
		user := expand(auth.Params["username"])
		pass := expand(auth.Params["password"])
		if req.Header.Get("Authorization") == "" {
			req.SetBasicAuth(user, pass)
		}
	case "bearer":
		token := expand(auth.Params["token"])
		if req.Header.Get("Authorization") == "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	case "apikey", "api-key":
		placement := strings.ToLower(auth.Params["placement"])
		name := expand(auth.Params["name"])
		value := expand(auth.Params["value"])
		if placement == "query" {
			q := req.URL.Query()
			q.Set(name, value)
			req.URL.RawQuery = q.Encode()
		} else {
			if name == "" {
				name = "X-API-Key"
			}
			if req.Header.Get(name) == "" {
				req.Header.Set(name, value)
			}
		}
	case "header":
		name := expand(auth.Params["header"])
		value := expand(auth.Params["value"])
		if name != "" && req.Header.Get(name) == "" {
			req.Header.Set(name, value)
		}
	}
}

type reqMeta struct {
	headers http.Header
	method  string
	host    string
	length  int64
	te      []string
}

func captureReqMeta(sent *http.Request, resp *http.Response) reqMeta {
	var h http.Header

	// Prefer the final request attached to the response, since redirects and transports can mutate it.
	reqForMeta := sent
	if resp != nil && resp.Request != nil {
		reqForMeta = resp.Request
	}

	if reqForMeta != nil && reqForMeta.Header != nil {
		h = reqForMeta.Header.Clone()
	} else if sent != nil && sent.Header != nil {
		h = sent.Header.Clone()
	}
	if h == nil {
		h = make(http.Header)
	}

	host := ""
	length := int64(0)
	var te []string
	method := ""

	if reqForMeta != nil {
		host = reqForMeta.Host
		if strings.TrimSpace(host) == "" && reqForMeta.URL != nil {
			host = reqForMeta.URL.Host
		}
		length = reqForMeta.ContentLength
		if len(reqForMeta.TransferEncoding) > 0 {
			te = append([]string(nil), reqForMeta.TransferEncoding...)
		}
		method = reqForMeta.Method
	}

	return reqMeta{headers: h, method: method, host: host, length: length, te: te}
}

func applyRequestSettings(opts Options, settings map[string]string) Options {
	norm := normalizeSettings(settings)
	if len(norm) == 0 {
		return opts
	}

	effective := opts

	if value, ok := norm["timeout"]; ok {
		if dur, err := time.ParseDuration(value); err == nil {
			effective.Timeout = dur
		}
	}

	if value, ok := norm["proxy"]; ok && value != "" {
		effective.ProxyURL = value
	}

	if value, ok := norm["followredirects"]; ok {
		if b, err := strconv.ParseBool(value); err == nil {
			effective.FollowRedirects = b
		}
	}

	if value, ok := norm["insecure"]; ok {
		if b, err := strconv.ParseBool(value); err == nil {
			effective.InsecureSkipVerify = b
		}
	}
	if v := resolveHTTPVersion(opts, norm); v != httpver.Unknown {
		effective.HTTPVersion = v
	}

	return effective
}

func normalizeSettings(settings map[string]string) map[string]string {
	if len(settings) == 0 {
		return nil
	}
	norm := make(map[string]string, len(settings))
	for k, v := range settings {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		norm[key] = v
	}
	return norm
}

func applyHTTPVersion(req *http.Request, v httpver.Version) {
	if req == nil {
		return
	}
	switch v {
	case httpver.V10:
		req.Proto = "HTTP/1.0"
		req.ProtoMajor = 1
		req.ProtoMinor = 0
	case httpver.V11:
		req.Proto = "HTTP/1.1"
		req.ProtoMajor = 1
		req.ProtoMinor = 1
	case httpver.V2:
		// HTTP/2 is negotiated by the transport; net/http ignores req.Proto for h2.
	}
}

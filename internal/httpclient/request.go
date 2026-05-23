package httpclient

import (
	"context"
	"net/http"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/httpver"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type authType string

const (
	authTypeBasic  authType = "basic"
	authTypeBearer authType = "bearer"
	authTypeAPIKey authType = "apikey"
	authTypeHeader authType = "header"
)

const (
	authParamUsername  = "username"
	authParamPassword  = "password"
	authParamToken     = "token"
	authParamPlacement = "placement"
	authParamName      = "name"
	authParamValue     = "value"
	authParamHeader    = "header"

	authPlacementQuery   = "query"
	authorizationHeader  = "Authorization"
	defaultAPIKeyHeader  = "X-API-Key"
	bearerTokenPrefix    = "Bearer "
	legacyAPIKeyAuthType = "api-key"
)

func (c *Client) prepareHTTPRequest(
	ctx context.Context,
	req *restfile.Request,
	resolver *vars.Resolver,
	opts Options,
) (*http.Request, Options, error) {
	if req == nil {
		return nil, opts, diag.New(
			diag.ClassProtocol,
			"request is nil",
			diag.WithComponent(diag.ComponentHTTP),
		)
	}

	effective := applyRequestSettings(opts, req.Settings)
	prepared, err := c.prepareRequest(ctx, req, resolver, effective, false)
	if err != nil {
		return nil, effective, err
	}
	return prepared.request, prepared.options, nil
}

func (c *Client) prepareHTTPRequestWithOpts(
	ctx context.Context,
	req *restfile.Request,
	resolver *vars.Resolver,
	opts Options,
) (*http.Request, Options, error) {
	prepared, err := c.prepareRequest(ctx, req, resolver, opts, false)
	if err != nil {
		return nil, opts, err
	}
	return prepared.request, prepared.options, nil
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
	case string(authTypeBasic):
		user := expand(auth.Params[authParamUsername])
		pass := expand(auth.Params[authParamPassword])
		if req.Header.Get(authorizationHeader) == "" {
			req.SetBasicAuth(user, pass)
		}
	case string(authTypeBearer):
		token := expand(auth.Params[authParamToken])
		if req.Header.Get(authorizationHeader) == "" {
			req.Header.Set(authorizationHeader, bearerTokenPrefix+token)
		}
	case string(authTypeAPIKey), legacyAPIKeyAuthType:
		placement := strings.ToLower(auth.Params[authParamPlacement])
		name := expand(auth.Params[authParamName])
		value := expand(auth.Params[authParamValue])
		if placement == authPlacementQuery {
			q := req.URL.Query()
			q.Set(name, value)
			req.URL.RawQuery = q.Encode()
		} else {
			if name == "" {
				name = defaultAPIKeyHeader
			}
			if req.Header.Get(name) == "" {
				req.Header.Set(name, value)
			}
		}
	case string(authTypeHeader):
		name := expand(auth.Params[authParamHeader])
		value := expand(auth.Params[authParamValue])
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
		if host == "" && reqForMeta.URL != nil {
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
	effective := opts
	_ = applyOptionSettings(&effective, settings, false)
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

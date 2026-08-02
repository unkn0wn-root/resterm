package httpclient

import (
	"context"
	"fmt"
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

// applyAuthentication fails closed: an unresolved variable in any auth param that
// would reach the wire aborts the build instead of sending the literal placeholder.
func (c *Client) applyAuthentication(
	req *http.Request,
	resolver *vars.Resolver,
	auth *restfile.AuthSpec,
) error {
	if auth == nil || len(auth.Params) == 0 {
		return nil
	}

	kind := strings.ToLower(auth.Type)
	expand := func(param string) (string, error) {
		value := auth.Params[param]
		if value == "" || resolver == nil {
			return value, nil
		}
		expanded, err := resolver.ExpandTemplates(value)
		if err != nil {
			op := fmt.Sprintf("expand %s auth %s", kind, param)
			if at := auth.Origin(); at != "" {
				op += " (" + at + ")"
			}
			return "", diag.WrapAs(diag.ClassAuth, err, op)
		}
		return expanded, nil
	}

	switch kind {
	case string(authTypeBasic):
		if req.Header.Get(authorizationHeader) != "" {
			return nil
		}
		user, err := expand(authParamUsername)
		if err != nil {
			return err
		}
		pass, err := expand(authParamPassword)
		if err != nil {
			return err
		}
		req.SetBasicAuth(user, pass)
	case string(authTypeBearer):
		if req.Header.Get(authorizationHeader) != "" {
			return nil
		}
		token, err := expand(authParamToken)
		if err != nil {
			return err
		}
		req.Header.Set(authorizationHeader, bearerTokenPrefix+token)
	case string(authTypeAPIKey), legacyAPIKeyAuthType:
		name, err := expand(authParamName)
		if err != nil {
			return err
		}
		if strings.ToLower(auth.Params[authParamPlacement]) == authPlacementQuery {
			value, err := expand(authParamValue)
			if err != nil {
				return err
			}
			q := req.URL.Query()
			q.Set(name, value)
			req.URL.RawQuery = q.Encode()
			return nil
		}
		if name == "" {
			name = defaultAPIKeyHeader
		}
		if req.Header.Get(name) != "" {
			return nil
		}
		value, err := expand(authParamValue)
		if err != nil {
			return err
		}
		req.Header.Set(name, value)
	case string(authTypeHeader):
		name, err := expand(authParamHeader)
		if err != nil {
			return err
		}
		if name == "" || req.Header.Get(name) != "" {
			return nil
		}
		value, err := expand(authParamValue)
		if err != nil {
			return err
		}
		req.Header.Set(name, value)
	}
	return nil
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

// The settings were validated when they were applied, so re-reading them here
// takes what it can and leaves anything unreadable at its current value.
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

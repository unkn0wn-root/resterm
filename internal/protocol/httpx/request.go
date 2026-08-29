package httpx

import (
	"context"
	"net/http"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

const (
	authParamUsername  = "username"
	authParamPassword  = "password"
	authParamToken     = "token"
	authParamPlacement = "placement"
	authParamName      = "name"
	authParamValue     = "value"
	authParamHeader    = "header"

	authPlacementQuery  = "query"
	authPlacementHeader = "header"
	authorizationHeader = "Authorization"
	defaultAPIKeyHeader = "X-API-Key"
	bearerTokenPrefix   = "Bearer "
	basicPrefix         = "Basic "
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

// applyAuthentication expands every auth param that would reach the wire and
// propagates expansion errors, so a strict resolver fails the build closed
// while a lenient one previews with placeholders intact.
func (c *Client) applyAuthentication(
	req *http.Request,
	resolver *vars.Resolver,
	auth *restfile.AuthSpec,
) ([]string, error) {
	plan, err := ResolveAuth(auth, resolver, req.Header, diag.ComponentHTTP)
	if err != nil {
		return nil, err
	}

	for _, v := range plan.Values {
		switch v.Placement {
		case AuthInQuery:
			q := req.URL.Query()
			q.Set(v.Name, v.Value)
			req.URL.RawQuery = q.Encode()
		default:
			req.Header.Set(v.Name, v.Value)
		}
	}
	return plan.Targets, nil
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
		if key := util.LowerTrim(k); key != "" {
			norm[key] = v
		}
	}
	return norm
}

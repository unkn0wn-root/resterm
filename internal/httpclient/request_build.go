package httpclient

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// BuildHTTPRequest prepares the request with expansions and returns the body bytes for reuse.
func (c *Client) BuildHTTPRequest(
	ctx context.Context,
	req *restfile.Request,
	resolver *vars.Resolver,
	opts Options,
) (*http.Request, Options, []byte, error) {
	if req == nil {
		return nil, opts, nil, errdef.New(errdef.CodeHTTP, "request is nil")
	}

	effective := applyRequestSettings(opts, req.Settings)
	plan, err := c.prepareBody(req, resolver, effective)
	if err != nil {
		return nil, opts, nil, err
	}

	var body []byte
	if plan.rd != nil {
		body, err = io.ReadAll(plan.rd)
		if err != nil {
			return nil, opts, nil, errdef.Wrap(errdef.CodeHTTP, err, "read request body")
		}
	}

	var reader io.Reader
	if plan.rd != nil {
		reader = bytes.NewReader(body)
	}

	httpReq, effective, err := c.buildHTTPRequest(
		ctx,
		req,
		resolver,
		effective,
		reader,
		plan.url,
	)
	if err != nil {
		return nil, effective, nil, err
	}
	return httpReq, effective, body, nil
}

func (c *Client) buildHTTPRequest(
	ctx context.Context,
	req *restfile.Request,
	resolver *vars.Resolver,
	opts Options,
	body io.Reader,
	urlOverride string,
) (*http.Request, Options, error) {
	if req == nil {
		return nil, opts, errdef.New(errdef.CodeHTTP, "request is nil")
	}

	expandedURL := strings.TrimSpace(urlOverride)
	if expandedURL == "" {
		expandedURL = strings.TrimSpace(req.URL)
		if expandedURL == "" {
			return nil, opts, errdef.New(errdef.CodeHTTP, "request url is empty")
		}
		if resolver != nil {
			var err error
			expandedURL, err = resolver.ExpandTemplates(expandedURL)
			if err != nil {
				return nil, opts, errdef.Wrap(errdef.CodeHTTP, err, "expand url")
			}
		}
	} else if resolver != nil {
		var err error
		expandedURL, err = resolver.ExpandTemplates(expandedURL)
		if err != nil {
			return nil, opts, errdef.Wrap(errdef.CodeHTTP, err, "expand url")
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, expandedURL, body)
	if err != nil {
		return nil, opts, errdef.Wrap(errdef.CodeHTTP, err, "build request")
	}
	applyHTTPVersion(httpReq, opts.HTTPVersion)
	if verErr := checkHTTPVersionRequest(httpReq, opts.HTTPVersion); verErr != nil {
		return nil, opts, verErr
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
	return httpReq, opts, nil
}

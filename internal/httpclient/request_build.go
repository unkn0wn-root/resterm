package httpclient

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type preparedHTTPRequest struct {
	request    *http.Request
	options    Options
	optionsSet bool
	body       []byte
}

// BuildHTTPRequest prepares the request with expansions and returns the body bytes for reuse.
func (c *Client) BuildHTTPRequest(
	ctx context.Context,
	req *restfile.Request,
	resolver *vars.Resolver,
	opts Options,
) (*http.Request, Options, []byte, error) {
	if req == nil {
		return nil, opts, nil, diag.New(
			diag.ClassProtocol,
			"request is nil",
			diag.WithComponent(diag.ComponentHTTP),
		)
	}

	effective := applyRequestSettings(opts, req.Settings)
	prepared, err := c.prepareRequest(ctx, req, resolver, effective, true)
	if err != nil {
		if prepared.optionsSet {
			return nil, prepared.options, nil, err
		}
		return nil, opts, nil, err
	}

	return prepared.request, prepared.options, prepared.body, nil
}

func (c *Client) prepareRequest(
	ctx context.Context,
	req *restfile.Request,
	resolver *vars.Resolver,
	opts Options,
	captureBody bool,
) (preparedHTTPRequest, error) {
	if req == nil {
		return preparedHTTPRequest{options: opts, optionsSet: true}, diag.New(
			diag.ClassProtocol,
			"request is nil",
			diag.WithComponent(diag.ComponentHTTP),
		)
	}

	plan, err := c.prepareBody(req, resolver, opts)
	if err != nil {
		return preparedHTTPRequest{}, err
	}

	body, reader, err := requestBodyReader(plan, captureBody)
	if err != nil {
		return preparedHTTPRequest{}, err
	}

	httpReq, effective, err := c.buildHTTPRequest(
		ctx,
		req,
		resolver,
		opts,
		reader,
		plan.effectiveURL(req.URL),
	)
	if err != nil {
		return preparedHTTPRequest{options: effective, optionsSet: true}, err
	}

	return preparedHTTPRequest{
		request:    httpReq,
		options:    effective,
		optionsSet: true,
		body:       body,
	}, nil
}

func requestBodyReader(plan bodyPlan, captureBody bool) ([]byte, io.Reader, error) {
	if plan.rd == nil {
		return nil, nil, nil
	}

	if !captureBody {
		return nil, plan.rd, nil
	}

	body, err := io.ReadAll(plan.rd)
	if err != nil {
		return nil, nil, diag.WrapAs(
			diag.ClassProtocol,
			err,
			"read request body",
			diag.WithComponent(diag.ComponentHTTP),
		)
	}

	return body, bytes.NewReader(body), nil
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
		return nil, opts, diag.New(
			diag.ClassProtocol,
			"request is nil",
			diag.WithComponent(diag.ComponentHTTP),
		)
	}

	expandedURL := urlOverride
	if expandedURL == "" {
		expandedURL = req.URL
	}
	if expandedURL == "" {
		return nil, opts, diag.New(
			diag.ClassProtocol,
			"request url is empty",
			diag.WithComponent(diag.ComponentHTTP),
		)
	}
	if resolver != nil {
		var err error
		expandedURL, err = resolver.ExpandTemplates(expandedURL)
		if err != nil {
			return nil, opts, diag.WrapAs(
				diag.ClassProtocol,
				err,
				"expand url",
				diag.WithComponent(diag.ComponentHTTP),
			)
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, expandedURL, body)
	if err != nil {
		return nil, opts, diag.WrapAs(
			diag.ClassProtocol,
			err,
			"build request",
			diag.WithComponent(diag.ComponentHTTP),
		)
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

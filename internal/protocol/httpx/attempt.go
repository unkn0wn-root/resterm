package httpx

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// AttemptPlan stores one prepared request so it can be sent more than once.
// Templates and authentication are resolved only once.
type AttemptPlan struct {
	owner   *Client
	request *http.Request
	source  *restfile.Request
	options Options
	http    *http.Client
	body    []byte
	hasBody bool
}

// PrepareAttempts resolves a request once and buffers its body for reuse.
func (c *Client) PrepareAttempts(
	ctx context.Context,
	req *restfile.Request,
	resolver *vars.Resolver,
	opts Options,
) (*AttemptPlan, error) {
	prepared, effective, body, err := c.BuildHTTPRequest(ctx, req, resolver, opts)
	if err != nil {
		return nil, err
	}
	client, err := c.httpClient(effective)
	if err != nil {
		return nil, err
	}
	return &AttemptPlan{
		owner:   c,
		request: prepared,
		source:  req,
		options: effective,
		http:    client,
		body:    body,
		hasBody: prepared.Body != nil && prepared.Body != http.NoBody,
	}, nil
}

// Execute sends a new copy of the prepared request.
func (p *AttemptPlan) Execute(ctx context.Context) (*Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Request.Clone reuses Body, so replace it with a new reader.
	req := p.request.Clone(ctx)
	if p.hasBody {
		req.Body = io.NopCloser(bytes.NewReader(p.body))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(p.body)), nil
		}
	} else {
		req.Body = http.NoBody
	}
	return p.owner.executeHTTPRequest(req, p.source, p.options, p.http)
}

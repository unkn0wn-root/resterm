package httpclient

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/httpver"
	"github.com/unkn0wn-root/resterm/internal/k8s"
	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/ssh"
	"github.com/unkn0wn-root/resterm/internal/telemetry"
	"github.com/unkn0wn-root/resterm/internal/tlsconfig"
	"github.com/unkn0wn-root/resterm/internal/vars"
	"nhooyr.io/websocket"
)

type Options struct {
	Timeout            time.Duration
	FollowRedirects    bool
	InsecureSkipVerify bool
	ProxyURL           string
	RootCAs            []string
	RootMode           tlsconfig.RootMode
	ClientCert         string
	ClientKey          string
	HTTPVersion        httpver.Version
	BaseDir            string
	FallbackBaseDirs   []string
	NoFallback         bool
	Trace              bool
	TraceBudget        *nettrace.Budget
	SSH                *ssh.Plan
	K8s                *k8s.Plan
	CookieJar          http.CookieJar
}

type HTTPClientFactory func(Options) (*http.Client, error)

type WebSocketDialer func(
	context.Context,
	string,
	*websocket.DialOptions,
) (*websocket.Conn, *http.Response, error)

type ClientOption func(*Client)

type Client struct {
	fs          FileSystem
	httpFactory HTTPClientFactory
	wsDial      WebSocketDialer
	telemetry   telemetry.Instrumenter
}

func (c *Client) resolveHTTPFactory() HTTPClientFactory {
	if c == nil {
		return nil
	}
	if c.httpFactory != nil {
		return c.httpFactory
	}
	return c.buildHTTPClient
}

func (c *Client) httpClient(opts Options) (*http.Client, error) {
	factory := c.resolveHTTPFactory()
	if factory == nil {
		return nil, diag.New(
			diag.ClassInternal,
			"http client factory unavailable",
			diag.WithComponent(diag.ComponentHTTP),
		)
	}
	return factory(opts)
}

func (c *Client) streamClient(opts Options) (*http.Client, error) {
	client, err := c.httpClient(opts)
	if err != nil {
		return nil, err
	}
	client.Timeout = 0
	return client, nil
}

func NewClient(fs FileSystem) *Client {
	return NewClientWithOptions(WithFileSystem(fs))
}

func NewClientWithOptions(opts ...ClientOption) *Client {
	c := &Client{}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	if c.fs == nil {
		c.fs = OSFileSystem{}
	}
	if c.wsDial == nil {
		c.wsDial = websocket.Dial
	}
	if c.telemetry == nil {
		c.telemetry = telemetry.Noop()
	}
	return c
}

func WithFileSystem(fs FileSystem) ClientOption {
	return func(c *Client) {
		if fs != nil {
			c.fs = fs
		}
	}
}

func WithHTTPFactory(factory HTTPClientFactory) ClientOption {
	return func(c *Client) {
		c.httpFactory = factory
	}
}

func WithWebSocketDialer(dialer WebSocketDialer) ClientOption {
	return func(c *Client) {
		if dialer != nil {
			c.wsDial = dialer
		}
	}
}

func WithTelemetry(instr telemetry.Instrumenter) ClientOption {
	return func(c *Client) {
		if instr == nil {
			instr = telemetry.Noop()
		}
		c.telemetry = instr
	}
}

// Clone returns a snapshot of c's client configuration.
// Later field updates on c do not affect the clone.
func (c *Client) Clone() *Client {
	if c == nil {
		return nil
	}
	return &Client{
		fs:          c.fs,
		httpFactory: c.httpFactory,
		wsDial:      c.wsDial,
		telemetry:   c.telemetry,
	}
}

type Response struct {
	Status         string
	StatusCode     int
	Proto          string
	Headers        http.Header
	ReqMethod      string
	RequestHeaders http.Header
	ReqHost        string
	ReqLen         int64
	ReqTE          []string
	Body           []byte
	Duration       time.Duration
	EffectiveURL   string
	Request        *restfile.Request
	Timeline       *nettrace.Timeline
	TraceReport    *nettrace.Report
}

// Wraps the HTTP roundtrip with telemetry spans and network tracing.
// Trace session hooks into http.Client's transport to capture timing info,
// while the defer ensures we always report metrics even on failure.
func (c *Client) Execute(
	ctx context.Context,
	req *restfile.Request,
	resolver *vars.Resolver,
	opts Options,
) (resp *Response, err error) {
	httpReq, effectiveOpts, err := c.prepareHTTPRequest(ctx, req, resolver, opts)
	if err != nil {
		return nil, err
	}

	client, err := c.httpClient(effectiveOpts)
	if err != nil {
		return nil, err
	}

	proxy := proxyForRequest(httpReq, effectiveOpts, client)

	var (
		timeline    *nettrace.Timeline
		traceSess   *traceSession
		traceReport *nettrace.Report
		k8sDiag     *k8s.RequestDiag
	)

	httpReq, requestSpan := c.startRequestSpan(req, httpReq, effectiveOpts)
	if effectiveOpts.K8s != nil && effectiveOpts.K8s.Active() {
		reqCtx, diag := k8s.BindRequestContext(httpReq.Context())
		httpReq = httpReq.WithContext(reqCtx)
		k8sDiag = diag
	}

	defer func() {
		endRequestSpan(requestSpan, resp, err, timeline, traceReport)
	}()

	if effectiveOpts.Trace {
		traceSess = newTraceSession()
		httpReq = traceSess.bind(httpReq)
	}

	start := time.Now()
	httpResp, err := client.Do(httpReq)
	if err != nil {
		if k8sDiag != nil {
			err = k8s.AnnotateRequestError(err, start, k8sDiag)
		}
		duration := time.Since(start)
		if traceSess != nil {
			traceSess.fail(err)
			timeline = traceSess.complete(buildTraceExtras(httpReq, nil, effectiveOpts, proxy))
			traceReport = buildTraceReport(timeline, effectiveOpts.TraceBudget)
		}
		return partialResp(req, duration, timeline, traceReport), diag.Wrap(
			err,
			"perform request",
			diag.WithComponent(diag.ComponentHTTP),
		)
	}
	if verErr := checkHTTPVersion(httpResp, effectiveOpts.HTTPVersion); verErr != nil {
		duration := time.Since(start)
		if traceSess != nil {
			traceSess.fail(verErr)
			traceSess.finishTransfer(verErr)
			timeline = traceSess.complete(buildTraceExtras(httpReq, httpResp, effectiveOpts, proxy))
			traceReport = buildTraceReport(timeline, effectiveOpts.TraceBudget)
		}
		_ = httpResp.Body.Close()
		return partialResp(req, duration, timeline, traceReport), verErr
	}

	defer func() {
		if closeErr := httpResp.Body.Close(); closeErr != nil && err == nil {
			err = diag.Wrap(
				closeErr,
				"close response body",
				diag.WithComponent(diag.ComponentHTTP),
			)
		}
	}()

	body, err := io.ReadAll(httpResp.Body)
	if traceSess != nil {
		traceSess.finishTransfer(err)
	}
	if err != nil {
		if traceSess != nil {
			traceSess.fail(err)
			traceSess.complete(buildTraceExtras(httpReq, httpResp, effectiveOpts, proxy))
		}
		return nil, diag.Wrap(
			err,
			"read response body",
			diag.WithComponent(diag.ComponentHTTP),
		)
	}

	if traceSess != nil {
		timeline = traceSess.complete(buildTraceExtras(httpReq, httpResp, effectiveOpts, proxy))
		traceReport = buildTraceReport(timeline, effectiveOpts.TraceBudget)
	}
	duration := time.Since(start)

	resp = respFromHTTP(httpReq, httpResp, req, body, duration)
	resp.Timeline = timeline
	resp.TraceReport = traceReport

	return resp, nil
}

func (c *Client) startRequestSpan(
	req *restfile.Request,
	httpReq *http.Request,
	opts Options,
) (*http.Request, telemetry.RequestSpan) {
	instrumenter := c.telemetry
	if !opts.Trace || instrumenter == nil {
		instrumenter = telemetry.Noop()
	}

	spanCtx, requestSpan := instrumenter.Start(httpReq.Context(), telemetry.RequestStart{
		Request:     req,
		HTTPRequest: httpReq,
		Budget:      cloneBudget(opts.TraceBudget),
	})
	return httpReq.WithContext(spanCtx), requestSpan
}

func cloneBudget(budget *nettrace.Budget) *nettrace.Budget {
	if budget == nil {
		return nil
	}
	clone := budget.Clone()
	return &clone
}

func buildTraceReport(tl *nettrace.Timeline, budget *nettrace.Budget) *nettrace.Report {
	if tl == nil {
		return nil
	}
	var reportBudget nettrace.Budget
	if budget != nil {
		reportBudget = budget.Clone()
	}
	return nettrace.NewReport(tl, reportBudget)
}

func endRequestSpan(
	span telemetry.RequestSpan,
	resp *Response,
	err error,
	timeline *nettrace.Timeline,
	report *nettrace.Report,
) {
	if span == nil {
		return
	}
	if timeline != nil || report != nil {
		span.RecordTrace(timeline, report)
	}
	statusCode := 0
	if resp != nil {
		statusCode = resp.StatusCode
	}
	span.End(telemetry.RequestResult{
		Err:        err,
		StatusCode: statusCode,
		Report:     report,
	})
}

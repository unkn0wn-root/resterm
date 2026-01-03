package httpclient

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"

	"github.com/unkn0wn-root/resterm/internal/errdef"
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
	BaseDir            string
	FallbackBaseDirs   []string
	NoFallback         bool
	Trace              bool
	TraceBudget        *nettrace.Budget
	SSH                *ssh.Plan
}

type Client struct {
	fs          FileSystem
	jar         http.CookieJar
	httpFactory func(Options) (*http.Client, error)
	wsDial      func(context.Context, string, *websocket.DialOptions) (*websocket.Conn, *http.Response, error)
	telemetry   telemetry.Instrumenter
}

func (c *Client) resolveHTTPFactory() func(Options) (*http.Client, error) {
	if c == nil {
		return nil
	}
	if c.httpFactory != nil {
		return c.httpFactory
	}
	return c.buildHTTPClient
}

func NewClient(fs FileSystem) *Client {
	if fs == nil {
		fs = OSFileSystem{}
	}

	jar, _ := cookiejar.New(nil)
	c := &Client{fs: fs, jar: jar, telemetry: telemetry.Noop()}
	c.httpFactory = c.buildHTTPClient
	c.wsDial = websocket.Dial
	return c
}

// SetHTTPFactory allows callers to override how http.Client instances are created.
// Passing nil restores the default factory.
func (c *Client) SetHTTPFactory(factory func(Options) (*http.Client, error)) {
	c.httpFactory = factory
}

// SetTelemetry configures the instrumenter used to emit OpenTelemetry spans. Passing nil restores the no-op implementation.
func (c *Client) SetTelemetry(instr telemetry.Instrumenter) {
	if instr == nil {
		instr = telemetry.Noop()
	}
	c.telemetry = instr
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

	factory := c.resolveHTTPFactory()
	if factory == nil {
		return nil, errdef.New(errdef.CodeHTTP, "http client factory unavailable")
	}

	client, err := factory(effectiveOpts)
	if err != nil {
		return nil, err
	}

	proxy := proxyForRequest(httpReq, effectiveOpts, client)

	var (
		timeline    *nettrace.Timeline
		traceSess   *traceSession
		traceReport *nettrace.Report
	)

	instrumenter := c.telemetry
	if !effectiveOpts.Trace || instrumenter == nil {
		instrumenter = telemetry.Noop()
	}

	var budgetCopy *nettrace.Budget
	if effectiveOpts.TraceBudget != nil {
		clone := effectiveOpts.TraceBudget.Clone()
		budgetCopy = &clone
	}

	spanCtx, requestSpan := instrumenter.Start(httpReq.Context(), telemetry.RequestStart{
		Request:     req,
		HTTPRequest: httpReq,
		Budget:      budgetCopy,
	})
	httpReq = httpReq.WithContext(spanCtx)

	defer func() {
		if requestSpan == nil {
			return
		}
		if timeline != nil || traceReport != nil {
			requestSpan.RecordTrace(timeline, traceReport)
		}
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		requestSpan.End(telemetry.RequestResult{
			Err:        err,
			StatusCode: statusCode,
			Report:     traceReport,
		})
	}()

	if effectiveOpts.Trace {
		traceSess = newTraceSession()
		httpReq = traceSess.bind(httpReq)
	}

	buildTraceReport := func(tl *nettrace.Timeline) *nettrace.Report {
		if tl == nil {
			return nil
		}
		var budget nettrace.Budget
		if effectiveOpts.TraceBudget != nil {
			budget = effectiveOpts.TraceBudget.Clone()
		}
		return nettrace.NewReport(tl, budget)
	}

	start := time.Now()
	httpResp, err := client.Do(httpReq)
	if err != nil {
		duration := time.Since(start)
		if traceSess != nil {
			traceSess.fail(err)
			timeline = traceSess.complete(buildTraceExtras(httpReq, nil, effectiveOpts, proxy))
			traceReport = buildTraceReport(timeline)
		}
		return &Response{
				Request:     req,
				Duration:    duration,
				Timeline:    timeline,
				TraceReport: traceReport,
			}, errdef.Wrap(
				errdef.CodeHTTP,
				err,
				"perform request",
			)
	}

	defer func() {
		if closeErr := httpResp.Body.Close(); closeErr != nil && err == nil {
			err = errdef.Wrap(errdef.CodeHTTP, closeErr, "close response body")
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
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "read response body")
	}

	if traceSess != nil {
		timeline = traceSess.complete(buildTraceExtras(httpReq, httpResp, effectiveOpts, proxy))
		traceReport = buildTraceReport(timeline)
	}
	duration := time.Since(start)

	meta := captureReqMeta(httpReq, httpResp)

	resp = &Response{
		Status:         httpResp.Status,
		StatusCode:     httpResp.StatusCode,
		Proto:          httpResp.Proto,
		Headers:        httpResp.Header.Clone(),
		ReqMethod:      meta.method,
		RequestHeaders: meta.headers,
		ReqHost:        meta.host,
		ReqLen:         meta.length,
		ReqTE:          meta.te,
		Body:           body,
		EffectiveURL:   effURL(httpReq, httpResp),
		Request:        req,
		Timeline:       timeline,
		TraceReport:    traceReport,
	}
	resp.Duration = duration

	return resp, nil
}

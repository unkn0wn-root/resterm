package telemetry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

var (
	tracerName  = "github.com/unkn0wn-root/resterm/internal/telemetry"
	httpHostKey = attribute.Key("http.host")
)

type Instrumenter interface {
	Start(ctx context.Context, info RequestStart) (context.Context, RequestSpan)
	Shutdown(ctx context.Context) error
}

type RequestStart struct {
	Request     *restfile.Request
	HTTPRequest *http.Request
	Budget      *nettrace.Budget
}

type RequestResult struct {
	Err        error
	StatusCode int
	Report     *nettrace.Report
}

type RequestSpan interface {
	RecordTrace(tl *nettrace.Timeline, report *nettrace.Report)
	End(result RequestResult)
}

type providerOptions struct {
	exporter       sdktrace.SpanExporter
	spanProcessors []sdktrace.SpanProcessor
}

type Option func(*providerOptions)

func WithSpanProcessor(proc sdktrace.SpanProcessor) Option {
	return func(opts *providerOptions) {
		if proc != nil {
			opts.spanProcessors = append(opts.spanProcessors, proc)
		}
	}
}

func WithExporter(exp sdktrace.SpanExporter) Option {
	return func(opts *providerOptions) {
		if exp != nil {
			opts.exporter = exp
		}
	}
}

type manager struct {
	tracer   trace.Tracer
	provider *sdktrace.TracerProvider
	shutdown sync.Once
}

func New(cfg Config, opts ...Option) (Instrumenter, error) {
	builder := providerOptions{}
	for _, opt := range opts {
		opt(&builder)
	}

	if !cfg.Enabled() && builder.exporter == nil && len(builder.spanProcessors) == 0 {
		return Noop(), nil
	}

	res, err := resource.New(
		context.Background(),
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithAttributes(buildResourceAttributes(cfg)...),
	)
	if err != nil {
		return nil, err
	}

	exporter := builder.exporter
	if exporter == nil && cfg.Enabled() {
		exporter, err = newExporter(cfg)
		if err != nil {
			return nil, err
		}
	}

	var tpOpts []sdktrace.TracerProviderOption
	tpOpts = append(tpOpts, sdktrace.WithResource(res))
	if exporter != nil {
		tpOpts = append(tpOpts, sdktrace.WithBatcher(exporter))
	}
	for _, proc := range builder.spanProcessors {
		tpOpts = append(tpOpts, sdktrace.WithSpanProcessor(proc))
	}

	tp := sdktrace.NewTracerProvider(tpOpts...)
	return &manager{tracer: tp.Tracer(tracerName), provider: tp}, nil
}

func (m *manager) Start(ctx context.Context, info RequestStart) (context.Context, RequestSpan) {
	if info.HTTPRequest == nil {
		return ctx, noopSpan{}
	}

	attrs := buildSpanAttributes(info)
	spanName := spanNameFor(info)
	ctx, span := m.tracer.Start(
		ctx,
		spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
	return ctx, &requestSpan{span: span}
}

func (m *manager) Shutdown(ctx context.Context) error {
	if m == nil || m.provider == nil {
		return nil
	}
	var shutdownErr error
	m.shutdown.Do(func() {
		shutdownErr = m.provider.Shutdown(ctx)
	})
	return shutdownErr
}

type requestSpan struct {
	span trace.Span
}

func (rs *requestSpan) RecordTrace(tl *nettrace.Timeline, report *nettrace.Report) {
	if rs == nil || rs.span == nil || tl == nil {
		return
	}

	rs.span.SetAttributes(attribute.Int64("resterm.trace.duration_ms", tl.Duration.Milliseconds()))
	if !tl.Started.IsZero() {
		rs.span.SetAttributes(
			attribute.String("resterm.trace.started_at", tl.Started.Format(time.RFC3339Nano)),
		)
	}
	if !tl.Completed.IsZero() {
		rs.span.SetAttributes(
			attribute.String("resterm.trace.completed_at", tl.Completed.Format(time.RFC3339Nano)),
		)
	}
	if strings.TrimSpace(tl.Err) != "" {
		rs.span.AddEvent(
			"resterm.trace.error",
			trace.WithAttributes(attribute.String("resterm.error", tl.Err)),
		)
	}

	for _, phase := range tl.Phases {
		attrs := []attribute.KeyValue{
			attribute.String("resterm.trace.phase", string(phase.Kind)),
			attribute.Int64("resterm.trace.phase_duration_ms", phase.Duration.Milliseconds()),
		}
		if phase.Meta.Addr != "" {
			attrs = append(attrs, attribute.String("resterm.trace.addr", phase.Meta.Addr))
		}
		if phase.Meta.Reused {
			attrs = append(attrs, attribute.Bool("resterm.trace.reused", true))
		}
		if phase.Meta.Cached {
			attrs = append(attrs, attribute.Bool("resterm.trace.cached", true))
		}
		if strings.TrimSpace(phase.Err) != "" {
			attrs = append(attrs, attribute.String("resterm.trace.phase_error", phase.Err))
		}

		options := []trace.EventOption{trace.WithAttributes(attrs...)}
		if !phase.End.IsZero() {
			options = append(options, trace.WithTimestamp(phase.End))
		}
		rs.span.AddEvent("resterm.trace.phase", options...)
	}

	if report != nil {
		withinLimit := len(report.BudgetReport.Breaches) == 0
		rs.span.SetAttributes(attribute.Bool("resterm.trace.within_budget", withinLimit))
		for _, breach := range report.BudgetReport.Breaches {
			attrs := []attribute.KeyValue{
				attribute.String("resterm.trace.breach_phase", string(breach.Kind)),
				attribute.Int64("resterm.trace.breach_limit_ms", breach.Limit.Milliseconds()),
				attribute.Int64("resterm.trace.breach_actual_ms", breach.Actual.Milliseconds()),
				attribute.Int64("resterm.trace.breach_over_ms", breach.Over.Milliseconds()),
			}
			rs.span.AddEvent("resterm.trace.budget_breach", trace.WithAttributes(attrs...))
		}
	}
}

func (rs *requestSpan) End(result RequestResult) {
	if rs == nil || rs.span == nil {
		return
	}

	if result.StatusCode > 0 {
		rs.span.SetAttributes(semconv.HTTPStatusCodeKey.Int(result.StatusCode))
	}

	statusCode := codes.Unset
	statusMsg := ""

	if result.Err != nil {
		rs.span.RecordError(result.Err)
		statusCode = codes.Error
		statusMsg = result.Err.Error()
	}

	if result.Report != nil && len(result.Report.BudgetReport.Breaches) > 0 {
		if statusCode != codes.Error {
			statusCode = codes.Error
			statusMsg = "trace budget breach"
		}
	}

	if result.Err == nil &&
		(result.Report == nil || len(result.Report.BudgetReport.Breaches) == 0) &&
		result.StatusCode >= 400 {
		statusCode = codes.Error
		statusMsg = fmt.Sprintf("HTTP %d", result.StatusCode)
	}

	if statusCode == codes.Unset {
		statusCode = codes.Ok
		statusMsg = "OK"
	}

	rs.span.SetStatus(statusCode, statusMsg)
	rs.span.End()
}

func Noop() Instrumenter {
	return noopInstrumenter{}
}

type noopInstrumenter struct{}

type noopSpan struct{}

func (noopInstrumenter) Start(ctx context.Context, _ RequestStart) (context.Context, RequestSpan) {
	return ctx, noopSpan{}
}

func (noopInstrumenter) Shutdown(context.Context) error { return nil }

func (noopSpan) RecordTrace(*nettrace.Timeline, *nettrace.Report) {}

func (noopSpan) End(RequestResult) {}

func newExporter(cfg Config) (sdktrace.SpanExporter, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, errors.New("telemetry endpoint is required")
	}

	timeout := cfg.DialTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	clientOpts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
	}
	if cfg.Insecure {
		clientOpts = append(clientOpts, otlptracegrpc.WithInsecure())
	}
	if len(cfg.Headers) > 0 {
		clientOpts = append(clientOpts, otlptracegrpc.WithHeaders(cfg.Headers))
	}

	client := otlptracegrpc.NewClient(clientOpts...)
	return otlptrace.New(ctx, client)
}

func buildResourceAttributes(cfg Config) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(cfg.ServiceName),
	}
	if strings.TrimSpace(cfg.Version) != "" {
		attrs = append(attrs, semconv.ServiceVersion(cfg.Version))
	}
	return attrs
}

func buildSpanAttributes(info RequestStart) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.Bool("resterm.trace.enabled", true),
	}

	req := info.HTTPRequest
	if req.Method != "" {
		attrs = append(attrs, semconv.HTTPMethodKey.String(req.Method))
	} else if info.Request != nil && info.Request.Method != "" {
		attrs = append(attrs, semconv.HTTPMethodKey.String(info.Request.Method))
	}

	if req.URL != nil {
		if scheme := req.URL.Scheme; scheme != "" {
			attrs = append(attrs, semconv.HTTPSchemeKey.String(scheme))
		}
		if host := req.URL.Host; host != "" {
			attrs = append(attrs, httpHostKey.String(host))
		}
		if target := req.URL.RequestURI(); target != "" {
			attrs = append(attrs, semconv.HTTPTargetKey.String(target))
		}
		if full := req.URL.String(); full != "" {
			attrs = append(attrs, semconv.HTTPURLKey.String(full))
		}
	}

	if info.Request != nil {
		meta := info.Request.Metadata
		if name := strings.TrimSpace(meta.Name); name != "" {
			attrs = append(attrs, attribute.String("resterm.request.name", name))
		}
		if desc := strings.TrimSpace(meta.Description); desc != "" {
			attrs = append(attrs, attribute.String("resterm.request.description", desc))
		}
		if len(meta.Tags) > 0 {
			attrs = append(
				attrs,
				attribute.String("resterm.request.tags", strings.Join(meta.Tags, ",")),
			)
		}
	}

	if info.Budget != nil {
		budget := info.Budget.Clone()
		if budget.Total > 0 {
			attrs = append(
				attrs,
				attribute.Int64("resterm.trace.budget.total_ms", budget.Total.Milliseconds()),
			)
		}
		if budget.Tolerance > 0 {
			attrs = append(
				attrs,
				attribute.Int64(
					"resterm.trace.budget.tolerance_ms",
					budget.Tolerance.Milliseconds(),
				),
			)
		}
		for phase, limit := range budget.Phases {
			key := fmt.Sprintf("resterm.trace.budget.%s_ms", phase)
			attrs = append(attrs, attribute.Int64(key, limit.Milliseconds()))
		}
	}

	return attrs
}

func spanNameFor(info RequestStart) string {
	if info.Request != nil {
		if name := strings.TrimSpace(info.Request.Metadata.Name); name != "" {
			return name
		}
	}
	if info.HTTPRequest != nil && info.HTTPRequest.Method != "" {
		if info.HTTPRequest.URL != nil && info.HTTPRequest.URL.Host != "" {
			return fmt.Sprintf("%s %s", info.HTTPRequest.Method, info.HTTPRequest.URL.Host)
		}
		return info.HTTPRequest.Method
	}
	return "http.request"
}

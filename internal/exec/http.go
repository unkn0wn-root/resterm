package exec

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

const (
	StageCaptures = "captures"

	decisionSent = "HTTP request sent"
)

type HTTPInput struct {
	Client           *httpx.Client
	Scripts          *scripts.Runner
	Context          context.Context
	Doc              *restfile.Document
	Req              *restfile.Request
	Resolver         *vars.Resolver
	Options          httpx.Options
	EffectiveTimeout time.Duration
	ScriptVars       vars.NameMap[string]
	Locals           rts.Locals
	Secrets          *vars.Secrets
}

type CaptureInput struct {
	Doc      *restfile.Document
	Req      *restfile.Request
	Resolver *vars.Resolver
	Response *scripts.Response
	Stream   *scripts.StreamInfo
	Vars     map[string]string
	Locals   rts.Locals
}

type AssertInput struct {
	Context context.Context
	Doc     *restfile.Document
	Req     *restfile.Request
	BaseDir string
	Vars    map[string]string
	Locals  rts.Locals
	HTTP    *httpx.Response
	Stream  *scripts.StreamInfo
}

type PredicateInput struct {
	Context   context.Context
	Doc       *restfile.Document
	Req       *restfile.Request
	BaseDir   string
	Vars      map[string]string
	Locals    rts.Locals
	HTTP      *httpx.Response
	Predicate restfile.ResponsePredicate
}

type RepeatPhase uint8

const (
	RepeatAttempt RepeatPhase = iota
	RepeatRetryWait
	RepeatPollWait
)

type RepeatProgress struct {
	Phase      RepeatPhase
	Attempt    int
	Poll       int
	StatusCode int
	Delay      time.Duration
	Err        error
}

type RepeatCount struct {
	Attempts int
	Polls    int
}

func (c RepeatCount) describe(decision string) string {
	switch {
	case c.Polls > 1:
		return fmt.Sprintf(
			"%s after %d attempts across %d polling cycles",
			decision,
			c.Attempts,
			c.Polls,
		)
	case c.Attempts > 1:
		return fmt.Sprintf("%s after %d attempts", decision, c.Attempts)
	}
	return decision
}

type HTTPHooks struct {
	AttachSSEHandle       func(*httpx.StreamHandle, *restfile.Request)
	AttachWebSocketHandle func(*httpx.WebSocketHandle, *restfile.Request)
	ApplyCaptures         func(CaptureInput) error
	// CollectVariables rebuilds variables after pre-request scripts; the third
	// argument contains their writes at script precedence.
	CollectVariables    func(*restfile.Document, *restfile.Request, vars.NameMap[string]) map[string]string
	CollectGlobalValues func(*restfile.Document) vars.Globals
	RunAsserts          func(AssertInput) ([]scripts.TestResult, error)
	EvaluatePredicate   func(PredicateInput) (bool, error)
	ApplyRuntimeGlobals func(vars.Globals)
	OnRepeatProgress    func(RepeatProgress)
	Warn                func(string)
}

type HTTPResult struct {
	Response  *httpx.Response
	Stream    *scripts.StreamInfo
	Tests     []scripts.TestResult
	ScriptErr error
	Err       error
	Decision  string
	ErrStage  string
	Repeat    RepeatCount
}

type Runner struct {
	Hooks HTTPHooks
}

func (r Runner) RunHTTP(in HTTPInput) HTTPResult {
	if in.Client == nil {
		return HTTPResult{
			Err:      diag.New(diag.ClassProtocol, "http client is not initialised"),
			Decision: "HTTP request failed",
		}
	}
	if in.Scripts == nil {
		in.Scripts = scripts.NewRunner(nil)
	}
	if in.Req == nil {
		return HTTPResult{
			Err:      diag.New(diag.ClassProtocol, "request is nil"),
			Decision: "HTTP request failed",
		}
	}
	if in.Req.WebSocket != nil && len(in.Req.WebSocket.Steps) == 0 {
		return HTTPResult{
			Err: diag.New(
				diag.ClassProtocol,
				"interactive websocket execution requires caller-managed session handling",
			),
			Decision: "WebSocket request failed",
		}
	}

	ctx := in.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if in.Req.Metadata.Poll != nil || in.Req.Metadata.Retry != nil {
		return r.runRepeatedHTTP(ctx, in)
	}
	ctx, cancel := context.WithTimeout(ctx, in.EffectiveTimeout)
	defer cancel()

	res := HTTPResult{Decision: "HTTP request sent"}

	var (
		resp *httpx.Response
		err  error
	)

	switch {
	case in.Req.WebSocket != nil:
		handle, fallback, startErr := in.Client.StartWebSocket(
			ctx,
			in.Req,
			in.Resolver,
			in.Options,
		)
		if startErr != nil {
			res.Err = startErr
			res.Decision = "WebSocket request failed"
			return res
		}
		if fallback != nil {
			resp = fallback
		} else {
			if r.Hooks.AttachWebSocketHandle != nil {
				r.Hooks.AttachWebSocketHandle(handle, in.Req)
			}
			resp, err = in.Client.CompleteWebSocket(ctx, handle, in.Req, in.Options)
		}
	case in.Req.SSE != nil:
		handle, fallback, startErr := in.Client.StartSSE(ctx, in.Req, in.Resolver, in.Options)
		if startErr != nil {
			res.Err = startErr
			res.Decision = "SSE request failed"
			return res
		}
		if fallback != nil {
			resp = fallback
		} else {
			if r.Hooks.AttachSSEHandle != nil {
				r.Hooks.AttachSSEHandle(handle, in.Req)
			}
			resp, err = httpx.CompleteSSE(handle)
		}
	default:
		resp, err = in.Client.Execute(ctx, in.Req, in.Resolver, in.Options)
	}

	res.Response = resp
	if err != nil {
		res.Err = err
		res.Decision = httpFailureDecision(in.Req)
		return res
	}
	return r.finalizeHTTP(ctx, in, resp)
}

func (r Runner) finalizeHTTP(
	ctx context.Context,
	in HTTPInput,
	resp *httpx.Response,
) HTTPResult {
	res := HTTPResult{
		Response: resp,
		Decision: decisionSent,
		Repeat:   RepeatCount{Attempts: 1, Polls: 1},
	}
	streamInfo, streamErr := streamInfoFromResponse(in.Req, resp)
	if streamErr != nil {
		res.Err = diag.WrapAs(diag.ClassProtocol, streamErr, "decode stream transcript")
		res.Decision = "Stream decoding failed"
		return res
	}
	res.Stream = streamInfo

	respForScripts := httpScriptResponse(resp)
	if r.Hooks.ApplyCaptures != nil {
		err := r.Hooks.ApplyCaptures(CaptureInput{
			Doc:      in.Doc,
			Req:      in.Req,
			Resolver: in.Resolver,
			Response: respForScripts,
			Stream:   streamInfo,
			Vars:     r.collectVars(in.Doc, in.Req, in.ScriptVars),
			Locals:   in.Locals,
		})
		if err != nil {
			res.Err = err
			res.Decision = "Capture evaluation failed"
			res.ErrStage = StageCaptures
			return res
		}
	}

	testVars := r.collectVars(in.Doc, in.Req, in.ScriptVars)
	testGlobals := r.collectGlobals(in.Doc)

	var assertErr error
	if r.Hooks.RunAsserts != nil {
		res.Tests, assertErr = r.Hooks.RunAsserts(AssertInput{
			Context: ctx,
			Doc:     in.Doc,
			Req:     in.Req,
			BaseDir: in.Options.BaseDir,
			Vars:    testVars,
			Locals:  in.Locals,
			HTTP:    resp,
			Stream:  streamInfo,
		})
	}

	var traceSpec *restfile.TraceSpec
	if in.Req != nil {
		traceSpec = in.Req.Metadata.Trace
	}
	traceInput := scripts.NewTraceInput(resp.Timeline, traceSpec)
	tests, globalChanges, testErr := in.Scripts.RunTests(
		in.Context,
		in.Req.Metadata.Scripts,
		scripts.TestInput{
			Response:  respForScripts,
			Variables: testVars,
			Globals:   testGlobals,
			BaseDir:   in.Options.BaseDir,
			Stream:    streamInfo,
			Trace:     traceInput,
			Secrets:   in.Secrets,
		},
	)
	if globalChanges.Len() > 0 && r.Hooks.ApplyRuntimeGlobals != nil {
		r.Hooks.ApplyRuntimeGlobals(globalChanges)
	}

	res.Tests = append(res.Tests, tests...)
	res.ScriptErr = joinErr(assertErr, testErr)
	return res
}

func (r Runner) collectVars(
	doc *restfile.Document,
	req *restfile.Request,
	scriptVars vars.NameMap[string],
) map[string]string {
	if r.Hooks.CollectVariables == nil {
		return nil
	}
	return r.Hooks.CollectVariables(doc, req, scriptVars)
}

func (r Runner) collectGlobals(doc *restfile.Document) vars.Globals {
	if r.Hooks.CollectGlobalValues == nil {
		return vars.Globals{}
	}
	return r.Hooks.CollectGlobalValues(doc)
}

func httpFailureDecision(req *restfile.Request) string {
	switch {
	case req == nil:
		return "HTTP request failed"
	case req.WebSocket != nil:
		return "WebSocket request failed"
	case req.SSE != nil:
		return "SSE request failed"
	default:
		return "HTTP request failed"
	}
}

func httpScriptResponse(resp *httpx.Response) *scripts.Response {
	if resp == nil {
		return nil
	}
	return &scripts.Response{
		Kind:   scripts.ResponseKindHTTP,
		Status: resp.Status,
		Code:   resp.StatusCode,
		URL:    resp.EffectiveURL,
		Time:   resp.Duration,
		Header: cloneHeader(resp.Headers),
		Body:   append([]byte(nil), resp.Body...),
	}
}

func streamInfoFromResponse(
	req *restfile.Request,
	resp *httpx.Response,
) (*scripts.StreamInfo, error) {
	if req == nil || resp == nil {
		return nil, nil
	}
	streamType := strings.ToLower(resp.Headers.Get(httpx.StreamHeaderType))
	if req.SSE != nil && streamType == "sse" {
		transcript, err := httpx.DecodeSSETranscript(resp.Body)
		if err != nil {
			return nil, err
		}
		return convertSSETranscript(transcript), nil
	}
	if req.WebSocket != nil && streamType == "websocket" {
		transcript, err := httpx.DecodeWebSocketTranscript(resp.Body)
		if err != nil {
			return nil, err
		}
		return convertWebSocketTranscript(transcript), nil
	}
	return nil, nil
}

func convertSSETranscript(t *httpx.SSETranscript) *scripts.StreamInfo {
	if t == nil {
		return nil
	}
	info := &scripts.StreamInfo{Kind: "sse", Err: t.Summary.Err()}
	info.Summary = map[string]any{
		"eventCount": t.Summary.EventCount,
		"byteCount":  t.Summary.ByteCount,
		"duration":   t.Summary.Duration,
		"reason":     t.Summary.Reason,
		"dropped":    t.Summary.Dropped,
		"error":      t.Summary.Error,
	}
	if len(t.Events) > 0 {
		events := make([]map[string]any, len(t.Events))
		for i, evt := range t.Events {
			events[i] = map[string]any{
				"index":     evt.Index,
				"id":        evt.ID,
				"event":     evt.Event,
				"data":      evt.Data,
				"comment":   evt.Comment,
				"retry":     evt.Retry,
				"timestamp": evt.Timestamp.Format(time.RFC3339Nano),
			}
		}
		info.Events = events
	}
	return info
}

func convertWebSocketTranscript(t *httpx.WebSocketTranscript) *scripts.StreamInfo {
	if t == nil {
		return nil
	}
	info := &scripts.StreamInfo{Kind: "websocket", Err: t.Summary.Err()}
	info.Summary = map[string]any{
		"sentCount":     t.Summary.SentCount,
		"receivedCount": t.Summary.ReceivedCount,
		"duration":      t.Summary.Duration,
		"closedBy":      t.Summary.ClosedBy,
		"closeCode":     t.Summary.CloseCode,
		"closeReason":   t.Summary.CloseReason,
		"dropped":       t.Summary.Dropped,
	}
	if len(t.Events) > 0 {
		events := make([]map[string]any, len(t.Events))
		for i, evt := range t.Events {
			events[i] = map[string]any{
				"step":      evt.Step,
				"direction": evt.Direction,
				"type":      evt.Type,
				"size":      evt.Size,
				"text":      evt.Text,
				"base64":    evt.Base64,
				"code":      evt.Code,
				"reason":    evt.Reason,
				"timestamp": evt.Timestamp.Format(time.RFC3339Nano),
			}
		}
		info.Events = events
	}
	return info
}

func cloneHeader(src map[string][]string) map[string][]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string][]string, len(src))
	for key, values := range src {
		dst[key] = append([]string(nil), values...)
	}
	return dst
}

func joinErr(a, b error) error {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return fmt.Errorf("%v; %v", a, b)
}

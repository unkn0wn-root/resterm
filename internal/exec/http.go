package exec

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/prerequest"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

const (
	StageCaptures = "captures"

	streamHeaderType = "X-Resterm-Stream-Type"
)

type HTTPInput struct {
	Client           *httpclient.Client
	Scripts          *scripts.Runner
	Context          context.Context
	Doc              *restfile.Document
	Req              *restfile.Request
	Resolver         *vars.Resolver
	Options          httpclient.Options
	EnvName          string
	EffectiveTimeout time.Duration
	ScriptVars       map[string]string
	ExtraVals        map[string]rts.Value
}

type CaptureInput struct {
	Doc       *restfile.Document
	Req       *restfile.Request
	Resolver  *vars.Resolver
	Response  *scripts.Response
	Stream    *scripts.StreamInfo
	EnvName   string
	Vars      map[string]string
	ExtraVals map[string]rts.Value
}

type AssertInput struct {
	Context   context.Context
	Doc       *restfile.Document
	Req       *restfile.Request
	EnvName   string
	BaseDir   string
	Vars      map[string]string
	ExtraVals map[string]rts.Value
	HTTP      *httpclient.Response
	Stream    *scripts.StreamInfo
}

type HTTPHooks struct {
	AttachSSEHandle       func(*httpclient.StreamHandle, *restfile.Request)
	AttachWebSocketHandle func(*httpclient.WebSocketHandle, *restfile.Request)
	ApplyCaptures         func(CaptureInput) error
	CollectVariables      func(*restfile.Document, *restfile.Request, string) map[string]string
	CollectGlobalValues   func(*restfile.Document, string) map[string]prerequest.GlobalValue
	RunAsserts            func(AssertInput) ([]scripts.TestResult, error)
	ApplyRuntimeGlobals   func(map[string]prerequest.GlobalValue)
}

type HTTPResult struct {
	Response  *httpclient.Response
	Stream    *scripts.StreamInfo
	Tests     []scripts.TestResult
	ScriptErr error
	Err       error
	Decision  string
	ErrStage  string
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
	ctx, cancel := context.WithTimeout(ctx, in.EffectiveTimeout)
	defer cancel()

	res := HTTPResult{Decision: "HTTP request sent"}

	var (
		resp *httpclient.Response
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
			resp, err = httpclient.CompleteSSE(handle)
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
			Doc:       in.Doc,
			Req:       in.Req,
			Resolver:  in.Resolver,
			Response:  respForScripts,
			Stream:    streamInfo,
			EnvName:   in.EnvName,
			Vars:      r.captureVars(in.Doc, in.Req, in.EnvName, in.ScriptVars),
			ExtraVals: in.ExtraVals,
		})
		if err != nil {
			res.Err = err
			res.Decision = "Capture evaluation failed"
			res.ErrStage = StageCaptures
			return res
		}
	}

	updatedVars := r.collectVars(in.Doc, in.Req, in.EnvName)
	testVars := mergeStringMaps(updatedVars, in.ScriptVars)
	testGlobals := r.collectGlobals(in.Doc, in.EnvName)

	var assertErr error
	if r.Hooks.RunAsserts != nil {
		res.Tests, assertErr = r.Hooks.RunAsserts(AssertInput{
			Context:   ctx,
			Doc:       in.Doc,
			Req:       in.Req,
			EnvName:   in.EnvName,
			BaseDir:   in.Options.BaseDir,
			Vars:      testVars,
			ExtraVals: in.ExtraVals,
			HTTP:      resp,
			Stream:    streamInfo,
		})
	}

	var traceSpec *restfile.TraceSpec
	if in.Req != nil {
		traceSpec = in.Req.Metadata.Trace
	}
	traceInput := scripts.NewTraceInput(resp.Timeline, traceSpec)
	tests, globalChanges, testErr := in.Scripts.RunTests(
		in.Req.Metadata.Scripts,
		scripts.TestInput{
			Response:  respForScripts,
			Variables: testVars,
			Globals:   testGlobals,
			BaseDir:   in.Options.BaseDir,
			Stream:    streamInfo,
			Trace:     traceInput,
		},
	)
	if len(globalChanges) > 0 && r.Hooks.ApplyRuntimeGlobals != nil {
		r.Hooks.ApplyRuntimeGlobals(globalChanges)
	}

	res.Tests = append(res.Tests, tests...)
	res.ScriptErr = joinErr(assertErr, testErr)
	return res
}

func (r Runner) captureVars(
	doc *restfile.Document,
	req *restfile.Request,
	envName string,
	scriptVars map[string]string,
) map[string]string {
	return mergeStringMaps(r.collectVars(doc, req, envName), scriptVars)
}

func (r Runner) collectVars(
	doc *restfile.Document,
	req *restfile.Request,
	envName string,
) map[string]string {
	if r.Hooks.CollectVariables == nil {
		return nil
	}
	return r.Hooks.CollectVariables(doc, req, envName)
}

func (r Runner) collectGlobals(
	doc *restfile.Document,
	envName string,
) map[string]prerequest.GlobalValue {
	if r.Hooks.CollectGlobalValues == nil {
		return nil
	}
	return r.Hooks.CollectGlobalValues(doc, envName)
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

func httpScriptResponse(resp *httpclient.Response) *scripts.Response {
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
	resp *httpclient.Response,
) (*scripts.StreamInfo, error) {
	if req == nil || resp == nil {
		return nil, nil
	}
	streamType := strings.ToLower(resp.Headers.Get(streamHeaderType))
	if req.SSE != nil && streamType == "sse" {
		transcript, err := httpclient.DecodeSSETranscript(resp.Body)
		if err != nil {
			return nil, err
		}
		return convertSSETranscript(transcript), nil
	}
	if req.WebSocket != nil && streamType == "websocket" {
		transcript, err := httpclient.DecodeWebSocketTranscript(resp.Body)
		if err != nil {
			return nil, err
		}
		return convertWebSocketTranscript(transcript), nil
	}
	return nil, nil
}

func convertSSETranscript(t *httpclient.SSETranscript) *scripts.StreamInfo {
	if t == nil {
		return nil
	}
	info := &scripts.StreamInfo{Kind: "sse"}
	info.Summary = map[string]interface{}{
		"eventCount": t.Summary.EventCount,
		"byteCount":  t.Summary.ByteCount,
		"duration":   t.Summary.Duration,
		"reason":     t.Summary.Reason,
	}
	if len(t.Events) > 0 {
		events := make([]map[string]interface{}, len(t.Events))
		for i, evt := range t.Events {
			events[i] = map[string]interface{}{
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

func convertWebSocketTranscript(t *httpclient.WebSocketTranscript) *scripts.StreamInfo {
	if t == nil {
		return nil
	}
	info := &scripts.StreamInfo{Kind: "websocket"}
	info.Summary = map[string]interface{}{
		"sentCount":     t.Summary.SentCount,
		"receivedCount": t.Summary.ReceivedCount,
		"duration":      t.Summary.Duration,
		"closedBy":      t.Summary.ClosedBy,
		"closeCode":     t.Summary.CloseCode,
		"closeReason":   t.Summary.CloseReason,
	}
	if len(t.Events) > 0 {
		events := make([]map[string]interface{}, len(t.Events))
		for i, evt := range t.Events {
			events[i] = map[string]interface{}{
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

func mergeStringMaps(base map[string]string, extra map[string]string) map[string]string {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	out := make(map[string]string, len(base)+len(extra))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range extra {
		out[key] = value
	}
	return out
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

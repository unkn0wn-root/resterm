package httpx

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type metaDefaults struct {
	status string
	code   int
	proto  string
}

// errStreamLimit identifies an SSE duration limit. It must not be used for
// timeouts returned directly to callers because it replaces the timeout cause.
var errStreamLimit = errors.New("stream duration limit")

// A nil cause keeps context.DeadlineExceeded, matching context.WithTimeout.
func ctxWithTimeout(
	ctx context.Context,
	d time.Duration,
	cause error,
) (context.Context, context.CancelFunc) {
	if d > 0 {
		return context.WithTimeoutCause(ctx, d, cause)
	}
	return context.WithCancel(ctx)
}

func buildStreamMeta(
	req *restfile.Request,
	httpReq *http.Request,
	httpResp *http.Response,
	baseDir string,
	def metaDefaults,
) StreamMeta {
	meta := StreamMeta{
		Status:       def.status,
		StatusCode:   def.code,
		Proto:        def.proto,
		EffectiveURL: effURL(httpReq, httpResp),
		ConnectedAt:  time.Now(),
		Request:      req,
		BaseDir:      baseDir,
	}

	reqMeta := captureReqMeta(httpReq, httpResp)
	meta.RequestHeaders = reqMeta.headers
	meta.RequestMethod = reqMeta.method
	meta.RequestHost = reqMeta.host
	meta.RequestLength = reqMeta.length
	meta.RequestTE = reqMeta.te

	if httpResp != nil {
		meta.Status = httpResp.Status
		meta.StatusCode = httpResp.StatusCode
		meta.Proto = httpResp.Proto
		meta.Headers = httpResp.Header.Clone()
	}

	return meta
}

func streamResp(meta StreamMeta, headers http.Header, body []byte, dur time.Duration) *Response {
	return &Response{
		Status:         meta.Status,
		StatusCode:     meta.StatusCode,
		Proto:          meta.Proto,
		Headers:        headers,
		ReqMethod:      meta.RequestMethod,
		RequestHeaders: meta.RequestHeaders.Clone(),
		ReqHost:        meta.RequestHost,
		ReqLen:         meta.RequestLength,
		ReqTE:          slices.Clone(meta.RequestTE),
		Body:           body,
		Duration:       dur,
		EffectiveURL:   meta.EffectiveURL,
		Request:        meta.Request,
	}
}

// This temporary response lets the UI show an interactive WebSocket before the
// session closes and its final transcript is available.
func StreamingWebSocketResponse(meta StreamMeta) *Response {
	headers := meta.Headers.Clone()
	if headers == nil {
		headers = make(http.Header)
	}
	headers.Set(StreamHeaderType, "websocket")
	headers.Set(StreamHeaderSummary, "streaming")

	if meta.Status == "" {
		meta.Status = webSocketSwitchingProtocolsStatus
	}
	if meta.StatusCode == 0 {
		meta.StatusCode = http.StatusSwitchingProtocols
	}
	if meta.Proto == "" {
		meta.Proto = "HTTP/1.1"
	}
	return streamResp(meta, headers, nil, 0)
}

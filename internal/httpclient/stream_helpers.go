package httpclient

import (
	"context"
	"net/http"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type metaDefaults struct {
	status string
	code   int
	proto  string
}

func ctxWithTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d > 0 {
		return context.WithTimeout(ctx, d)
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
		meta.Headers = cloneHdr(httpResp.Header)
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
		RequestHeaders: cloneHdr(meta.RequestHeaders),
		ReqHost:        meta.RequestHost,
		ReqLen:         meta.RequestLength,
		ReqTE:          cloneStrs(meta.RequestTE),
		Body:           body,
		Duration:       dur,
		EffectiveURL:   meta.EffectiveURL,
		Request:        meta.Request,
	}
}

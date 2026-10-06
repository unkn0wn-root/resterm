package httpx

import (
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func effURL(req *http.Request, resp *http.Response) string {
	if resp != nil && resp.Request != nil && resp.Request.URL != nil {
		return resp.Request.URL.String()
	}
	if req != nil && req.URL != nil {
		return req.URL.String()
	}
	return ""
}

func respFromHTTP(
	sent *http.Request,
	resp *http.Response,
	req *restfile.Request,
	body []byte,
	dur time.Duration,
) *Response {
	if resp == nil {
		return &Response{
			Body:         body,
			Duration:     dur,
			EffectiveURL: effURL(sent, nil),
			Request:      req,
		}
	}

	meta := captureReqMeta(sent, resp)
	return &Response{
		Status:         resp.Status,
		StatusCode:     resp.StatusCode,
		Proto:          resp.Proto,
		Headers:        resp.Header.Clone(),
		ReqMethod:      meta.method,
		RequestHeaders: meta.headers,
		ReqHost:        meta.host,
		ReqLen:         meta.length,
		ReqTE:          slices.Clone(meta.te),
		Body:           body,
		Duration:       dur,
		EffectiveURL:   effURL(sent, resp),
		Request:        req,
	}
}

func partialResp(
	req *restfile.Request,
	dur time.Duration,
	timeline *nettrace.Timeline,
	report *nettrace.Report,
) *Response {
	return &Response{
		Request:     req,
		Duration:    dur,
		Timeline:    timeline,
		TraceReport: report,
	}
}

// SentHeaders adds the headers net/http writes from request fields, which
// RequestHeaders does not hold.
func (r *Response) SentHeaders() http.Header {
	h := r.RequestHeaders.Clone()
	if h == nil {
		h = make(http.Header)
	}
	if h.Get("Host") == "" && r.ReqHost != "" {
		h.Set("Host", r.ReqHost)
	}
	if h.Get("Transfer-Encoding") == "" && len(r.ReqTE) > 0 {
		h["Transfer-Encoding"] = slices.Clone(r.ReqTE)
	}
	if h.Get("Content-Length") == "" && r.ReqLen > 0 {
		h.Set("Content-Length", strconv.FormatInt(r.ReqLen, 10))
	}
	return h
}

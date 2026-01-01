package httpclient

import (
	"net/http"
	"time"

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

func cloneHdr(h http.Header) http.Header {
	if h == nil {
		return nil
	}
	return h.Clone()
}

func cloneStrs(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
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
		Headers:        cloneHdr(resp.Header),
		ReqMethod:      meta.method,
		RequestHeaders: meta.headers,
		ReqHost:        meta.host,
		ReqLen:         meta.length,
		ReqTE:          cloneStrs(meta.te),
		Body:           body,
		Duration:       dur,
		EffectiveURL:   effURL(sent, resp),
		Request:        req,
	}
}

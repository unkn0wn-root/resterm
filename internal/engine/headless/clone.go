package headless

import (
	"slices"

	"github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

func cloneHTTP(resp *httpclient.Response) *httpclient.Response {
	if resp == nil {
		return nil
	}

	out := *resp
	out.Headers = cloneStringSliceMap(resp.Headers)
	out.RequestHeaders = cloneStringSliceMap(resp.RequestHeaders)
	out.ReqTE = slices.Clone(resp.ReqTE)
	out.Body = copyBytes(resp.Body)
	out.Request = request.CloneRequest(resp.Request)
	out.Timeline = resp.Timeline.Clone()
	out.TraceReport = resp.TraceReport.Clone()
	return &out
}

func cloneGRPC(resp *grpcclient.Response) *grpcclient.Response {
	if resp == nil {
		return nil
	}

	out := *resp
	out.Headers = cloneStringSliceMap(resp.Headers)
	out.Trailers = cloneStringSliceMap(resp.Trailers)
	out.Body = copyBytes(resp.Body)
	out.Wire = copyBytes(resp.Wire)
	return &out
}

func cloneStringSliceMap[M ~map[string][]string](src M) M {
	if len(src) == 0 {
		return nil
	}

	out := make(M, len(src))
	for key, values := range src {
		out[key] = slices.Clone(values)
	}
	return out
}

func cloneStream(info *scripts.StreamInfo) *scripts.StreamInfo {
	return info.Clone()
}

func copyBytes(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	return append([]byte(nil), src...)
}

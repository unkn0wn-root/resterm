package httpclient

import (
	"net/http"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/httpver"
)

func checkHTTPVersion(resp *http.Response, v httpver.Version) error {
	if v != httpver.V2 {
		return nil
	}
	if resp == nil || resp.ProtoMajor != 2 {
		proto := ""
		if resp != nil {
			proto = resp.Proto
		}
		if strings.TrimSpace(proto) == "" {
			proto = "unknown"
		}
		return errdef.New(errdef.CodeHTTP, "expected HTTP/2 response, got %s", proto)
	}
	return nil
}

func checkWebSocketHTTPVersion(v httpver.Version) error {
	switch v {
	case httpver.V10:
		return errdef.New(
			errdef.CodeHTTP,
			"http-version=1.0 is not supported for WebSocket requests",
		)
	case httpver.V2:
		return errdef.New(errdef.CodeHTTP, "http-version=2 is not supported for WebSocket requests")
	default:
		return nil
	}
}

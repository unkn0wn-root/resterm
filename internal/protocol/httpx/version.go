package httpx

import (
	"net/http"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/http/version"
)

func checkHTTPVersion(resp *http.Response, v version.HTTP) error {
	if v != version.V2 {
		return nil
	}
	if resp == nil || resp.ProtoMajor != 2 {
		proto := ""
		if resp != nil {
			proto = resp.Proto
		}
		if proto == "" {
			proto = "unknown"
		}
		return diag.Newf(diag.ClassProtocol, "expected HTTP/2 response, got %s", proto)
	}
	return nil
}

func checkHTTPVersionRequest(req *http.Request, v version.HTTP) error {
	if v != version.V2 {
		return nil
	}
	if req == nil || req.URL == nil {
		return nil
	}
	if strings.EqualFold(req.URL.Scheme, "http") {
		return diag.Newf(
			diag.ClassProtocol,
			"http-version=2 requires https (h2c is not supported)",
		)
	}
	return nil
}

func checkWebSocketHTTPVersion(v version.HTTP) error {
	if v != version.V2 {
		return nil
	}
	return diag.Newf(
		diag.ClassProtocol,
		"http-version=2 is not supported for WebSocket requests",
	)
}

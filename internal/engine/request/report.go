package request

import (
	"bytes"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/unkn0wn-root/resterm/internal/http/coding"
	"github.com/unkn0wn-root/resterm/internal/http/header"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/runx/report"
)

// ReportResponse returns the response of an HTTP or gRPC result for a run
// report, or nil when no response arrived. A gzip body is decoded before it is
// masked and loses its Content-Encoding and Content-Length headers. A body with
// any other content coding, or one that cannot be decoded, is left out.
// Secrets, sensitive headers and cookies are masked before the body is cut to
// runfmt.MaxBodyBytes, so the cut cannot leave part of a secret behind.
func ReportResponse(
	resp *httpx.Response,
	grpc *grpcx.Response,
	caps []restfile.Variable,
	secs []string,
) *runfmt.Response {
	// A transport error leaves an HTTP response with no status. grpc-go records
	// the response content-type in the header metadata, or in the trailer
	// metadata for a Trailers-Only response, so a call with neither got no gRPC
	// response.
	var src http.Header
	var body, data []byte
	var err error
	switch {
	case resp != nil && resp.StatusCode != 0:
		src, body = resp.Headers, resp.Body
		// Every Content-Encoding line counts, so identity listed before gzip
		// cannot pass a coded body through unmasked.
		enc := strings.Join(src.Values("Content-Encoding"), ",")
		data, err = coding.Decode(body, enc, httpx.DefaultMaxResponseBytes)
	case grpc != nil && (len(grpc.Headers) > 0 || len(grpc.Trailers) > 0):
		src = grpc.HeaderMap()
		body, _ = grpc.ViewBody()
	default:
		return nil
	}

	out := &runfmt.Response{Headers: make(http.Header, len(src)), BodySize: int64(len(body))}
	for name, vals := range src {
		mask := header.Sensitive(name) || header.CarriesCookie(name)
		for _, v := range vals {
			if mask {
				v = secretMask
			} else {
				v = RedactSecrets(v, secs)
			}
			out.Headers[name] = append(out.Headers[name], v)
		}
	}
	for _, c := range caps {
		out.Captures = append(out.Captures, runfmt.Capture{
			Scope:  c.Scope.String(),
			Name:   c.Name,
			Value:  RedactSecrets(c.Value, secs),
			Secret: c.Secret,
		})
	}

	// A body left out keeps the headers that explain why. A decoded one drops
	// them, as net/http's Transport does.
	switch {
	case err != nil:
		return out
	case data != nil:
		out.Headers.Del("Content-Encoding")
		out.Headers.Del("Content-Length")
		body = data
	}

	b := []byte(RedactSecrets(string(body), secs))
	if len(b) > runfmt.MaxBodyBytes {
		n := runfmt.MaxBodyBytes
		// Back up to the start of a rune the cut would split. A rune has at
		// most utf8.UTFMax bytes, so a binary body loses at most three.
		for i := n; i > n-utf8.UTFMax; i-- {
			if utf8.RuneStart(b[i]) {
				n = i
				break
			}
		}
		b, out.BodyTruncated = bytes.Clone(b[:n]), true
	}
	out.Body = b
	return out
}

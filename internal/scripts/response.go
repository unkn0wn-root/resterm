package scripts

import (
	"net/http"
	"time"
)

type ResponseKind string

const (
	ResponseKindHTTP ResponseKind = "http"
	ResponseKindGRPC ResponseKind = "grpc"
)

type Response struct {
	Kind   ResponseKind
	Status string
	Code   int
	URL    string
	Time   time.Duration
	Header http.Header
	Body   []byte
}

func (r *Response) Clone() *Response {
	if r == nil {
		return nil
	}

	copyHeaders := func(h http.Header) http.Header {
		if h == nil {
			return nil
		}

		cloned := make(http.Header, len(h))
		for k, values := range h {
			cloned[k] = append([]string(nil), values...)
		}
		return cloned
	}

	clone := *r
	clone.Header = copyHeaders(r.Header)
	clone.Body = append([]byte(nil), r.Body...)
	return &clone
}

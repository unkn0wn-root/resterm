package update

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

type stubTransport struct {
	res map[string]stubResponse
}

type stubResponse struct {
	status int
	body   string
	header http.Header
}

func (s stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r, ok := s.res[req.URL.String()]
	if !ok {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Status:     http.StatusText(http.StatusNotFound),
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	}
	status := r.status
	if status == 0 {
		status = http.StatusOK
	}
	hdr := r.header
	if hdr == nil {
		hdr = make(http.Header)
	}
	body := io.NopCloser(strings.NewReader(r.body))
	return &http.Response{
		StatusCode:    status,
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Body:          body,
		Header:        hdr,
		Request:       req,
		ContentLength: int64(len(r.body)),
	}, nil
}

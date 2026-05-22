package grpcclient

import (
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func newResponse(headerMD, trailerMD metadata.MD, dur time.Duration) *Response {
	return &Response{
		Headers:         copyMetadata(headerMD),
		Trailers:        copyMetadata(trailerMD),
		StatusCode:      codes.OK,
		StatusMessage:   "OK",
		ContentType:     "application/json",
		WireContentType: "application/grpc+proto",
		Duration:        dur,
	}
}

func setResponseStatus(resp *Response, err error) {
	st := status.Convert(err)
	resp.StatusCode = st.Code()
	resp.StatusMessage = st.Message()
}

func ensureContentType(resp *Response) {
	if resp.Headers == nil {
		resp.Headers = make(map[string][]string)
	}
	if len(resp.Headers["Content-Type"]) == 0 && strings.TrimSpace(resp.ContentType) != "" {
		resp.Headers["Content-Type"] = []string{resp.ContentType}
	}
}

func copyMetadata(md metadata.MD) map[string][]string {
	if md == nil {
		return nil
	}

	out := make(map[string][]string, len(md))
	for k, vals := range md {
		out[k] = append([]string(nil), vals...)
	}
	return out
}

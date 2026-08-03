package grpcclient

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	statusTextOK       = "OK"
	defaultContentType = "application/json"
	wireContentType    = "application/grpc+proto"

	trailerHeaderPrefix = "Grpc-Trailer-"
)

type Response struct {
	Message         string
	Body            []byte
	ContentType     string
	Wire            []byte
	WireContentType string
	Headers         map[string][]string
	Trailers        map[string][]string
	StatusCode      codes.Code
	StatusMessage   string
	// StatusDetails contains decoded gRPC status details as JSON.
	StatusDetails []string
	Duration      time.Duration
}

type mdCapture struct {
	header  metadata.MD
	trailer metadata.MD
}

func (m *mdCapture) opts() []grpc.CallOption {
	return []grpc.CallOption{grpc.Header(&m.header), grpc.Trailer(&m.trailer)}
}

func (m *mdCapture) response(start time.Time) *Response {
	return newResponse(m.header, m.trailer, time.Since(start))
}

func newResponse(headerMD, trailerMD metadata.MD, dur time.Duration) *Response {
	return &Response{
		Headers:         cloneMetaMap(headerMD),
		Trailers:        cloneMetaMap(trailerMD),
		StatusCode:      codes.OK,
		StatusMessage:   statusTextOK,
		ContentType:     defaultContentType,
		WireContentType: wireContentType,
		Duration:        dur,
	}
}

func setResponseStatus(resp *Response, err error, cd codec) {
	st := status.Convert(err)
	resp.StatusCode = st.Code()
	resp.StatusMessage = st.Message()
	resp.StatusDetails = statusDetails(st, cd)
}

// statusDetails uses the request descriptors for custom detail types. A detail
// that cannot be decoded is omitted without discarding the response.
func statusDetails(st *status.Status, cd codec) []string {
	if st == nil {
		return nil
	}

	var out []string
	for _, detail := range st.Proto().GetDetails() {
		data, err := cd.marshal(detail)
		if err != nil {
			continue
		}
		out = append(out, string(data))
	}
	return out
}

func cloneMetaMap[M ~map[string][]string](src M) map[string][]string {
	if len(src) == 0 {
		return nil
	}

	out := make(map[string][]string, len(src))
	for key, vals := range src {
		out[key] = slices.Clone(vals)
	}
	return out
}

func (r *Response) Clone() *Response {
	if r == nil {
		return nil
	}

	out := *r
	out.Headers = cloneMetaMap(r.Headers)
	out.Trailers = cloneMetaMap(r.Trailers)
	out.Body = slices.Clone(r.Body)
	out.Wire = slices.Clone(r.Wire)
	out.StatusDetails = slices.Clone(r.StatusDetails)
	return &out
}

// HeaderMap combines headers and trailers without changing their key casing.
// Trailer keys are prefixed with "Grpc-Trailer-". Content-Type is added when
// gRPC omits it from the returned metadata.
func (r *Response) HeaderMap() http.Header {
	if r == nil {
		return nil
	}

	ct := strings.TrimSpace(r.ContentType)
	if len(r.Headers) == 0 && len(r.Trailers) == 0 && ct == "" {
		return nil
	}

	out := make(http.Header, len(r.Headers)+len(r.Trailers)+1)
	for key, vals := range r.Headers {
		out[key] = slices.Clone(vals)
	}
	for key, vals := range r.Trailers {
		out[trailerHeaderPrefix+key] = slices.Clone(vals)
	}
	if ct != "" && !hasContentType(out) {
		out.Set("Content-Type", ct)
	}
	return out
}

func hasContentType(h http.Header) bool {
	for key := range h {
		if strings.EqualFold(key, "content-type") {
			return true
		}
	}
	return false
}

// StatusText formats the gRPC status code and message for display.
func (r *Response) StatusText() string {
	if r == nil {
		return ""
	}

	code := strings.TrimSpace(r.StatusCode.String())
	msg := strings.TrimSpace(r.StatusMessage)
	if code == "" || msg == "" || strings.EqualFold(code, msg) {
		return code
	}
	return code + " (" + msg + ")"
}

// ViewBody returns the JSON response and its content type.
func (r *Response) ViewBody() ([]byte, string) {
	if r == nil {
		return nil, ""
	}

	body := slices.Clone(r.Body)
	if len(body) == 0 && strings.TrimSpace(r.Message) != "" {
		body = []byte(r.Message)
	}
	ct := strings.TrimSpace(r.ContentType)
	if ct == "" && len(body) > 0 {
		ct = defaultContentType
	}
	return body, ct
}

// RawBody returns the protobuf wire form, or the view body when no single wire
// message exists.
func (r *Response) RawBody() ([]byte, string) {
	if r == nil {
		return nil, ""
	}

	view, viewType := r.ViewBody()
	body := slices.Clone(r.Wire)
	ct := strings.TrimSpace(r.WireContentType)
	if len(body) == 0 {
		body = view
	}
	if ct == "" {
		ct = viewType
	}
	return body, ct
}

// DiffSummary names the fields that differ between r and other, for compare runs.
func (r *Response) DiffSummary(other *Response) string {
	if r == nil || other == nil {
		return "unavailable"
	}

	var diff []string
	if other.StatusCode != r.StatusCode {
		diff = append(diff, "status")
	}
	if strings.TrimSpace(other.StatusMessage) != strings.TrimSpace(r.StatusMessage) {
		diff = append(diff, "message")
	}
	base, _ := r.ViewBody()
	target, _ := other.ViewBody()
	if strings.TrimSpace(string(base)) != strings.TrimSpace(string(target)) {
		diff = append(diff, "body")
	}
	if len(diff) == 0 {
		return "match"
	}
	return strings.Join(diff, ", ") + " differ"
}

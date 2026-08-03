package grpcx

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
)

// fullResponse populates every field so Clone coverage fails when Response grows.
func fullResponse() *Response {
	return &Response{
		Message:         `{"id":"a"}`,
		Body:            []byte(`{"id":"a"}`),
		ContentType:     "application/json",
		Wire:            []byte{0x0a, 0x01, 0x61},
		WireContentType: wireContentType,
		Headers:         map[string][]string{"x-req-id": {"a"}},
		Trailers:        map[string][]string{"x-trace": {"b"}},
		StatusCode:      codes.NotFound,
		StatusMessage:   "missing",
		StatusDetails:   []string{`{"reason":"NOT_FOUND"}`},
		Duration:        3 * time.Second,
	}
}

func TestResponseCloneCoversEveryField(t *testing.T) {
	src := fullResponse()

	v := reflect.ValueOf(*src)
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).IsZero() {
			t.Fatalf("fullResponse leaves %s zero, so Clone coverage is untested",
				v.Type().Field(i).Name)
		}
	}

	got := src.Clone()
	if !reflect.DeepEqual(got, src) {
		t.Fatalf("Clone() = %+v, want %+v", got, src)
	}

	src.Body[0] = 'X'
	src.Wire[0] = 'X'
	src.Headers["x-req-id"][0] = "mutated"
	src.Trailers["x-trace"] = append(src.Trailers["x-trace"], "extra")
	src.StatusDetails[0] = "mutated"
	if string(got.Body) != `{"id":"a"}` {
		t.Fatalf("Clone shares Body: %q", got.Body)
	}
	if got.Wire[0] != 0x0a {
		t.Fatalf("Clone shares Wire: %v", got.Wire)
	}
	if got.Headers["x-req-id"][0] != "a" {
		t.Fatalf("Clone shares Headers: %v", got.Headers)
	}
	if len(got.Trailers["x-trace"]) != 1 {
		t.Fatalf("Clone shares Trailers: %v", got.Trailers)
	}
	if got.StatusDetails[0] == "mutated" {
		t.Fatalf("Clone shares StatusDetails: %v", got.StatusDetails)
	}
}

func TestResponseCloneNil(t *testing.T) {
	var resp *Response
	if resp.Clone() != nil {
		t.Fatal("Clone() on nil should stay nil")
	}
}

func TestResponseHeaderMapFoldsTrailers(t *testing.T) {
	hdrKey := "x-req-id"
	trailerKey := "x-trace"
	resp := &Response{
		Headers:  map[string][]string{hdrKey: {"a"}},
		Trailers: map[string][]string{trailerKey: {"b", "c"}},
	}

	got := resp.HeaderMap()
	want := http.Header{
		hdrKey:                           {"a"},
		trailerHeaderPrefix + trailerKey: {"b", "c"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("HeaderMap() = %v, want %v", got, want)
	}

	resp.Headers[hdrKey][0] = "mutated"
	if got[hdrKey][0] != "a" {
		t.Fatalf("HeaderMap shares the header slice: %v", got)
	}
}

func TestResponseHeaderMapSynthesizesContentType(t *testing.T) {
	resp := &Response{
		Headers:     map[string][]string{"x-req-id": {"a"}},
		ContentType: defaultContentType,
	}
	if got := resp.HeaderMap().Get("Content-Type"); got != defaultContentType {
		t.Fatalf("Content-Type = %q, want %q", got, defaultContentType)
	}

	if _, ok := resp.Headers["Content-Type"]; ok {
		t.Fatal("HeaderMap wrote Content-Type back into Headers")
	}

	wireKey := "content-type"
	served := &Response{
		Headers:     map[string][]string{wireKey: {"application/grpc"}},
		ContentType: defaultContentType,
	}
	if got := served.HeaderMap()[wireKey]; len(got) != 1 || got[0] != "application/grpc" {
		t.Fatalf("content-type = %v, want the served value kept", got)
	}
}

func TestResponseHeaderMapEmpty(t *testing.T) {
	if got := (&Response{}).HeaderMap(); got != nil {
		t.Fatalf("HeaderMap() = %v, want nil", got)
	}
	var resp *Response
	if got := resp.HeaderMap(); got != nil {
		t.Fatalf("HeaderMap() on nil = %v, want nil", got)
	}
}

func TestResponseStatusText(t *testing.T) {
	tests := []struct {
		name string
		code codes.Code
		msg  string
		want string
	}{
		{"ok drops the echoed message", codes.OK, "OK", "OK"},
		{"ok without a message", codes.OK, "", "OK"},
		{"error keeps the message", codes.NotFound, "missing", "NotFound (missing)"},
		{"case insensitive echo", codes.NotFound, "notfound", "NotFound"},
		{"blank message", codes.Internal, "   ", "Internal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &Response{StatusCode: tt.code, StatusMessage: tt.msg}
			if got := resp.StatusText(); got != tt.want {
				t.Fatalf("StatusText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResponseViewBody(t *testing.T) {
	tests := []struct {
		name     string
		resp     *Response
		wantBody string
		wantType string
	}{
		{
			name:     "body wins",
			resp:     &Response{Body: []byte("a"), Message: "b", ContentType: "text/plain"},
			wantBody: "a",
			wantType: "text/plain",
		},
		{
			name:     "falls back to message",
			resp:     &Response{Message: "b"},
			wantBody: "b",
			wantType: defaultContentType,
		},
		{
			name:     "empty stays empty",
			resp:     &Response{},
			wantBody: "",
			wantType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, ct := tt.resp.ViewBody()
			if string(body) != tt.wantBody || ct != tt.wantType {
				t.Fatalf("ViewBody() = %q, %q, want %q, %q", body, ct, tt.wantBody, tt.wantType)
			}
		})
	}
}

func TestResponseRawBodyFallsBackToView(t *testing.T) {
	withWire := &Response{
		Body:            []byte(`{"a":1}`),
		ContentType:     defaultContentType,
		Wire:            []byte{0x08, 0x01},
		WireContentType: wireContentType,
	}
	body, ct := withWire.RawBody()
	if string(body) != "\x08\x01" || ct != wireContentType {
		t.Fatalf("RawBody() = %q, %q, want the wire form", body, ct)
	}

	noWire := &Response{Body: []byte(`[]`), ContentType: defaultContentType}
	body, ct = noWire.RawBody()
	if string(body) != "[]" || ct != defaultContentType {
		t.Fatalf("RawBody() = %q, %q, want the view body", body, ct)
	}
}

func TestResponseDiffSummary(t *testing.T) {
	base := &Response{StatusCode: codes.OK, StatusMessage: "OK", Body: []byte(`{"a":1}`)}

	tests := []struct {
		name  string
		other *Response
		want  string
	}{
		{"identical", base.Clone(), "match"},
		{"nil", nil, "unavailable"},
		{
			name:  "status only",
			other: &Response{StatusCode: codes.NotFound, StatusMessage: "OK", Body: base.Body},
			want:  "status differ",
		},
		{
			name:  "body only",
			other: &Response{StatusCode: codes.OK, StatusMessage: "OK", Body: []byte(`{"a":2}`)},
			want:  "body differ",
		},
		{
			name:  "everything",
			other: &Response{StatusCode: codes.Internal, StatusMessage: "boom"},
			want:  "status, message, body differ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := base.DiffSummary(tt.other); got != tt.want {
				t.Fatalf("DiffSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResponseDiffSummaryMatchesAcrossBodyAndMessage(t *testing.T) {
	base := &Response{StatusCode: codes.OK, StatusMessage: "OK", Body: []byte(`{"a":1}`)}
	other := &Response{StatusCode: codes.OK, StatusMessage: "OK", Message: `{"a":1}`}
	if got := base.DiffSummary(other); got != "match" {
		t.Fatalf("DiffSummary() = %q, want %q", got, "match")
	}
}

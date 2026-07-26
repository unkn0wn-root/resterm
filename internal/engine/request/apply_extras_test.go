package request

import (
	"io"
	"net/http"
	"strings"
	"testing"

	engcfg "github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

// @for-each binds its item as a typed value in ExecOptions.Values. Every other
// expression in the run reads it, and @apply used to be handed nil instead, so a
// patch dereferencing the loop item failed with an undefined name.
func TestApplySeesTypedForEachValues(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{
			name: "member of a dict item",
			expr: `{headers: {"X-Item": item.id}}`,
			want: "42",
		},
		{
			name: "element of a list item",
			expr: `{headers: {"X-Item": list[1]}}`,
			want: "second",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sent *http.Request
			client := httpclient.NewClientWithOptions(
				httpclient.WithHTTPFactory(func(httpclient.Options) (*http.Client, error) {
					return &http.Client{
						Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
							sent = r
							return &http.Response{
								Status:     "200 OK",
								StatusCode: http.StatusOK,
								Header:     make(http.Header),
								Body:       io.NopCloser(strings.NewReader("ok")),
								Request:    r,
							}, nil
						}),
					}, nil
				}),
			)

			e := New(engcfg.Config{Client: client}, nil)
			req := &restfile.Request{
				Method: http.MethodGet,
				URL:    "http://example.test",
				Metadata: restfile.RequestMetadata{
					Applies: []restfile.ApplySpec{{Expression: tt.expr}},
				},
			}

			res, err := e.ExecuteWith(nil, req, "", ExecOptions{
				Values: map[string]rts.Value{
					"item": rts.Dict(map[string]rts.Value{"id": rts.Str("42")}),
					"list": rts.List([]rts.Value{rts.Str("first"), rts.Str("second")}),
				},
			})
			if err != nil {
				t.Fatalf("ExecuteWith: %v", err)
			}
			if res.Err != nil {
				t.Fatalf("ExecuteWith result error: %v", res.Err)
			}
			if sent == nil {
				t.Fatal("request was never sent")
			}
			if got := sent.Header.Get("X-Item"); got != tt.want {
				t.Fatalf("X-Item = %q, want %q", got, tt.want)
			}
		})
	}
}

package request

import (
	"net/http"
	"slices"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestInjectedAuthHeaders(t *testing.T) {
	withHeaders := func(pairs ...string) *restfile.Request {
		h := make(http.Header)
		for i := 0; i < len(pairs); i += 2 {
			h.Set(pairs[i], pairs[i+1])
		}
		return &restfile.Request{Method: "GET", URL: "https://example.com", Headers: h}
	}

	tests := []struct {
		name   string
		before *restfile.Request
		after  *restfile.Request
		want   []string
	}{
		{
			name:   "nothing injected",
			before: withHeaders("Accept", "application/json"),
			after:  withHeaders("Accept", "application/json"),
		},
		{
			name:   "custom command auth header",
			before: withHeaders("Accept", "application/json"),
			after:  withHeaders("Accept", "application/json", "X-Registry-Token", "secret"),
			want:   []string{"X-Registry-Token"},
		},
		{
			name:   "oauth header",
			before: withHeaders(),
			after:  withHeaders("Authorization", "Bearer secret"),
			want:   []string{"Authorization"},
		},
		{
			name:   "value replaced in place",
			before: withHeaders("X-Api-Key", "placeholder"),
			after:  withHeaders("X-Api-Key", "secret"),
			want:   []string{"X-Api-Key"},
		},
		{
			name:  "no snapshot to compare against",
			after: withHeaders("X-Registry-Token", "secret"),
			want:  []string{"X-Registry-Token"},
		},
		{
			name:   "request went away",
			before: withHeaders("X-Api-Key", "secret"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InjectedAuthHeaders(tt.before, tt.after)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("InjectedAuthHeaders() = %v, want %v", got, tt.want)
			}
		})
	}
}

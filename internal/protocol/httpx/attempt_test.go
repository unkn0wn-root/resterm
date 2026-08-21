package httpx

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestAttemptPlanReplaysImmutableRequestAndEvolvingCookies(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New() error = %v", err)
	}
	factoryCalls := 0
	attempts := 0
	var bodies, headers, cookies []string
	client := NewClientWithOptions(WithHTTPFactory(func(Options) (*http.Client, error) {
		factoryCalls++
		return &http.Client{
			Jar: jar,
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				attempts++
				body, readErr := io.ReadAll(req.Body)
				if readErr != nil {
					t.Fatalf("read request body: %v", readErr)
				}
				bodies = append(bodies, string(body))
				headers = append(headers, req.Header.Get("X-Snapshot"))
				cookies = append(cookies, req.Header.Get("Cookie"))
				req.Header.Set("X-Snapshot", "transport mutation")
				responseHeader := make(http.Header)
				if attempts == 1 {
					responseHeader.Set("Set-Cookie", "session=next; Path=/")
				}
				return &http.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Proto:      "HTTP/1.1",
					Header:     responseHeader,
					Body:       io.NopCloser(strings.NewReader("ok")),
					Request:    req,
				}, nil
			}),
		}, nil
	}))
	resolver := vars.NewResolver(vars.NewMapProvider("env", map[string]string{"value": "prepared"}))
	req := &restfile.Request{
		Method:  "POST",
		URL:     "https://example.com/jobs",
		Headers: http.Header{"X-Snapshot": {"{{env.value}}"}},
		Body:    restfile.BodySource{Text: `{"value":"{{env.value}}"}`},
	}
	plan, err := client.PrepareAttempts(t.Context(), req, resolver, Options{CookieJar: jar})
	if err != nil {
		t.Fatalf("PrepareAttempts() error = %v", err)
	}
	req.Headers.Set("X-Snapshot", "source mutation")
	req.Body.Text = "source mutation"

	for range 2 {
		if _, err := plan.Execute(context.Background()); err != nil {
			t.Fatalf("AttemptPlan.Execute() error = %v", err)
		}
	}
	if factoryCalls != 1 {
		t.Fatalf("HTTP client factory calls = %d, want 1", factoryCalls)
	}
	if got, want := strings.Join(bodies, "|"), `{"value":"prepared"}|{"value":"prepared"}`; got != want {
		t.Fatalf("attempt bodies = %q, want %q", got, want)
	}
	if got := strings.Join(headers, "|"); got != "prepared|prepared" {
		t.Fatalf("attempt headers = %q", got)
	}
	if cookies[0] != "" || !strings.Contains(cookies[1], "session=next") {
		t.Fatalf("attempt cookies = %q", cookies)
	}
}

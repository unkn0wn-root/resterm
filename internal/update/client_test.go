package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestClientCheckUpdate(t *testing.T) {
	sum := sha256.Sum256([]byte("bin"))
	body := fmt.Sprintf(
		`{"tag_name":"v1.1.0","body":"notes","published_at":"2024-08-01T00:00:00Z","assets":[{"name":"resterm_Linux_x86_64","browser_download_url":"https://mock/bin","size":12,"digest":"sha256:%x"}]}`,
		sum,
	)
	cl := newTestClient(body)

	plat, err := For("linux", "amd64")
	if err != nil {
		t.Fatalf("platform error: %v", err)
	}

	res, ok, err := cl.Check(context.Background(), "1.0.0", plat)
	if err != nil {
		t.Fatalf("check error: %v", err)
	}
	if !ok {
		t.Fatal("expected update")
	}
	if res.Bin.Name != "resterm_Linux_x86_64" {
		t.Fatalf("unexpected asset: %s", res.Bin.Name)
	}
	if res.Digest != sum {
		t.Fatalf("unexpected digest: %s", hex.EncodeToString(res.Digest[:]))
	}
}

func TestClientCheckNoUpdate(t *testing.T) {
	cl := newTestClient(`{"tag_name":"1.0.0","body":"","published_at":"2024-01-01T00:00:00Z"}`)
	plat, _ := For("linux", "amd64")
	_, ok, err := cl.Check(context.Background(), "1.0.0", plat)
	if err != nil {
		t.Fatalf("check error: %v", err)
	}
	if ok {
		t.Fatal("expected no update")
	}
}

func TestClientCheckDevBuild(t *testing.T) {
	cl := newTestClient(`{"tag_name":"1.1.0","body":"","published_at":"2024-01-01T00:00:00Z"}`)
	plat, _ := For("linux", "amd64")
	if _, _, err := cl.Check(context.Background(), "dev", plat); !errors.Is(err, ErrDevBuild) {
		t.Fatalf("expected ErrDevBuild, got %v", err)
	}
}

func TestClientCheckMissingAsset(t *testing.T) {
	cl := newTestClient(`{"tag_name":"1.1.0","body":"","published_at":"2024-01-01T00:00:00Z"}`)
	plat, _ := For("linux", "amd64")
	if _, _, err := cl.Check(context.Background(), "1.0.0", plat); !errors.Is(err, ErrNoAsset) {
		t.Fatalf("expected ErrNoAsset, got %v", err)
	}
}

func TestClientCheckNoDigest(t *testing.T) {
	body := `{"tag_name":"v1.1.0","body":"","published_at":"2024-01-01T00:00:00Z","assets":[{"name":"resterm_Linux_x86_64","browser_download_url":"https://mock/bin","size":12}]}`
	cl := newTestClient(body)
	plat, _ := For("linux", "amd64")
	if _, _, err := cl.Check(context.Background(), "1.0.0", plat); !errors.Is(err, ErrNoDigest) {
		t.Fatalf("expected ErrNoDigest, got %v", err)
	}
}

func TestClientLatestRateLimited(t *testing.T) {
	limited := make(http.Header)
	limited.Set("X-RateLimit-Remaining", "0")

	cases := []struct {
		res  stubResponse
		want bool
	}{
		{stubResponse{status: http.StatusForbidden, header: limited}, true},
		{stubResponse{status: http.StatusTooManyRequests}, true},
		{stubResponse{status: http.StatusForbidden}, false},
	}
	for _, c := range cases {
		tr := stubTransport{res: map[string]stubResponse{
			"https://api.github.com/repos/unkn0wn-root/resterm/releases/latest": c.res,
		}}
		cl, _ := NewClient(&http.Client{Transport: tr}, "unkn0wn-root/resterm")
		_, err := cl.Latest(context.Background())
		if got := errors.Is(err, ErrRateLimited); got != c.want {
			t.Fatalf("status %d: rate limited = %v, want %v (err: %v)", c.res.status, got, c.want, err)
		}
	}
}

func TestNeedsUpdate(t *testing.T) {
	cases := []struct {
		curr, latest string
		want         bool
	}{
		{"1.0.0", "1.0.1", true},
		{"v1.0.0", "1.0.0", false},
		{"1.1.0", "1.0.9", false},
		{"1.0", "v1.0.1", true},
		{"1.0.0-rc.9", "1.0.0-rc.10", true},
		{"1.0.0-beta", "1.0.0", true},
		{"g1234abc", "1.0.0", true},
	}
	for _, c := range cases {
		got, err := needsUpdate(c.curr, c.latest)
		if err != nil {
			t.Fatalf("needsUpdate(%q, %q): %v", c.curr, c.latest, err)
		}
		if got != c.want {
			t.Fatalf("needsUpdate(%q, %q) = %v, want %v", c.curr, c.latest, got, c.want)
		}
	}

	if _, err := needsUpdate("1.0.0", "not-a-version"); err == nil {
		t.Fatal("expected error for invalid latest version")
	}
}

func newTestClient(body string) Client {
	tr := stubTransport{res: map[string]stubResponse{
		"https://api.github.com/repos/unkn0wn-root/resterm/releases/latest": {body: body},
	}}
	cl, _ := NewClient(&http.Client{Transport: tr}, "unkn0wn-root/resterm")
	return cl
}

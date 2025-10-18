package update

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestClientCheckUpdate(t *testing.T) {
	body := `{"tag_name":"v1.1.0","body":"notes","published_at":"2024-08-01T00:00:00Z","assets":[{"name":"resterm_Linux_x86_64","browser_download_url":"https://mock/bin","size":12},{"name":"resterm_Linux_x86_64.sha256","browser_download_url":"https://mock/sum","size":99}]}`
	cl := newTestClient(body)

	plat, err := For("linux", "amd64")
	if err != nil {
		t.Fatalf("platform error: %v", err)
	}

	res, err := cl.Check(context.Background(), "1.0.0", plat)
	if err != nil {
		t.Fatalf("check error: %v", err)
	}
	if !res.HasSum {
		t.Fatal("expected checksum asset")
	}
	if res.Bin.Name != "resterm_Linux_x86_64" {
		t.Fatalf("unexpected asset: %s", res.Bin.Name)
	}
}

func TestClientCheckNoUpdate(t *testing.T) {
	cl := newTestClient(`{"tag_name":"1.0.0","body":"","published_at":"2024-01-01T00:00:00Z"}`)
	plat, _ := For("linux", "amd64")
	if _, err := cl.Check(context.Background(), "1.0.0", plat); !errors.Is(err, ErrNoUpdate) {
		t.Fatalf("expected ErrNoUpdate, got %v", err)
	}
}

func TestClientCheckMissingAsset(t *testing.T) {
	cl := newTestClient(`{"tag_name":"1.1.0","body":"","published_at":"2024-01-01T00:00:00Z"}`)
	plat, _ := For("linux", "amd64")
	if _, err := cl.Check(context.Background(), "1.0.0", plat); !errors.Is(err, ErrNoAsset) {
		t.Fatalf("expected ErrNoAsset, got %v", err)
	}
}

func newTestClient(body string) Client {
	tr := stubTransport{res: map[string]stubResponse{
		"https://api.github.com/repos/unkn0wn-root/resterm/releases/latest": {body: body},
	}}
	cl, _ := NewClient(&http.Client{Transport: tr}, "unkn0wn-root/resterm")
	return cl
}

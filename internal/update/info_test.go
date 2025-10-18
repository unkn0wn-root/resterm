package update

import (
	"strings"
	"testing"
)

func TestDecodeInfo(t *testing.T) {
	js := `{
        "tag_name": "v1.2.3",
        "body": "notes",
        "published_at": "2024-08-01T12:00:00Z",
        "assets": [
            {"name": "resterm_Linux_x86_64", "browser_download_url": "https://x/bin", "size": 1024},
            {"name": "resterm_Linux_x86_64.sha256", "browser_download_url": "https://x/sum", "size": 99},
            {"name": "", "browser_download_url": ""}
        ]
    }`

	info, err := decodeInfo(strings.NewReader(js))
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if info.Version != "v1.2.3" {
		t.Fatalf("want version v1.2.3, got %s", info.Version)
	}
	if len(info.Assets) != 2 {
		t.Fatalf("want 2 assets, got %d", len(info.Assets))
	}
}

func TestDecodeInfoErrors(t *testing.T) {
	_, err := decodeInfo(strings.NewReader(`{"body": "x"}`))
	if err == nil {
		t.Fatal("expected error for missing tag")
	}
}

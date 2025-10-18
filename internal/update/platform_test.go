package update

import "testing"

func TestPlatformFor(t *testing.T) {
	p, err := For("linux", "amd64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Asset != "resterm_Linux_x86_64" {
		t.Fatalf("unexpected asset: %s", p.Asset)
	}
	if p.Sum != "resterm_Linux_x86_64.sha256" {
		t.Fatalf("unexpected sum: %s", p.Sum)
	}
}

func TestPlatformForWindows(t *testing.T) {
	p, err := For("windows", "arm64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Asset != "resterm_Windows_arm64.exe" {
		t.Fatalf("unexpected windows asset: %s", p.Asset)
	}
	if p.Sum != "resterm_Windows_arm64.exe.sha256" {
		t.Fatalf("unexpected windows sum: %s", p.Sum)
	}
}

func TestPlatformUnsupported(t *testing.T) {
	if _, err := For("plan9", "amd64"); err == nil {
		t.Fatal("expected error for unknown os")
	}
	if _, err := For("linux", "sparc"); err == nil {
		t.Fatal("expected error for unknown arch")
	}
}

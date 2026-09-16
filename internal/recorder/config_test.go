package recorder

import "testing"

func TestConfigResolve(t *testing.T) {
	const upstream = "https://api.example.com"
	got, err := (Config{Upstream: upstream + "/"}).Resolve()
	if err != nil || got.Upstream != upstream || got.Listen == "" || got.MaxEntries <= 0 ||
		got.BodyLimit <= 0 || got.MaxBytes < got.BodyLimit || got.CaptureConcurrency <= 0 {
		t.Fatalf("defaults: %+v, %v", got, err)
	}
	if _, err := (Config{Upstream: upstream, Skip: []string{"get /health", "/assets/{path...}"}}).Resolve(); err != nil {
		t.Fatal(err)
	}
	for _, cfg := range []Config{
		{Upstream: upstream + "?a=b"},
		{Upstream: upstream + "#fragment"},
		{Upstream: "ftp://api.example.com"},
		{Upstream: "https://{{host}}.example.com"},
		{Upstream: upstream, Listen: "nonsense"},
		{Upstream: upstream, MaxBytes: 1024, BodyLimit: 2048},
		{Upstream: upstream, MaxEntries: -1},
		{Upstream: upstream, RedactFields: []string{" "}},
		{Upstream: upstream, Skip: []string{"health"}},
		{Upstream: upstream, Only: []string{"bad/method /x"}},
		{Upstream: upstream, Skip: []string{"/a/{x}/{x}"}},
	} {
		if _, err := cfg.Resolve(); err == nil {
			t.Fatalf("accepted invalid config: %+v", cfg)
		}
	}
}

package httpclient

import (
	"net/http"
	"testing"
)

func TestNewClientUsesImplicitDefaultFactory(t *testing.T) {
	c := NewClient(nil)
	if c.httpFactory != nil {
		t.Fatal("httpFactory != nil")
	}
	if _, err := c.httpClient(Options{}); err != nil {
		t.Fatalf("httpClient: %v", err)
	}
}

func TestClientCloneKeepsDefaultFactory(t *testing.T) {
	src := NewClient(nil)
	cl := src.Clone()

	if cl == nil {
		t.Fatal("Clone() = nil")
	}
	if cl.httpFactory != nil {
		t.Fatal("clone httpFactory != nil")
	}
	if _, err := cl.httpClient(Options{}); err != nil {
		t.Fatalf("clone httpClient: %v", err)
	}
}

func TestClientCloneSnapshotsCustomFactory(t *testing.T) {
	used := false
	src := NewClientWithOptions(WithHTTPFactory(func(Options) (*http.Client, error) {
		used = true
		return &http.Client{}, nil
	}))

	cl := src.Clone()

	if _, err := cl.httpClient(Options{}); err != nil {
		t.Fatalf("clone httpClient: %v", err)
	}
	if !used {
		t.Fatal("clone did not use built factory snapshot")
	}
}

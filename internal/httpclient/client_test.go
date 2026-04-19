package httpclient

import (
	"errors"
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

func TestClientCloneKeepsDefaultFactoryIndependent(t *testing.T) {
	src := NewClient(nil)
	cl := src.Clone()

	want := errors.New("mutated")
	src.SetHTTPFactory(func(Options) (*http.Client, error) {
		return nil, want
	})

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
	src := NewClient(nil)

	used := false
	src.SetHTTPFactory(func(Options) (*http.Client, error) {
		used = true
		return &http.Client{}, nil
	})

	cl := src.Clone()

	want := errors.New("mutated")
	src.SetHTTPFactory(func(Options) (*http.Client, error) {
		return nil, want
	})

	if _, err := cl.httpClient(Options{}); err != nil {
		t.Fatalf("clone httpClient: %v", err)
	}
	if !used {
		t.Fatal("clone did not use built factory snapshot")
	}
}

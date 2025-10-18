package main

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/update"
)

func TestCLIUpdaterCheckDev(t *testing.T) {
	cl, err := update.NewClient(&http.Client{}, updateRepo)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	u := newCLIUpdater(cl, "dev")
	if _, _, err := u.check(context.Background()); !errors.Is(err, errUpdateDisabled) {
		t.Fatalf("expected errUpdateDisabled, got %v", err)
	}
}

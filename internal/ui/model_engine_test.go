package ui

import (
	"testing"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
)

func TestRunCfgAllowsInteractiveOAuth(t *testing.T) {
	model := New(Config{})

	cfg := model.runCfg(httpclient.Options{})
	if !cfg.AllowInteractiveOAuth {
		t.Fatalf("expected UI request engine config to allow interactive OAuth")
	}
}

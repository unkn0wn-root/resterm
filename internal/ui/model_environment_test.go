package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestEnvironmentSelectorRendersItems(t *testing.T) {
	cat := testCatalog(vars.EnvironmentSet{
		"dev":  {"baseUrl": "https://dev"},
		"prod": {"baseUrl": "https://prod"},
	})
	cfg := Config{Catalog: cat, Selection: cat.DefaultSelection()}

	model := New(cfg)
	model.ready = true
	model.width = 80
	model.height = 24
	model.frameWidth = 80
	model.frameHeight = 24
	model.applyLayout()

	model.openEnvironmentSelector()
	view := model.View()

	if !containsSubstring(view, "dev") || !containsSubstring(view, "prod") {
		t.Fatalf("environment selector should list environments, got view:\n%s", view)
	}
}

func TestApplyEnvironmentSelectionResetsLatency(t *testing.T) {
	cat := testCatalog(vars.EnvironmentSet{
		"dev":  {"baseUrl": "https://dev"},
		"prod": {"baseUrl": "https://prod"},
	})
	cfg := Config{Catalog: cat, Selection: cat.DefaultSelection()}

	model := New(cfg)
	model.latencySeries.add(120 * time.Millisecond)
	model.openEnvironmentSelector()
	for i, item := range model.envList.Items() {
		if env, ok := item.(envItem); ok && env.name == "prod" {
			model.envList.Select(i)
		}
	}

	model.applyEnvironmentSelection()
	if _, ok := model.latencySeries.summary(); ok {
		t.Fatal("expected latency series reset on environment switch")
	}
}

func TestGroupedEnvironmentSelectorChangesOneGroup(t *testing.T) {
	cat, err := vars.NewGroupedCatalog(nil, []vars.Group{
		{
			Name:    "api",
			Default: "dev",
			Profiles: vars.EnvironmentSet{
				"dev":  {"api.url": "dev"},
				"prod": {"api.url": "prod"},
			},
		},
		{
			Name:    "app",
			Default: "dev app 1",
			Profiles: vars.EnvironmentSet{
				"dev app 1": {"app.url": "one"},
				"dev app 2": {"app.url": "two"},
			},
		},
	})
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	model := New(Config{Catalog: cat, Selection: cat.DefaultSelection()})
	model.latencySeries.add(50 * time.Millisecond)
	model.openEnvironmentSelector()

	if got, want := model.envList.Title, "Environments · api=dev, app=dev app 1"; got != want {
		t.Fatalf("title = %q, want %q", got, want)
	}
	active := 0
	for i, item := range model.envList.Items() {
		env, ok := item.(envItem)
		if !ok {
			continue
		}
		if env.active {
			active++
		}
		if env.group == "app" && env.profile == "dev app 2" {
			model.envList.Select(i)
		}
	}
	if active != 2 {
		t.Fatalf("active rows = %d, want one per group", active)
	}

	model.applyEnvironmentSelection()
	env, err := cat.Resolve(model.cfg.Selection)
	if err != nil {
		t.Fatalf("resolve selection: %v", err)
	}
	if got, want := env.Label(), "api=dev, app=dev app 2"; got != want {
		t.Fatalf("selection = %q, want %q", got, want)
	}
	if got := env.Selection().Groups()["api"]; got != "dev" {
		t.Fatalf("unselected api profile changed to %q", got)
	}
	if !strings.Contains(model.statusMessage.text, env.Label()) {
		t.Fatalf("status = %q, want full selection", model.statusMessage.text)
	}
	if _, ok := model.latencySeries.summary(); ok {
		t.Fatal("latency series was not reset")
	}
	active = 0
	for _, item := range model.envList.Items() {
		if env, ok := item.(envItem); ok && env.active {
			active++
		}
	}
	if active != 2 {
		t.Fatalf("active rows after switch = %d, want one per group", active)
	}
}

func containsSubstring(view, substr string) bool {
	return strings.Contains(view, substr)
}

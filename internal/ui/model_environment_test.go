package ui

import (
	"strings"
	"testing"
)

func TestEnvironmentSelectorRendersItems(t *testing.T) {
	cfg := Config{
		EnvironmentSet: map[string]map[string]string{
			"dev":  {"baseUrl": "https://dev"},
			"prod": {"baseUrl": "https://prod"},
		},
		EnvironmentName: "dev",
	}

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

func containsSubstring(view, substr string) bool {
	return strings.Contains(view, substr)
}

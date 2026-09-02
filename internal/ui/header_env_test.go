package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// Every variant keeps the selected profile: a header that reads the same on dev
// and on prod has dropped the only part worth glancing at.
func TestHeaderEnvVariantsSummarisesGroups(t *testing.T) {
	cat := groupedEnvironmentCatalog(t)
	model := New(Config{Env: vars.Config{Catalog: cat, Selection: cat.DefaultSelection()}})

	got := model.headerEnvVariants()
	want := []string{"api=dev +1", "dev +1"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("variants = %q, want %q", got, want)
	}
	for _, variant := range got {
		if !strings.Contains(variant, "dev") {
			t.Fatalf("variant %q does not name the selected profile", variant)
		}
	}
}

func TestHeaderEnvVariantsSingleGroupOmitsCount(t *testing.T) {
	cat, err := vars.NewGroupedCatalog(nil, []vars.Group{{
		Name:     "api",
		Default:  "prod",
		Profiles: vars.EnvironmentSet{"prod": {"url": "https://prod"}},
	}})
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	model := New(Config{Env: vars.Config{Catalog: cat, Selection: cat.DefaultSelection()}})

	got := model.headerEnvVariants()
	want := []string{"api=prod", "prod"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("variants = %q, want %q", got, want)
	}
}

func TestHeaderEnvVariantsFlatCatalogUsesLabel(t *testing.T) {
	cat := testCatalog(vars.EnvironmentSet{"dev": {"url": "https://dev"}})
	model := New(Config{Env: vars.Config{Catalog: cat, Selection: cat.DefaultSelection()}})

	if got := model.headerEnvVariants(); len(got) != 1 || got[0] != "dev" {
		t.Fatalf("variants = %q, want [dev]", got)
	}
}

// The header names the first group and counts the rest. Spelling the selection
// out is what Ctrl+E and the status bar are for.
func TestHeaderShowsCompactGroupedEnvironment(t *testing.T) {
	cat, err := vars.NewGroupedCatalog(nil, []vars.Group{
		{Name: "api", Default: "prod", Profiles: vars.EnvironmentSet{"prod": {"api.url": "https://prod"}}},
		{Name: "app", Default: "ci", Profiles: vars.EnvironmentSet{"ci": {"app.url": "https://ci"}}},
		{Name: "region", Default: "eu", Profiles: vars.EnvironmentSet{"eu": {"region.id": "eu"}}},
	})
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}

	model := New(Config{Env: vars.Config{Catalog: cat, Selection: cat.DefaultSelection()}})
	model.ready = true
	model.width = 200
	model.height = 30

	view := ansi.Strip(model.renderHeader())
	if !strings.Contains(view, labelHeaderEnv+" api=prod +2") {
		t.Fatalf("header does not summarise the selection:\n%s", view)
	}
	for _, unwanted := range []string{"app=ci", "region=eu"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("header spelled out %q:\n%s", unwanted, view)
		}
	}
}

func TestHeaderKeepsActiveAndTestsWithGroupedEnvironment(t *testing.T) {
	cat, err := vars.NewGroupedCatalog(nil, []vars.Group{
		{
			Name:     "credentials",
			Default:  "developer account",
			Profiles: vars.EnvironmentSet{"developer account": {"token": "secret"}},
		},
		{
			Name:     "application",
			Default:  "quality assurance",
			Profiles: vars.EnvironmentSet{"quality assurance": {"url": "https://qa"}},
		},
		{
			Name:     "region",
			Default:  "europe west",
			Profiles: vars.EnvironmentSet{"europe west": {"region": "eu"}},
		},
	})
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}

	model := New(Config{Env: vars.Config{Catalog: cat, Selection: cat.DefaultSelection()}})
	model.ready = true
	model.width = 120
	model.height = 30
	model.activeRequestTitle = "GET users"
	model.testResults = []scripts.TestResult{{Name: "ok", Passed: true}}

	view := ansi.Strip(model.renderHeader())
	for _, want := range []string{labelHeaderEnv, iconHeaderActive, "GET users", iconTestPass} {
		if !strings.Contains(view, want) {
			t.Fatalf("header dropped %q:\n%s", want, view)
		}
	}
	if got := lipgloss.Width(view); got > model.width {
		t.Fatalf("header width = %d, want <= %d:\n%s", got, model.width, view)
	}
}

func TestHeaderCapsSingleValue(t *testing.T) {
	cat := testCatalog(vars.EnvironmentSet{"dev": {"url": "https://dev"}})
	model := New(Config{Env: vars.Config{Catalog: cat, Selection: cat.DefaultSelection()}})
	model.ready = true
	model.width = 100
	model.height = 30
	model.activeRequestTitle = strings.Repeat("long", 30)

	view := ansi.Strip(model.renderHeader())
	if strings.Contains(view, strings.Repeat("long", 10)) {
		t.Fatalf("uncapped value monopolised the header:\n%s", view)
	}
	for _, want := range []string{labelHeaderEnv, "dev", iconHeaderActive} {
		if !strings.Contains(view, want) {
			t.Fatalf("header dropped %q:\n%s", want, view)
		}
	}
}

func TestFitHeaderSegmentsCompactsBeforeDropping(t *testing.T) {
	segs := []headerSegment{
		{text: []string{"aaaaaaaaaa", "aaa"}, priority: headerPriorityEnv},
		{text: []string{"bbbb"}, priority: headerPriorityActive},
	}

	line, width := fitHeaderSegments(segs, " ", 1, 8)
	if line != "aaa bbbb" {
		t.Fatalf("line = %q, want the compacted variant next to a kept neighbour", line)
	}
	if width != 8 {
		t.Fatalf("width = %d, want 8", width)
	}
}

// Shrinking is per cell: a cell with room to spare keeps its full rendering
// while the one that has to give way steps down.
func TestFitHeaderSegmentsShrinksOnlyWhatItMustCells(t *testing.T) {
	segs := []headerSegment{
		{text: []string{"envenvenv", "env"}, lossy: 2, priority: headerPriorityEnv},
		{text: []string{"activeactive", "active…"}, lossy: 1, priority: headerPriorityActive},
	}

	line, width := fitHeaderSegments(segs, " ", 1, 20)
	if line != "env activeactive" {
		t.Fatalf("line = %q, want only the env cell shortened", line)
	}
	if width != 16 {
		t.Fatalf("width = %d, want 16", width)
	}
}

// A caller's own shorter wording says the same thing in less room, so it is
// spent before any cell starts losing characters.
func TestFitHeaderSegmentsPrefersLosslessOverTruncation(t *testing.T) {
	segs := []headerSegment{
		// One lossless step saving 2, then a truncation saving 4.
		{text: []string{"aaaaaa", "aaaa", "a…"}, lossy: 2, priority: headerPriorityEnv},
		// A truncation straight away, saving more.
		{text: []string{"bbbbbb", "b…"}, lossy: 1, priority: headerPriorityActive},
	}

	line, _ := fitHeaderSegments(segs, " ", 1, 11)
	if line != "aaaa bbbbbb" {
		t.Fatalf("line = %q, want the lossless step taken before any truncation", line)
	}
}

func TestFitHeaderSegmentsDropsByPriorityNotPosition(t *testing.T) {
	segs := []headerSegment{
		{text: []string{"low"}, priority: headerPriorityRequests},
		{text: []string{"mid"}, priority: headerPriorityActive},
		{text: []string{"high"}, priority: headerPriorityEnv},
	}

	line, _ := fitHeaderSegments(segs, " ", 1, 8)
	if line != "mid high" {
		t.Fatalf("line = %q, want the lowest priority dropped and order preserved", line)
	}

	line, _ = fitHeaderSegments(segs, " ", 1, 4)
	if line != "high" {
		t.Fatalf("line = %q, want only the most important segment", line)
	}
}

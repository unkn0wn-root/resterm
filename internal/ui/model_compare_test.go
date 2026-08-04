package ui

import (
	"reflect"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestBuildConfigCompareSpecBaselineFallback(t *testing.T) {
	spec := core.BuildCompareSpec(engine.CompareConfig{Targets: []string{"dev", "stage", "prod"}})
	if spec == nil {
		t.Fatalf("expected spec")
	}
	if spec.Baseline != "dev" {
		t.Fatalf("expected baseline dev, got %s", spec.Baseline)
	}
	expect := []string{"dev", "stage", "prod"}
	if !reflect.DeepEqual(expect, spec.Environments) {
		t.Fatalf("unexpected environments: %#v", spec.Environments)
	}
}

func TestBuildConfigCompareSpecKeepsUnknownBaselineForValidation(t *testing.T) {
	spec := core.BuildCompareSpec(engine.CompareConfig{Targets: []string{"dev", "stage"}, Base: "prod"})
	if spec == nil {
		t.Fatalf("expected spec")
	}
	if spec.Baseline != "prod" {
		t.Fatalf("expected baseline prod, got %s", spec.Baseline)
	}
	expect := []string{"dev", "stage"}
	if !reflect.DeepEqual(expect, spec.Environments) {
		t.Fatalf("unexpected environments: %#v", spec.Environments)
	}
}

func TestCompareSpecForRequestPrefersConfig(t *testing.T) {
	req := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Compare: &restfile.CompareSpec{
				Environments: []string{"file-dev", "file-stage"},
				Baseline:     "file-dev",
			},
		},
	}
	model := Model{
		cfg: Config{
			Compare: engine.CompareConfig{
				Targets: []string{"cli-dev", "cli-stage"},
				Base:    "cli-stage",
			},
		},
	}
	spec := model.compareSpecForRequest(req)
	if spec == nil {
		t.Fatalf("expected spec")
	}
	if spec.Baseline != "cli-stage" {
		t.Fatalf("expected CLI baseline, got %s", spec.Baseline)
	}
	expect := []string{"cli-dev", "cli-stage"}
	if !reflect.DeepEqual(expect, spec.Environments) {
		t.Fatalf("unexpected envs: %#v", spec.Environments)
	}
}

func TestNormalizeCompareTargets(t *testing.T) {
	spec := core.BuildCompareSpec(engine.CompareConfig{Targets: []string{"dev", "DEV", " stage ", ""}})
	if spec == nil {
		t.Fatalf("expected spec")
	}
	expect := []string{"dev", "stage"}
	if !reflect.DeepEqual(expect, spec.Environments) {
		t.Fatalf("unexpected targets: %#v", spec.Environments)
	}
}

// In a grouped compare the baseline names a profile while the visible label is
// the full selection, so the marker has to match on target name, not position.
func TestCompareProgressSummaryMarksGroupedBaseline(t *testing.T) {
	cat, err := vars.NewGroupedCatalog(nil, []vars.Group{
		{
			Name:    "api",
			Default: "dev",
			Profiles: vars.EnvironmentSet{
				"dev":   {"api.url": "d"},
				"stage": {"api.url": "s"},
				"prod":  {"api.url": "p"},
			},
		},
		{
			Name:     "app",
			Default:  "one",
			Profiles: vars.EnvironmentSet{"one": {"app.url": "1"}},
		},
	})
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	base := cat.DefaultSelection()
	targets, err := cat.CompareTargets(base, "api", "stage", []string{"dev", "stage", "prod"})
	if err != nil {
		t.Fatalf("compare targets: %v", err)
	}
	pl, err := core.PrepareCompare(core.CompareInput{
		Doc:      &restfile.Document{Path: "cmp.http"},
		Request:  &restfile.Request{Method: "GET", URL: "https://example.com"},
		Targets:  targets,
		Group:    "api",
		Baseline: "stage",
	})
	if err != nil {
		t.Fatalf("prepare compare: %v", err)
	}

	state := compareStateFromPlan(pl, httpx.Options{}, "Compare items")
	want := "api=dev, app=one? api=stage, app=one*? api=prod, app=one?"
	if got := state.progressSummary(); got != want {
		t.Fatalf("progress summary =\n%q\nwant\n%q", got, want)
	}
	if got := state.envAt(1); got != "api=stage, app=one" {
		t.Fatalf("envAt(1) = %q", got)
	}
	if got := state.envAt(3); got != "" {
		t.Fatalf("envAt(3) = %q, want empty for out of range", got)
	}
}

func TestCompareSpecForRequestRequiresMetadata(t *testing.T) {
	req := &restfile.Request{}
	model := Model{
		cfg: Config{
			Compare: engine.CompareConfig{
				Targets: []string{"cli-dev", "cli-stage"},
				Base:    "cli-stage",
			},
		},
	}
	if spec := model.compareSpecForRequest(req); spec != nil {
		t.Fatalf("expected nil spec when request lacks metadata, got %#v", spec)
	}
}

package ui

import (
	"reflect"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestBuildConfigCompareSpecBaselineFallback(t *testing.T) {
	spec := buildConfigCompareSpec([]string{"dev", "stage", "prod"}, "")
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

func TestBuildConfigCompareSpecAppendsBaseline(t *testing.T) {
	spec := buildConfigCompareSpec([]string{"dev", "stage"}, "prod")
	if spec == nil {
		t.Fatalf("expected spec")
	}
	if spec.Baseline != "prod" {
		t.Fatalf("expected baseline prod, got %s", spec.Baseline)
	}
	expect := []string{"dev", "stage", "prod"}
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
			CompareTargets: []string{"cli-dev", "cli-stage"},
			CompareBase:    "cli-stage",
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
	targets := normalizeCompareTargets([]string{"dev", "DEV", " stage ", ""})
	expect := []string{"dev", "stage"}
	if !reflect.DeepEqual(expect, targets) {
		t.Fatalf("unexpected targets: %#v", targets)
	}
}

func TestCompareSpecForRequestRequiresMetadata(t *testing.T) {
	req := &restfile.Request{}
	model := Model{
		cfg: Config{
			CompareTargets: []string{"cli-dev", "cli-stage"},
			CompareBase:    "cli-stage",
		},
	}
	if spec := model.compareSpecForRequest(req); spec != nil {
		t.Fatalf("expected nil spec when request lacks metadata, got %#v", spec)
	}
}

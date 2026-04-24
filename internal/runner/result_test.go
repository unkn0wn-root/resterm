package runner

import (
	"testing"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/runx/fail"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

func TestRunResultConstructorsTrimOwnedStrings(t *testing.T) {
	req := &restfile.Request{
		Method: " get ",
		URL:    " https://example.com/users ",
		Metadata: restfile.RequestMetadata{
			Name: " user lookup ",
		},
	}

	gotReq := requestRunResult(req, engine.RequestResult{
		Environment: " dev ",
		SkipReason:  " skipped ",
		Tests: []scripts.TestResult{{
			Name:    " status ",
			Message: " failed ",
			Passed:  false,
		}},
	}, " fallback ")
	if gotReq.Name != "user lookup" || gotReq.Method != "GET" ||
		gotReq.Target != "https://example.com/users" {
		t.Fatalf("unexpected request result identity: %+v", gotReq)
	}
	if gotReq.Environment != "dev" || gotReq.SkipReason != "skipped" {
		t.Fatalf("unexpected request result metadata: %+v", gotReq)
	}
	if len(gotReq.Tests) != 1 || gotReq.Tests[0].Name != "status" ||
		gotReq.Tests[0].Message != "failed" {
		t.Fatalf("unexpected request tests: %+v", gotReq.Tests)
	}
	if gotReq.Failure.Code != runfail.CodeAssertion || gotReq.Failure.Source != "tests" {
		t.Fatalf("expected request failure classification, got %+v", gotReq.Failure)
	}

	gotCompare := compareRunResult(req, engine.CompareResult{
		Environment: " stage ",
		Summary:     " compare summary ",
		Baseline:    " dev ",
		Rows: []engine.CompareRow{{
			Environment: " stage ",
			Summary:     " row summary ",
			SkipReason:  " row skipped ",
			Tests: []scripts.TestResult{{
				Name:    " compare test ",
				Message: " compare fail ",
			}},
		}},
	}, " fallback ")
	if gotCompare.Environment != "stage" || gotCompare.Summary != "compare summary" {
		t.Fatalf("unexpected compare result metadata: %+v", gotCompare)
	}
	if gotCompare.Compare == nil || gotCompare.Compare.Baseline != "dev" {
		t.Fatalf("unexpected compare baseline: %+v", gotCompare.Compare)
	}
	if len(gotCompare.Steps) != 1 {
		t.Fatalf("unexpected compare steps: %+v", gotCompare.Steps)
	}
	if step := gotCompare.Steps[0]; step.Name != "stage" || step.Summary != "row summary" ||
		step.SkipReason != "row skipped" || len(step.Tests) != 1 ||
		step.Tests[0].Name != "compare test" || step.Tests[0].Message != "compare fail" {
		t.Fatalf("unexpected compare step: %+v", step)
	}
	if gotCompare.Failure.Code != runfail.CodeAssertion ||
		gotCompare.Steps[0].Failure.Code != runfail.CodeAssertion {
		t.Fatalf("expected compare failure classification, got result=%+v step=%+v",
			gotCompare.Failure, gotCompare.Steps[0].Failure)
	}

	gotProfile := profileRunResult(req, engine.ProfileResult{
		Environment: " perf ",
		Summary:     " profile summary ",
		SkipReason:  " profile skipped ",
		Failures: []engine.ProfileFailure{{
			Reason:  " timeout ",
			Status:  " 500 ",
			Failure: runfail.New(runfail.CodeTimeout, "timeout", "profile"),
		}},
	}, " fallback ")
	if gotProfile.Environment != "perf" || gotProfile.Summary != "profile summary" ||
		gotProfile.SkipReason != "profile skipped" {
		t.Fatalf("unexpected profile result metadata: %+v", gotProfile)
	}
	if gotProfile.Profile == nil || len(gotProfile.Profile.Failures) != 1 ||
		gotProfile.Profile.Failures[0].Reason != " timeout " ||
		gotProfile.Profile.Failures[0].Status != " 500 " {
		t.Fatalf("unexpected profile failures: %+v", gotProfile.Profile)
	}
	if gotProfile.Profile.Failures[0].Failure.Code != runfail.CodeTimeout {
		t.Fatalf("expected profile failure classification, got %+v", gotProfile.Profile.Failures[0].Failure)
	}
	if gotProfile.Failure.Code != runfail.CodeTimeout {
		t.Fatalf("expected profile result failure classification, got %+v", gotProfile.Failure)
	}
	gotProfileFmt := ReportModel(&Report{Results: []Result{gotProfile}})
	if gotProfileFmt.Results[0].Profile.Failures[0].Reason != "timeout" ||
		gotProfileFmt.Results[0].Profile.Failures[0].Status != "500" {
		t.Fatalf("expected report conversion to trim profile failures, got %+v", gotProfileFmt.Results[0].Profile)
	}

	gotWorkflow := workflowRunResult(engine.WorkflowResult{
		Kind:        " workflow ",
		Name:        " nightly ",
		Environment: " prod ",
		Summary:     " workflow summary ",
		Steps: []engine.WorkflowStep{{
			Name:    " login ",
			Method:  " GET ",
			Target:  " https://example.com/login ",
			Branch:  " main ",
			Summary: " step summary ",
			Tests: []scripts.TestResult{{
				Name:    " wf test ",
				Message: " wf fail ",
			}},
		}},
	}, " fallback ")
	if gotWorkflow.Name != "nightly" || gotWorkflow.Method != "WORKFLOW" ||
		gotWorkflow.Environment != "prod" || gotWorkflow.Summary != "workflow summary" {
		t.Fatalf("unexpected workflow result metadata: %+v", gotWorkflow)
	}
	if len(gotWorkflow.Steps) != 1 {
		t.Fatalf("unexpected workflow steps: %+v", gotWorkflow.Steps)
	}
	if step := gotWorkflow.Steps[0]; step.Name != "login" || step.Method != "GET" ||
		step.Target != "https://example.com/login" || step.Branch != "main" ||
		step.Summary != "step summary" || len(step.Tests) != 1 ||
		step.Tests[0].Name != "wf test" || step.Tests[0].Message != "wf fail" {
		t.Fatalf("unexpected workflow step: %+v", step)
	}
	if gotWorkflow.Failure.Code != runfail.CodeAssertion ||
		gotWorkflow.Steps[0].Failure.Code != runfail.CodeAssertion {
		t.Fatalf("expected workflow failure classification, got result=%+v step=%+v",
			gotWorkflow.Failure, gotWorkflow.Steps[0].Failure)
	}
}

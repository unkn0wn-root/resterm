package headless

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/runx/fail"
)

func TestFailureConstantsMatchInternalValues(t *testing.T) {
	codeCases := []struct {
		name     string
		public   FailureCode
		internal runfail.Code
	}{
		{name: "FailureAssertion", public: FailureAssertion, internal: runfail.CodeAssertion},
		{name: "FailureTraceBudget", public: FailureTraceBudget, internal: runfail.CodeTraceBudget},
		{name: "FailureTimeout", public: FailureTimeout, internal: runfail.CodeTimeout},
		{name: "FailureNetwork", public: FailureNetwork, internal: runfail.CodeNetwork},
		{name: "FailureTLS", public: FailureTLS, internal: runfail.CodeTLS},
		{name: "FailureAuth", public: FailureAuth, internal: runfail.CodeAuth},
		{name: "FailureScript", public: FailureScript, internal: runfail.CodeScript},
		{name: "FailureFilesystem", public: FailureFilesystem, internal: runfail.CodeFilesystem},
		{name: "FailureProtocol", public: FailureProtocol, internal: runfail.CodeProtocol},
		{name: "FailureRoute", public: FailureRoute, internal: runfail.CodeRoute},
		{name: "FailureCanceled", public: FailureCanceled, internal: runfail.CodeCanceled},
		{name: "FailureInternal", public: FailureInternal, internal: runfail.CodeInternal},
		{name: "FailureUnknown", public: FailureUnknown, internal: runfail.CodeUnknown},
	}
	for _, tc := range codeCases {
		if string(tc.public) != string(tc.internal) {
			t.Fatalf("%s = %q, want %q", tc.name, tc.public, tc.internal)
		}
	}

	categoryCases := []struct {
		name     string
		public   FailureCategory
		internal runfail.Category
	}{
		{name: "CategorySemantic", public: CategorySemantic, internal: runfail.CategorySemantic},
		{name: "CategoryTimeout", public: CategoryTimeout, internal: runfail.CategoryTimeout},
		{name: "CategoryNetwork", public: CategoryNetwork, internal: runfail.CategoryNetwork},
		{name: "CategoryTLS", public: CategoryTLS, internal: runfail.CategoryTLS},
		{name: "CategoryAuth", public: CategoryAuth, internal: runfail.CategoryAuth},
		{name: "CategoryScript", public: CategoryScript, internal: runfail.CategoryScript},
		{
			name:     "CategoryFilesystem",
			public:   CategoryFilesystem,
			internal: runfail.CategoryFilesystem,
		},
		{name: "CategoryProtocol", public: CategoryProtocol, internal: runfail.CategoryProtocol},
		{name: "CategoryRoute", public: CategoryRoute, internal: runfail.CategoryRoute},
		{name: "CategoryCanceled", public: CategoryCanceled, internal: runfail.CategoryCanceled},
		{name: "CategoryInternal", public: CategoryInternal, internal: runfail.CategoryInternal},
	}
	for _, tc := range categoryCases {
		if string(tc.public) != string(tc.internal) {
			t.Fatalf("%s = %q, want %q", tc.name, tc.public, tc.internal)
		}
	}
}

func TestResultFailsLikeTheRunner(t *testing.T) {
	cases := []struct {
		name string
		res  Result
		code FailureCode
		exit int
	}{
		{
			name: "failed step",
			res: Result{Kind: KindWorkflow, Status: StatusPass, Steps: []Step{
				{Name: "a", Status: StatusPass},
				{Name: "b", Status: StatusPass, Failure: &Failure{Code: FailureTimeout, Message: "slow"}},
			}},
			code: FailureTimeout,
			exit: ExitTimeout,
		},
		{
			name: "measured profile failure",
			res: Result{Kind: KindProfile, Status: StatusPass, Profile: &Profile{Failures: []ProfileFailure{
				{Iteration: 1, Warmup: true, Failure: &Failure{Code: FailureNetwork}},
				{Iteration: 2, Failure: &Failure{Code: FailureTimeout}},
			}}},
			code: FailureTimeout,
			exit: ExitTimeout,
		},
		{
			name: "stream error",
			res:  Result{Kind: KindRequest, Status: StatusPass, Stream: &Stream{Error: "context deadline exceeded"}},
			code: FailureTimeout,
			exit: ExitTimeout,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep := &Report{Results: []Result{tc.res}}
			if !tc.res.Failed() || !rep.HasFailures() {
				t.Fatalf("Failed() = %v, HasFailures() = %v, want both true", tc.res.Failed(), rep.HasFailures())
			}
			if got := rep.ExitCode(ExitCodeSummary); got != ExitFailure {
				t.Fatalf("summary exit = %d, want %d", got, ExitFailure)
			}
			if got := rep.ExitCode(ExitCodeDetailed); got != tc.exit {
				t.Fatalf("detailed exit = %d, want %d", got, tc.exit)
			}
			if got := rep.FailureCodes(); !reflect.DeepEqual(got, []FailureCode{tc.code}) {
				t.Fatalf("FailureCodes() = %v, want [%s]", got, tc.code)
			}
			b, err := json.Marshal(tc.res)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var got struct {
				Status  string `json:"status"`
				Failure struct {
					Code FailureCode `json:"code"`
				} `json:"failure"`
			}
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got.Status != "fail" || got.Failure.Code != tc.code {
				t.Fatalf("json status = %q failure = %q, want fail and %s", got.Status, got.Failure.Code, tc.code)
			}
		})
	}
}

func TestWarmupProfileFailureDoesNotFailResult(t *testing.T) {
	res := Result{Kind: KindProfile, Status: StatusPass, Profile: &Profile{Failures: []ProfileFailure{
		{Iteration: 1, Warmup: true, Failure: &Failure{Code: FailureNetwork}},
	}}}
	rep := &Report{Results: []Result{res}}
	if res.Failed() || rep.HasFailures() {
		t.Fatalf("Failed() = %v, HasFailures() = %v, want both false", res.Failed(), rep.HasFailures())
	}
	if d, s := rep.ExitCode(ExitCodeDetailed), rep.ExitCode(ExitCodeSummary); d != ExitPass || s != ExitPass {
		t.Fatalf("exit detailed = %d summary = %d, want 0 and 0", d, s)
	}
}

func TestHasFailuresMatchesExitCode(t *testing.T) {
	rep := &Report{Results: []Result{{
		Kind:   KindWorkflow,
		Status: StatusSkip,
		Steps:  []Step{{Name: "a", Status: StatusFail, Failure: &Failure{Code: FailureTimeout}}},
	}}}
	if !rep.HasFailures() {
		t.Fatal("HasFailures() = false for a report holding a failed step")
	}
	if d, s := rep.ExitCode(ExitCodeDetailed), rep.ExitCode(ExitCodeSummary); d != ExitTimeout || s != ExitFailure {
		t.Fatalf("exit detailed = %d summary = %d, want %d and %d", d, s, ExitTimeout, ExitFailure)
	}
}

func TestFailureFollowsTheEvidence(t *testing.T) {
	ms := time.Millisecond
	cases := []struct {
		name string
		res  Result
		want *Failure
	}{
		{name: "pass", res: Result{Status: StatusPass}},
		{name: "skip wins", res: Result{Status: StatusSkip, Error: "boom"}},
		{
			name: "canceled before error",
			res:  Result{Canceled: true, Error: "boom"},
			want: &Failure{Code: FailureCanceled, Message: "canceled", Source: "canceled"},
		},
		{
			name: "error before script error",
			res:  Result{Error: "context deadline exceeded", ScriptError: "x is not defined"},
			want: &Failure{Code: FailureTimeout, Message: "context deadline exceeded", Source: "error"},
		},
		{
			name: "script error",
			res:  Result{ScriptError: "x is not defined"},
			want: &Failure{Code: FailureScript, Message: "x is not defined", Source: "scriptError"},
		},
		{
			name: "failed test",
			res:  Result{Tests: []Test{{Name: "ok", Passed: true}, {Name: "status", Message: "want 200"}}},
			want: &Failure{Code: FailureAssertion, Message: "status: want 200", Source: "tests"},
		},
		{
			name: "trace breach",
			res:  Result{Trace: &Trace{Breaches: []TraceBreach{{Kind: "total", Over: 5 * ms}}}},
			want: &Failure{Code: FailureTraceBudget, Message: "trace budget breach total (+5ms)", Source: "trace"},
		},
		{
			name: "status",
			res:  Result{Status: StatusFail, Summary: "expected 200"},
			want: &Failure{Code: FailureAssertion, Message: "expected 200", Source: "status"},
		},
	}
	failureOf := func(t *testing.T, v any) *Failure {
		t.Helper()
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got struct{ Failure *Failure }
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.Failure == nil {
			return nil
		}
		return &Failure{Code: got.Failure.Code, Message: got.Failure.Message, Source: got.Failure.Source}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.res
			step := Step{
				Status:      r.Status,
				Summary:     r.Summary,
				Canceled:    r.Canceled,
				Error:       r.Error,
				ScriptError: r.ScriptError,
				Trace:       r.Trace,
				Tests:       r.Tests,
			}
			for _, v := range []any{r, step} {
				if got := failureOf(t, v); !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("%T failure = %+v, want %+v", v, got, tc.want)
				}
			}
		})
	}
}

func TestFailureCodesEmpty(t *testing.T) {
	if got := (*Report)(nil).FailureCodes(); got != nil {
		t.Fatalf("nil report codes = %v, want nil", got)
	}
	rep := &Report{Results: []Result{{Status: StatusPass}, {Status: StatusSkip}}}
	if got := rep.FailureCodes(); got != nil {
		t.Fatalf("passing report codes = %v, want nil", got)
	}
}

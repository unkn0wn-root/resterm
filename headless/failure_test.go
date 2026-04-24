package headless

import (
	"testing"

	"github.com/unkn0wn-root/resterm/internal/runfail"
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
		{name: "CategoryFilesystem", public: CategoryFilesystem, internal: runfail.CategoryFilesystem},
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

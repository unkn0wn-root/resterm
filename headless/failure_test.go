package headless

import (
	"testing"

	"github.com/unkn0wn-root/resterm/internal/runclass"
)

func TestFailureConstantsMirrorRunclassValues(t *testing.T) {
	codeCases := []struct {
		name     string
		public   FailureCode
		internal runclass.FailureCode
	}{
		{name: "FailureAssertion", public: FailureAssertion, internal: runclass.FailureAssertion},
		{name: "FailureTraceBudget", public: FailureTraceBudget, internal: runclass.FailureTraceBudget},
		{name: "FailureTimeout", public: FailureTimeout, internal: runclass.FailureTimeout},
		{name: "FailureNetwork", public: FailureNetwork, internal: runclass.FailureNetwork},
		{name: "FailureTLS", public: FailureTLS, internal: runclass.FailureTLS},
		{name: "FailureAuth", public: FailureAuth, internal: runclass.FailureAuth},
		{name: "FailureScript", public: FailureScript, internal: runclass.FailureScript},
		{name: "FailureFilesystem", public: FailureFilesystem, internal: runclass.FailureFilesystem},
		{name: "FailureProtocol", public: FailureProtocol, internal: runclass.FailureProtocol},
		{name: "FailureRoute", public: FailureRoute, internal: runclass.FailureRoute},
		{name: "FailureCanceled", public: FailureCanceled, internal: runclass.FailureCanceled},
		{name: "FailureInternal", public: FailureInternal, internal: runclass.FailureInternal},
		{name: "FailureUnknown", public: FailureUnknown, internal: runclass.FailureUnknown},
	}
	for _, tc := range codeCases {
		if string(tc.public) != string(tc.internal) {
			t.Fatalf("%s = %q, want %q", tc.name, tc.public, tc.internal)
		}
	}

	categoryCases := []struct {
		name     string
		public   FailureCategory
		internal runclass.FailureCategory
	}{
		{name: "CategorySemantic", public: CategorySemantic, internal: runclass.CategorySemantic},
		{name: "CategoryTimeout", public: CategoryTimeout, internal: runclass.CategoryTimeout},
		{name: "CategoryNetwork", public: CategoryNetwork, internal: runclass.CategoryNetwork},
		{name: "CategoryTLS", public: CategoryTLS, internal: runclass.CategoryTLS},
		{name: "CategoryAuth", public: CategoryAuth, internal: runclass.CategoryAuth},
		{name: "CategoryScript", public: CategoryScript, internal: runclass.CategoryScript},
		{name: "CategoryFilesystem", public: CategoryFilesystem, internal: runclass.CategoryFilesystem},
		{name: "CategoryProtocol", public: CategoryProtocol, internal: runclass.CategoryProtocol},
		{name: "CategoryRoute", public: CategoryRoute, internal: runclass.CategoryRoute},
		{name: "CategoryCanceled", public: CategoryCanceled, internal: runclass.CategoryCanceled},
		{name: "CategoryInternal", public: CategoryInternal, internal: runclass.CategoryInternal},
	}
	for _, tc := range categoryCases {
		if string(tc.public) != string(tc.internal) {
			t.Fatalf("%s = %q, want %q", tc.name, tc.public, tc.internal)
		}
	}
}

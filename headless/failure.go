package headless

import "github.com/unkn0wn-root/resterm/internal/runclass"

// ExitCodeMode selects whether Report.ExitCode returns detailed classified
// codes or the legacy pass/fail summary codes.
type ExitCodeMode string

const (
	// ExitCodeDetailed returns classified CI exit codes such as timeout,
	// network, TLS, auth, script, filesystem, protocol, route, or canceled.
	ExitCodeDetailed ExitCodeMode = "detailed"
	// ExitCodeSummary returns the legacy pass/fail/usage-style code for reports.
	ExitCodeSummary ExitCodeMode = "summary"
)

const (
	ExitPass       = runclass.ExitPass
	ExitFailure    = runclass.ExitFailure
	ExitUsage      = runclass.ExitUsage
	ExitInternal   = runclass.ExitInternal
	ExitTimeout    = runclass.ExitTimeout
	ExitNetwork    = runclass.ExitNetwork
	ExitTLS        = runclass.ExitTLS
	ExitAuth       = runclass.ExitAuth
	ExitScript     = runclass.ExitScript
	ExitFilesystem = runclass.ExitFilesystem
	ExitProtocol   = runclass.ExitProtocol
	ExitRoute      = runclass.ExitRoute
	ExitCanceled   = runclass.ExitCanceled
)

// FailureCode identifies the stable machine-readable reason for a failed item.
type FailureCode string

const (
	FailureAssertion   FailureCode = FailureCode(runclass.FailureAssertion)
	FailureTraceBudget FailureCode = FailureCode(runclass.FailureTraceBudget)
	FailureTimeout     FailureCode = FailureCode(runclass.FailureTimeout)
	FailureNetwork     FailureCode = FailureCode(runclass.FailureNetwork)
	FailureTLS         FailureCode = FailureCode(runclass.FailureTLS)
	FailureAuth        FailureCode = FailureCode(runclass.FailureAuth)
	FailureScript      FailureCode = FailureCode(runclass.FailureScript)
	FailureFilesystem  FailureCode = FailureCode(runclass.FailureFilesystem)
	FailureProtocol    FailureCode = FailureCode(runclass.FailureProtocol)
	FailureRoute       FailureCode = FailureCode(runclass.FailureRoute)
	FailureCanceled    FailureCode = FailureCode(runclass.FailureCanceled)
	FailureInternal    FailureCode = FailureCode(runclass.FailureInternal)
	FailureUnknown     FailureCode = FailureCode(runclass.FailureUnknown)
)

// FailureCategory groups failure codes into broader operational categories.
type FailureCategory string

const (
	CategorySemantic   FailureCategory = FailureCategory(runclass.CategorySemantic)
	CategoryTimeout    FailureCategory = FailureCategory(runclass.CategoryTimeout)
	CategoryNetwork    FailureCategory = FailureCategory(runclass.CategoryNetwork)
	CategoryTLS        FailureCategory = FailureCategory(runclass.CategoryTLS)
	CategoryAuth       FailureCategory = FailureCategory(runclass.CategoryAuth)
	CategoryScript     FailureCategory = FailureCategory(runclass.CategoryScript)
	CategoryFilesystem FailureCategory = FailureCategory(runclass.CategoryFilesystem)
	CategoryProtocol   FailureCategory = FailureCategory(runclass.CategoryProtocol)
	CategoryRoute      FailureCategory = FailureCategory(runclass.CategoryRoute)
	CategoryCanceled   FailureCategory = FailureCategory(runclass.CategoryCanceled)
	CategoryInternal   FailureCategory = FailureCategory(runclass.CategoryInternal)
)

// Failure contains structured machine-readable metadata for a failed result,
// workflow step, compare step, or profile iteration.
type Failure struct {
	Code     FailureCode     `json:"code,omitempty"`
	Category FailureCategory `json:"category,omitempty"`
	ExitCode int             `json:"exitCode,omitempty"`
	Message  string          `json:"message,omitempty"`
	Source   string          `json:"source,omitempty"`
}

// ExitCode returns the report exit code for the requested mode.
func (r *Report) ExitCode(mode ExitCodeMode) int {
	if r == nil {
		return ExitPass
	}
	rep := toFormatReport(r)
	return runclass.ReportExitCode(rep.Failures(), r.HasFailures(), runclass.ExitCodeMode(mode))
}

// FailureCodes returns the unique failure codes present in the report.
func (r *Report) FailureCodes() []FailureCode {
	if r == nil {
		return nil
	}
	src := toFormatReport(r).FailureCodes()
	if len(src) == 0 {
		return nil
	}
	out := make([]FailureCode, 0, len(src))
	for _, code := range src {
		out = append(out, FailureCode(code))
	}
	return out
}

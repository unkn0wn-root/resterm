package headless

import (
	"github.com/unkn0wn-root/resterm/internal/runx/fail"
	"github.com/unkn0wn-root/resterm/internal/runx/report"
)

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
	ExitPass       = 0
	ExitFailure    = 1
	ExitUsage      = 2
	ExitInternal   = 3
	ExitTimeout    = 20
	ExitNetwork    = 21
	ExitTLS        = 22
	ExitAuth       = 23
	ExitScript     = 24
	ExitFilesystem = 25
	ExitProtocol   = 26
	ExitRoute      = 27
	ExitCanceled   = 130
)

// FailureCode identifies the stable machine-readable reason for a failed item.
type FailureCode string

const (
	FailureAssertion   FailureCode = "assertion"
	FailureTraceBudget FailureCode = "trace_budget"
	FailureTimeout     FailureCode = "timeout"
	FailureNetwork     FailureCode = "network"
	FailureTLS         FailureCode = "tls"
	FailureAuth        FailureCode = "auth"
	FailureScript      FailureCode = "script"
	FailureFilesystem  FailureCode = "filesystem"
	FailureProtocol    FailureCode = "protocol"
	FailureRoute       FailureCode = "route"
	FailureCanceled    FailureCode = "canceled"
	FailureInternal    FailureCode = "internal"
	FailureUnknown     FailureCode = "unknown"
)

// FailureCategory groups failure codes into broader operational categories.
type FailureCategory string

const (
	CategorySemantic   FailureCategory = "semantic"
	CategoryTimeout    FailureCategory = "timeout"
	CategoryNetwork    FailureCategory = "network"
	CategoryTLS        FailureCategory = "tls"
	CategoryAuth       FailureCategory = "auth"
	CategoryScript     FailureCategory = "script"
	CategoryFilesystem FailureCategory = "filesystem"
	CategoryProtocol   FailureCategory = "protocol"
	CategoryRoute      FailureCategory = "route"
	CategoryCanceled   FailureCategory = "canceled"
	CategoryInternal   FailureCategory = "internal"
)

// Failure contains structured machine-readable metadata for a failed result,
// workflow step, compare step, or profile iteration.
type Failure struct {
	Code     FailureCode     `json:"code,omitempty"`
	Category FailureCategory `json:"category,omitempty"`
	ExitCode int             `json:"exitCode,omitempty"`
	Message  string          `json:"message,omitempty"`
	Source   string          `json:"source,omitempty"`
	Chain    []FailureChain  `json:"chain,omitempty"`
	Frames   []FailureFrame  `json:"frames,omitempty"`
}

// FailureChain contains one context or cause entry in a failure chain.
type FailureChain = runfmt.FailureChain

// FailureFrame contains one runtime stack frame attached to a failure.
type FailureFrame = runfmt.FailureFrame

// FailurePos identifies a source location in a failure.
type FailurePos = runfmt.FailurePos

// ExitCode returns the report exit code for the requested mode.
func (r *Report) ExitCode(mode ExitCodeMode) int {
	if r == nil {
		return ExitPass
	}
	rep := toFormatReport(r)
	return runfail.ExitCode(rep.Failures(), r.HasFailures(), runfail.ExitMode(mode))
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

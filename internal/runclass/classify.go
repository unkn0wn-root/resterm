package runclass

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io/fs"
	"net"
	"net/url"
	"os"
	"slices"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ExitCodeMode string

const (
	ExitCodeModeDetailed ExitCodeMode = "detailed"
	ExitCodeModeSummary  ExitCodeMode = "summary"
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

type Failure struct {
	Code     FailureCode
	Category FailureCategory
	ExitCode int
	Message  string
	Source   string
}

func ValidExitCodeMode(mode ExitCodeMode) bool {
	switch mode {
	case "", ExitCodeModeDetailed, ExitCodeModeSummary:
		return true
	default:
		return false
	}
}

func SummaryExitCode(failed bool) int {
	if failed {
		return ExitFailure
	}
	return ExitPass
}

func ReportExitCode(failures []Failure, failed bool, mode ExitCodeMode) int {
	if mode == ExitCodeModeSummary {
		return SummaryExitCode(failed)
	}
	if len(failures) == 0 {
		return SummaryExitCode(failed)
	}
	return DominantFailure(failures).ExitCode
}

func DominantFailure(failures []Failure) Failure {
	var out Failure
	for _, f := range failures {
		f = normalizeFailure(f)
		if out.Code == "" || failurePriority(f) < failurePriority(out) {
			out = f
		}
	}
	return out
}

func NewFailure(code FailureCode, message, source string) Failure {
	o := Failure{
		Code:    code,
		Message: message,
		Source:  source,
	}
	return normalizeFailure(o)
}

func AssertionFailure(message, source string) Failure {
	if message == "" {
		message = "assertion failed"
	}
	return NewFailure(FailureAssertion, message, source)
}

func TraceBudgetFailure(message string) Failure {
	if message == "" {
		message = "trace budget breached"
	}
	return NewFailure(FailureTraceBudget, message, "trace")
}

func ScriptFailure(message, source string) Failure {
	if source == "" {
		source = "scriptError"
	}
	return NewFailure(FailureScript, message, source)
}

func CanceledFailure(message, source string) Failure {
	if message == "" {
		message = "canceled"
	}
	return NewFailure(FailureCanceled, message, source)
}

func ClassifyError(err error) Failure {
	return ClassifyErrorSource(err, "error")
}

func ClassifyErrorSource(err error, source string) Failure {
	if err == nil {
		return Failure{}
	}
	msg := err.Error()
	switch {
	case isCanceled(err):
		return NewFailure(FailureCanceled, msg, source)
	case isTimeout(err):
		return NewFailure(FailureTimeout, msg, source)
	case isTLS(err):
		return NewFailure(FailureTLS, msg, source)
	}

	if f, ok := classifyGRPC(err, msg, source); ok {
		return f
	}
	if f, ok := classifyTypedNetwork(err, msg, source); ok {
		return f
	}
	if f, ok := classifyTypedFilesystem(err, msg, source); ok {
		return f
	}
	if f, ok := classifyErrdef(err, msg, source); ok {
		return f
	}
	return classifyMessage(msg, source)
}

func classifyGRPC(err error, msg, source string) (Failure, bool) {
	st, ok := status.FromError(err)
	if !ok {
		return Failure{}, false
	}
	code := st.Code()
	switch code {
	case codes.OK:
		return Failure{}, false
	case codes.Canceled:
		return NewFailure(FailureCanceled, msg, source), true
	case codes.DeadlineExceeded:
		return NewFailure(FailureTimeout, msg, source), true
	case codes.Unavailable:
		return NewFailure(FailureNetwork, msg, source), true
	case codes.Unauthenticated, codes.PermissionDenied:
		return NewFailure(FailureAuth, msg, source), true
	default:
		return NewFailure(FailureProtocol, msg, source), true
	}
}

func classifyTypedNetwork(err error, msg, source string) (Failure, bool) {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return NewFailure(FailureNetwork, msg, source), true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return NewFailure(FailureNetwork, msg, source), true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return NewFailure(FailureNetwork, msg, source), true
	}
	return Failure{}, false
}

func classifyTypedFilesystem(err error, msg, source string) (Failure, bool) {
	switch {
	case errors.Is(err, fs.ErrNotExist),
		errors.Is(err, fs.ErrPermission),
		errors.Is(err, fs.ErrExist),
		errors.Is(err, fs.ErrClosed):
		return NewFailure(FailureFilesystem, msg, source), true
	}
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return NewFailure(FailureFilesystem, msg, source), true
	}
	return Failure{}, false
}

func classifyErrdef(err error, msg, source string) (Failure, bool) {
	codes := errdef.Codes(err)
	if len(codes) == 0 {
		return Failure{}, false
	}
	if c, ok := dominantErrdefFailureCode(codes); ok {
		return NewFailure(c, msg, source), true
	}
	if containsErrdefCode(codes, errdef.CodeHTTP) {
		return classifyHTTPMessage(msg, source), true
	}
	return Failure{}, false
}

func dominantErrdefFailureCode(codes []errdef.Code) (FailureCode, bool) {
	var out FailureCode
	for _, c := range codes {
		fc, ok := errdefFailureCode(c)
		if !ok {
			continue
		}
		if out == "" || failurePriority(Failure{Code: fc}) < failurePriority(Failure{Code: out}) {
			out = fc
		}
	}
	return out, out != ""
}

func containsErrdefCode(codes []errdef.Code, want errdef.Code) bool {
	return slices.Contains(codes, want)
}

func errdefFailureCode(code errdef.Code) (FailureCode, bool) {
	switch code {
	case errdef.CodeTimeout:
		return FailureTimeout, true
	case errdef.CodeCanceled:
		return FailureCanceled, true
	case errdef.CodeNetwork:
		return FailureNetwork, true
	case errdef.CodeTLS:
		return FailureTLS, true
	case errdef.CodeAuth:
		return FailureAuth, true
	case errdef.CodeProtocol:
		return FailureProtocol, true
	case errdef.CodeRoute:
		return FailureRoute, true
	case errdef.CodeScript:
		return FailureScript, true
	case errdef.CodeFilesystem, errdef.CodeHistory:
		return FailureFilesystem, true
	case errdef.CodeConfig, errdef.CodeParse, errdef.CodeUI:
		return FailureInternal, true
	default:
		return "", false
	}
}

func isCanceled(err error) bool {
	return errors.Is(err, context.Canceled)
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func isTLS(err error) bool {
	var unknownAuthority x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthority) {
		return true
	}
	var hostname x509.HostnameError
	if errors.As(err, &hostname) {
		return true
	}
	var invalid x509.CertificateInvalidError
	if errors.As(err, &invalid) {
		return true
	}
	var roots x509.SystemRootsError
	if errors.As(err, &roots) {
		return true
	}
	var record tls.RecordHeaderError
	return errors.As(err, &record)
}

func normalizeFailure(f Failure) Failure {
	switch f.Code {
	case FailureAssertion:
		f.Category = CategorySemantic
		f.ExitCode = ExitFailure
	case FailureTraceBudget:
		f.Category = CategorySemantic
		f.ExitCode = ExitFailure
	case FailureTimeout:
		f.Category = CategoryTimeout
		f.ExitCode = ExitTimeout
	case FailureNetwork:
		f.Category = CategoryNetwork
		f.ExitCode = ExitNetwork
	case FailureTLS:
		f.Category = CategoryTLS
		f.ExitCode = ExitTLS
	case FailureAuth:
		f.Category = CategoryAuth
		f.ExitCode = ExitAuth
	case FailureScript:
		f.Category = CategoryScript
		f.ExitCode = ExitScript
	case FailureFilesystem:
		f.Category = CategoryFilesystem
		f.ExitCode = ExitFilesystem
	case FailureProtocol:
		f.Category = CategoryProtocol
		f.ExitCode = ExitProtocol
	case FailureRoute:
		f.Category = CategoryRoute
		f.ExitCode = ExitRoute
	case FailureCanceled:
		f.Category = CategoryCanceled
		f.ExitCode = ExitCanceled
	case FailureInternal:
		f.Category = CategoryInternal
		f.ExitCode = ExitInternal
	default:
		f.Code = FailureUnknown
		f.Category = CategoryInternal
		f.ExitCode = ExitInternal
	}
	return f
}

func failurePriority(f Failure) int {
	switch f.Code {
	case FailureCanceled:
		return 0
	case FailureTimeout:
		return 10
	case FailureNetwork:
		return 20
	case FailureTLS:
		return 30
	case FailureAuth:
		return 40
	case FailureScript:
		return 50
	case FailureFilesystem:
		return 60
	case FailureProtocol:
		return 70
	case FailureRoute:
		return 80
	case FailureInternal, FailureUnknown:
		return 90
	case FailureAssertion, FailureTraceBudget:
		return 100
	default:
		return 1000
	}
}

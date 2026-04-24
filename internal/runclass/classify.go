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
	"strings"
	"unicode"

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

func NormalizeExitCodeMode(mode ExitCodeMode) ExitCodeMode {
	if mode == "" {
		return ExitCodeModeDetailed
	}
	return mode
}

func SummaryExitCode(failed bool) int {
	if failed {
		return ExitFailure
	}
	return ExitPass
}

func ReportExitCode(failures []Failure, failed bool, mode ExitCodeMode) int {
	if NormalizeExitCodeMode(mode) == ExitCodeModeSummary {
		return SummaryExitCode(failed)
	}
	if len(failures) == 0 {
		return SummaryExitCode(failed)
	}
	return DominantFailure(failures).ExitCode
}

func DominantFailure(failures []Failure) Failure {
	var out Failure
	for _, failure := range failures {
		failure = normalizeFailure(failure)
		if out.Code == "" || failurePriority(failure) < failurePriority(out) {
			out = failure
		}
	}
	return out
}

func NewFailure(code FailureCode, message, source string) Failure {
	out := Failure{
		Code:    code,
		Message: message,
		Source:  source,
	}
	return normalizeFailure(out)
}

func AssertionFailure(message, source string) Failure {
	if strings.TrimSpace(message) == "" {
		message = "assertion failed"
	}
	return NewFailure(FailureAssertion, message, source)
}

func TraceBudgetFailure(message string) Failure {
	if strings.TrimSpace(message) == "" {
		message = "trace budget breached"
	}
	return NewFailure(FailureTraceBudget, message, "trace")
}

func ScriptFailure(message, source string) Failure {
	if strings.TrimSpace(source) == "" {
		source = "scriptError"
	}
	return NewFailure(FailureScript, message, source)
}

func CanceledFailure(message, source string) Failure {
	if strings.TrimSpace(message) == "" {
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

	if failure, ok := classifyGRPC(err, msg, source); ok {
		return failure
	}
	if failure, ok := classifyTypedNetwork(err, msg, source); ok {
		return failure
	}
	if failure, ok := classifyTypedFilesystem(err, msg, source); ok {
		return failure
	}
	if failure, ok := classifyErrdef(err, msg, source); ok {
		return failure
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
	switch errdef.CodeOf(err) {
	case errdef.CodeScript:
		return NewFailure(FailureScript, msg, source), true
	case errdef.CodeFilesystem, errdef.CodeHistory:
		return NewFailure(FailureFilesystem, msg, source), true
	case errdef.CodeConfig, errdef.CodeParse:
		return NewFailure(FailureInternal, msg, source), true
	case errdef.CodeUI:
		return NewFailure(FailureInternal, msg, source), true
	case errdef.CodeHTTP:
		return classifyHTTPMessage(msg, source), true
	default:
		return Failure{}, false
	}
}

type messageRule struct {
	code    FailureCode
	phrases []string
	tokens  []string
}

var commonMessageRules = []messageRule{
	{code: FailureTimeout, phrases: []string{"deadline exceeded", "timed out"}, tokens: []string{"timeout"}},
	{code: FailureTLS, tokens: []string{"certificate", "x509", "tls", "ssl"}},
	{code: FailureAuth, tokens: []string{"oauth", "auth", "authorization", "credential", "token"}},
	{
		code:    FailureRoute,
		phrases: []string{"port-forward", "port forward"},
		tokens:  []string{"ssh", "k8s", "kubernetes", "tunnel"},
	},
	{code: FailureProtocol, tokens: []string{"grpc", "websocket", "sse", "stream", "decode"}},
}

var genericMessageRules = []messageRule{
	{
		code:    FailureFilesystem,
		phrases: []string{"file not found", "no such file", "permission denied"},
		tokens:  []string{"file", "files", "filesystem", "directory"},
	},
	{
		code:    FailureNetwork,
		phrases: []string{"no such host", "connection refused"},
		tokens:  []string{"connect", "dial", "dns", "network"},
	},
}

var httpMessageRules = []messageRule{
	{
		code:    FailureNetwork,
		phrases: []string{"no such host", "connection refused", "connection reset"},
		tokens:  []string{"connect", "dial", "dns", "network", "proxy"},
	},
}

func classifyHTTPMessage(msg, source string) Failure {
	return classifyMessageWithRules(msg, source, FailureProtocol, httpMessageRules)
}

func classifyMessage(msg, source string) Failure {
	return classifyMessageWithRules(msg, source, FailureUnknown, genericMessageRules)
}

func classifyMessageWithRules(msg, source string, defaultCode FailureCode, rules []messageRule) Failure {
	lower := strings.ToLower(msg)
	tokens := messageTokens(lower)
	for _, rule := range commonMessageRules {
		if matchesMessageRule(lower, tokens, rule) {
			return NewFailure(rule.code, msg, source)
		}
	}
	for _, rule := range rules {
		if matchesMessageRule(lower, tokens, rule) {
			return NewFailure(rule.code, msg, source)
		}
	}
	return NewFailure(defaultCode, msg, source)
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

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func matchesMessageRule(s string, tokens map[string]struct{}, rule messageRule) bool {
	return containsAny(s, rule.phrases...) || containsAnyToken(tokens, rule.tokens...)
}

func containsAnyToken(tokens map[string]struct{}, words ...string) bool {
	for _, word := range words {
		if _, ok := tokens[word]; ok {
			return true
		}
	}
	return false
}

func messageTokens(s string) map[string]struct{} {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	tokens := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		tokens[field] = struct{}{}
	}
	return tokens
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

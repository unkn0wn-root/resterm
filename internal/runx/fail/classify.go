package runfail

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

type ExitMode string

const (
	ExitDetailed ExitMode = "detailed"
	ExitSummary  ExitMode = "summary"
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

type Code string

const (
	CodeAssertion   Code = "assertion"
	CodeTraceBudget Code = "trace_budget"
	CodeTimeout     Code = "timeout"
	CodeNetwork     Code = "network"
	CodeTLS         Code = "tls"
	CodeAuth        Code = "auth"
	CodeScript      Code = "script"
	CodeFilesystem  Code = "filesystem"
	CodeProtocol    Code = "protocol"
	CodeRoute       Code = "route"
	CodeCanceled    Code = "canceled"
	CodeInternal    Code = "internal"
	CodeUnknown     Code = "unknown"
)

type Category string

const (
	CategorySemantic   Category = "semantic"
	CategoryTimeout    Category = "timeout"
	CategoryNetwork    Category = "network"
	CategoryTLS        Category = "tls"
	CategoryAuth       Category = "auth"
	CategoryScript     Category = "script"
	CategoryFilesystem Category = "filesystem"
	CategoryProtocol   Category = "protocol"
	CategoryRoute      Category = "route"
	CategoryCanceled   Category = "canceled"
	CategoryInternal   Category = "internal"
)

type Failure struct {
	Code     Code
	Category Category
	ExitCode int
	Message  string
	Source   string
}

type Meta struct {
	Category Category
	ExitCode int
	Rank     int
}

var codeOrder = []Code{
	CodeAssertion,
	CodeTraceBudget,
	CodeTimeout,
	CodeNetwork,
	CodeTLS,
	CodeAuth,
	CodeScript,
	CodeFilesystem,
	CodeProtocol,
	CodeRoute,
	CodeCanceled,
	CodeInternal,
	CodeUnknown,
}

var catalog = map[Code]Meta{
	CodeAssertion:   {Category: CategorySemantic, ExitCode: ExitFailure, Rank: 100},
	CodeTraceBudget: {Category: CategorySemantic, ExitCode: ExitFailure, Rank: 100},
	CodeTimeout:     {Category: CategoryTimeout, ExitCode: ExitTimeout, Rank: 10},
	CodeNetwork:     {Category: CategoryNetwork, ExitCode: ExitNetwork, Rank: 20},
	CodeTLS:         {Category: CategoryTLS, ExitCode: ExitTLS, Rank: 30},
	CodeAuth:        {Category: CategoryAuth, ExitCode: ExitAuth, Rank: 40},
	CodeScript:      {Category: CategoryScript, ExitCode: ExitScript, Rank: 50},
	CodeFilesystem:  {Category: CategoryFilesystem, ExitCode: ExitFilesystem, Rank: 60},
	CodeProtocol:    {Category: CategoryProtocol, ExitCode: ExitProtocol, Rank: 70},
	CodeRoute:       {Category: CategoryRoute, ExitCode: ExitRoute, Rank: 80},
	CodeCanceled:    {Category: CategoryCanceled, ExitCode: ExitCanceled, Rank: 0},
	CodeInternal:    {Category: CategoryInternal, ExitCode: ExitInternal, Rank: 90},
	CodeUnknown:     {Category: CategoryInternal, ExitCode: ExitInternal, Rank: 90},
}

func KnownCodes() []Code {
	return slices.Clone(codeOrder)
}

func Lookup(code Code) (Meta, bool) {
	m, ok := catalog[code]
	return m, ok
}

func metaOf(code Code) (Code, Meta) {
	if m, ok := catalog[code]; ok {
		return code, m
	}
	return CodeUnknown, catalog[CodeUnknown]
}

func ValidExitMode(mode ExitMode) bool {
	switch mode {
	case "", ExitDetailed, ExitSummary:
		return true
	default:
		return false
	}
}

func SummaryCode(failed bool) int {
	if failed {
		return ExitFailure
	}
	return ExitPass
}

func ExitCode(failures []Failure, failed bool, mode ExitMode) int {
	if mode == ExitSummary {
		return SummaryCode(failed)
	}
	if len(failures) == 0 {
		return SummaryCode(failed)
	}
	return Dominant(failures).ExitCode
}

func Dominant(failures []Failure) Failure {
	var out Failure
	for _, f := range failures {
		f = New(f.Code, f.Message, f.Source)
		if out.Code == "" || rankOf(f.Code) < rankOf(out.Code) {
			out = f
		}
	}
	return out
}

func New(code Code, message, source string) Failure {
	code, m := metaOf(code)
	return Failure{
		Code:     code,
		Category: m.Category,
		ExitCode: m.ExitCode,
		Message:  message,
		Source:   source,
	}
}

func Assertion(message, source string) Failure {
	if message == "" {
		message = "assertion failed"
	}
	return New(CodeAssertion, message, source)
}

func TraceBudget(message string) Failure {
	if message == "" {
		message = "trace budget breached"
	}
	return New(CodeTraceBudget, message, "trace")
}

func Script(message, source string) Failure {
	if source == "" {
		source = "scriptError"
	}
	return New(CodeScript, message, source)
}

func Canceled(message, source string) Failure {
	if message == "" {
		message = "canceled"
	}
	return New(CodeCanceled, message, source)
}

func FromError(err error) Failure {
	return FromErrorSource(err, "error")
}

func FromErrorSource(err error, source string) Failure {
	if err == nil {
		return Failure{}
	}
	msg := err.Error()
	switch {
	case isCanceled(err):
		return New(CodeCanceled, msg, source)
	case isTimeout(err):
		return New(CodeTimeout, msg, source)
	case isTLS(err):
		return New(CodeTLS, msg, source)
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
		return New(CodeCanceled, msg, source), true
	case codes.DeadlineExceeded:
		return New(CodeTimeout, msg, source), true
	case codes.Unavailable:
		return New(CodeNetwork, msg, source), true
	case codes.Unauthenticated, codes.PermissionDenied:
		return New(CodeAuth, msg, source), true
	default:
		return New(CodeProtocol, msg, source), true
	}
}

func classifyTypedNetwork(err error, msg, source string) (Failure, bool) {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return New(CodeNetwork, msg, source), true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return New(CodeNetwork, msg, source), true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return New(CodeNetwork, msg, source), true
	}
	return Failure{}, false
}

func classifyTypedFilesystem(err error, msg, source string) (Failure, bool) {
	switch {
	case errors.Is(err, fs.ErrNotExist),
		errors.Is(err, fs.ErrPermission),
		errors.Is(err, fs.ErrExist),
		errors.Is(err, fs.ErrClosed):
		return New(CodeFilesystem, msg, source), true
	}
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return New(CodeFilesystem, msg, source), true
	}
	return Failure{}, false
}

func classifyErrdef(err error, msg, source string) (Failure, bool) {
	codes := errdef.Codes(err)
	if len(codes) == 0 {
		return Failure{}, false
	}
	if c, ok := dominantErrdefFailureCode(codes); ok {
		return New(c, msg, source), true
	}
	if containsErrdefCode(codes, errdef.CodeHTTP) {
		return classifyHTTPMessage(msg, source), true
	}
	return Failure{}, false
}

func dominantErrdefFailureCode(codes []errdef.Code) (Code, bool) {
	var out Code
	for _, c := range codes {
		fc, ok := errdefFailureCode(c)
		if !ok {
			continue
		}
		if out == "" || rankOf(fc) < rankOf(out) {
			out = fc
		}
	}
	return out, out != ""
}

func containsErrdefCode(codes []errdef.Code, want errdef.Code) bool {
	return slices.Contains(codes, want)
}

func errdefFailureCode(code errdef.Code) (Code, bool) {
	switch code {
	case errdef.CodeTimeout:
		return CodeTimeout, true
	case errdef.CodeCanceled:
		return CodeCanceled, true
	case errdef.CodeNetwork:
		return CodeNetwork, true
	case errdef.CodeTLS:
		return CodeTLS, true
	case errdef.CodeAuth:
		return CodeAuth, true
	case errdef.CodeProtocol:
		return CodeProtocol, true
	case errdef.CodeRoute:
		return CodeRoute, true
	case errdef.CodeScript:
		return CodeScript, true
	case errdef.CodeFilesystem, errdef.CodeHistory:
		return CodeFilesystem, true
	case errdef.CodeConfig, errdef.CodeParse, errdef.CodeUI:
		return CodeInternal, true
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

func rankOf(code Code) int {
	_, m := metaOf(code)
	return m.Rank
}

package runfail

import (
	"slices"

	"github.com/unkn0wn-root/resterm/internal/diag"
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
	if f, ok := classifyDiag(err, msg, source); ok {
		return f
	}
	return classifyMessage(msg, source)
}

func classifyDiag(err error, msg, source string) (Failure, bool) {
	classes := diag.Classes(err)
	if len(classes) == 0 {
		return Failure{}, false
	}
	if c, ok := dominantDiagFailureCode(classes); ok {
		return New(c, msg, source), true
	}
	return Failure{}, false
}

func dominantDiagFailureCode(classes []diag.Class) (Code, bool) {
	var out Code
	for _, c := range classes {
		fc, ok := diagFailureCode(c)
		if !ok {
			continue
		}
		if out == "" || rankOf(fc) < rankOf(out) {
			out = fc
		}
	}
	return out, out != ""
}

func diagFailureCode(class diag.Class) (Code, bool) {
	switch class {
	case diag.ClassTimeout:
		return CodeTimeout, true
	case diag.ClassCanceled:
		return CodeCanceled, true
	case diag.ClassNetwork:
		return CodeNetwork, true
	case diag.ClassTLS:
		return CodeTLS, true
	case diag.ClassAuth:
		return CodeAuth, true
	case diag.ClassProtocol:
		return CodeProtocol, true
	case diag.ClassRoute:
		return CodeRoute, true
	case diag.ClassScript:
		return CodeScript, true
	case diag.ClassFilesystem, diag.ClassHistory:
		return CodeFilesystem, true
	case diag.ClassConfig, diag.ClassParse, diag.ClassUI, diag.ClassInternal:
		return CodeInternal, true
	default:
		return "", false
	}
}

func rankOf(code Code) int {
	_, m := metaOf(code)
	return m.Rank
}

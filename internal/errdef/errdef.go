package errdef

import (
	stdErrors "errors"
	"fmt"
)

type Code string

const (
	CodeUnknown    Code = "unknown"
	CodeConfig     Code = "config"
	CodeParse      Code = "parse"
	CodeHTTP       Code = "http"
	CodeTimeout    Code = "timeout"
	CodeCanceled   Code = "canceled"
	CodeNetwork    Code = "network"
	CodeTLS        Code = "tls"
	CodeAuth       Code = "auth"
	CodeProtocol   Code = "protocol"
	CodeRoute      Code = "route"
	CodeFilesystem Code = "filesystem"
	CodeScript     Code = "script"
	CodeHistory    Code = "history"
	CodeUI         Code = "ui"
)

type Error struct {
	Code    Code
	Message string
	Err     error
}

func (e *Error) Error() string {
	switch {
	case e == nil:
		return ""
	case e.Err != nil && e.Message != "":
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	case e.Err != nil:
		return fmt.Sprintf("%s: %v", e.Code, e.Err)
	case e.Message != "":
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	default:
		return string(e.Code)
	}
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func Wrap(code Code, err error, format string, args ...any) error {
	if err == nil {
		return nil
	}

	msg := ""
	if format != "" {
		msg = fmt.Sprintf(format, args...)
	}
	return &Error{Code: ensureCode(code), Message: msg, Err: err}
}

func New(code Code, format string, args ...any) error {
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	return &Error{Code: ensureCode(code), Message: msg}
}

func Join(code Code, errs ...error) error {
	err := stdErrors.Join(errs...)
	if err == nil {
		return nil
	}
	return &Error{Code: ensureCode(code), Err: err}
}

func CodeOf(err error) Code {
	var e *Error
	if stdErrors.As(err, &e) {
		return e.Code
	}
	return CodeUnknown
}

func Codes(err error) []Code {
	var out []Code
	seen := make(map[Code]struct{})
	collectCodes(err, func(code Code) {
		if _, ok := seen[code]; ok {
			return
		}
		seen[code] = struct{}{}
		out = append(out, code)
	})
	return out
}

func HasCode(err error, code Code) bool {
	found := false
	collectCodes(err, func(got Code) {
		found = found || got == code
	})
	return found
}

func collectCodes(err error, visit func(Code)) {
	if err == nil {
		return
	}
	if e, ok := err.(*Error); ok {
		visit(e.Code)
	}
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range multi.Unwrap() {
			collectCodes(child, visit)
		}
		return
	}
	if single, ok := err.(interface{ Unwrap() error }); ok {
		collectCodes(single.Unwrap(), visit)
	}
}

func ensureCode(code Code) Code {
	if code == "" {
		return CodeUnknown
	}
	return code
}

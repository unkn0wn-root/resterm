package errdef

import (
	stdErrors "errors"
	"fmt"
)

type Code string

const (
	CodeUnknown    Code = "unknown"
	CodeParse      Code = "parse"
	CodeHTTP       Code = "http"
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

func Wrap(code Code, err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}

	msg := ""
	if format != "" {
		msg = fmt.Sprintf(format, args...)
	}
	return &Error{Code: ensureCode(code), Message: msg, Err: err}
}

func New(code Code, format string, args ...interface{}) error {
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	return &Error{Code: ensureCode(code), Message: msg}
}

func CodeOf(err error) Code {
	var e *Error
	if stdErrors.As(err, &e) {
		return e.Code
	}
	return CodeUnknown
}

func Is(err error, code Code) bool {
	if err == nil {
		return false
	}
	var e *Error
	if stdErrors.As(err, &e) {
		return e.Code == code
	}
	return false
}

func Message(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func ensureCode(code Code) Code {
	if code == "" {
		return CodeUnknown
	}
	return code
}

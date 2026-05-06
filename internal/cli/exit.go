package cli

import "errors"

type ExitErr struct {
	Err  error
	Code int
}

type exitCoder interface {
	ExitCode() int
}

func (e ExitErr) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e ExitErr) Unwrap() error {
	return e.Err
}

func (e ExitErr) ExitCode() int {
	if e.Code == 0 {
		return 1
	}
	return e.Code
}

// IsExitCodeOnly reports whether err is only an exit-code carrier.
//
// Commands use this after they have already written their output and need to
// return a non-zero process status without producing another stderr diagnostic.
func IsExitCodeOnly(err error) bool {
	switch e := err.(type) {
	case ExitErr:
		return e.Err == nil
	case *ExitErr:
		return e != nil && e.Err == nil
	default:
		return false
	}
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var ex exitCoder
	if errors.As(err, &ex) {
		return ex.ExitCode()
	}
	return 1
}

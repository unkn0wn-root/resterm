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

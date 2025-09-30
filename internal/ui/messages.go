package ui

import (
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

type statusPulseMsg struct{}

type statusLevel int

const (
	statusInfo statusLevel = iota
	statusWarn
	statusError
	statusSuccess
)

type responseMsg struct {
	response    *httpclient.Response
	grpc        *grpcclient.Response
	err         error
	tests       []scripts.TestResult
	scriptErr   error
	executed    *restfile.Request
	requestText string
	environment string
}

type statusMsg struct {
	text  string
	level statusLevel
}

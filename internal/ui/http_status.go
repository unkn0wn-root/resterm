package ui

import "net/http"

type httpStatusClass uint8

const (
	httpStatusOther httpStatusClass = iota
	httpStatusSuccess
	httpStatusClientError
)

func classifyHTTPStatus(code int) httpStatusClass {
	switch {
	case code >= http.StatusOK && code < http.StatusMultipleChoices:
		return httpStatusSuccess
	case code >= http.StatusBadRequest && code < http.StatusInternalServerError:
		return httpStatusClientError
	default:
		return httpStatusOther
	}
}

func statusLevelForHTTPStatus(code int) statusLevel {
	switch classifyHTTPStatus(code) {
	case httpStatusSuccess:
		return statusSuccess
	case httpStatusClientError:
		return statusWarn
	default:
		return statusError
	}
}

func maxStatusLevel(a, b statusLevel) statusLevel {
	if statusLevelSeverity(b) > statusLevelSeverity(a) {
		return b
	}
	return a
}

func statusLevelSeverity(level statusLevel) int {
	switch level {
	case statusError:
		return 3
	case statusWarn:
		return 2
	default:
		return 1
	}
}

package scriptapi

import "time"

type TestResult struct {
	Name    string
	Message string
	Passed  bool
	Elapsed time.Duration
}

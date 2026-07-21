package mock

import (
	"context"
	"errors"
	"fmt"
)

var ErrInspectorUnavailable = errors.New("mock request inspector is unavailable")

type Inspector interface {
	Count(context.Context, RequestPattern) (uint64, error)
}

type Expectation struct {
	Pattern RequestPattern
	Calls   uint64
	Source  string
	Line    int
	Title   string
}

func (e Expectation) Label() string {
	label := fmt.Sprintf("%s:%d", e.Source, e.Line)
	if e.Title != "" {
		label += " " + e.Title
	}
	return label
}

type VerificationResult struct {
	Expectation Expectation
	Actual      uint64
	Err         error
	Passed      bool
}

func (r VerificationResult) Detail() string {
	switch {
	case r.Err != nil:
		return r.Err.Error()
	case r.Passed:
		return fmt.Sprintf("%d call(s)", r.Actual)
	default:
		return fmt.Sprintf("expected %d call(s), received %d", r.Expectation.Calls, r.Actual)
	}
}

func Verify(
	ctx context.Context,
	inspector Inspector,
	expectations []Expectation,
) []VerificationResult {
	results := make([]VerificationResult, 0, len(expectations))
	for _, expectation := range expectations {
		result := VerificationResult{Expectation: expectation}
		result.Actual, result.Err = inspector.Count(ctx, expectation.Pattern)
		result.Passed = result.Err == nil && result.Actual == expectation.Calls
		results = append(results, result)
	}
	return results
}

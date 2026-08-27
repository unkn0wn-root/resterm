package exec

import (
	"context"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

func TestRunHTTPPollsUntilPredicateMatches(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		statuses := []string{"pending", "pending", "completed"}
		attempt := 0
		client := repeatTestClient(func(req *http.Request) (*http.Response, error) {
			status := statuses[attempt]
			attempt++
			return repeatTestResponse(req, http.StatusOK, `{"status":"`+status+`"}`, nil), nil
		})
		captures := 0
		start := time.Now()
		result := Runner{Hooks: HTTPHooks{
			EvaluatePredicate: func(in PredicateInput) (bool, error) {
				return strings.Contains(string(in.HTTP.Body), `"completed"`), nil
			},
			ApplyCaptures: func(CaptureInput) error {
				captures++
				return nil
			},
		}}.RunHTTP(HTTPInput{
			Client:  client,
			Context: t.Context(),
			Req: &restfile.Request{
				Method: "GET",
				URL:    "https://example.com/jobs/123",
				Metadata: restfile.RequestMetadata{Poll: &restfile.PollSpec{
					Every:   500 * time.Millisecond,
					Timeout: 5 * time.Second,
					Until:   restfile.ResponsePredicate{Expression: "completed"},
				}},
			},
			EffectiveTimeout: time.Second,
		})
		if result.Err != nil {
			t.Fatalf("RunHTTP() error = %v", result.Err)
		}
		if result.Repeat.Attempts != 3 || result.Repeat.Polls != 3 || attempt != 3 {
			t.Fatalf("attempt counts = result %+v, transport %d", result.Repeat, attempt)
		}
		if captures != 1 {
			t.Fatalf("capture calls = %d, want 1", captures)
		}
		if got := time.Since(start); got != time.Second {
			t.Fatalf("poll duration = %s, want 1s", got)
		}
		if !strings.Contains(result.Decision, "3 attempts across 3 polling cycles") {
			t.Fatalf("decision = %q", result.Decision)
		}
	})
}

func TestRunHTTPRetriesTransientAttemptErrors(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		attempts := 0
		client := repeatTestClient(func(req *http.Request) (*http.Response, error) {
			attempts++
			if attempts < 3 {
				return nil, &net.DNSError{Err: "temporary lookup failure", Name: "example.com", IsTemporary: true}
			}
			return repeatTestResponse(req, http.StatusOK, "ok", nil), nil
		})
		start := time.Now()
		result := Runner{}.RunHTTP(HTTPInput{
			Client:  client,
			Context: t.Context(),
			Req: repeatTestRequest(&restfile.RetrySpec{
				Count: 3,
				Backoff: restfile.ExponentialBackoffSpec{
					Initial: 100 * time.Millisecond,
					Max:     time.Second,
				},
			}),
			EffectiveTimeout: time.Second,
		})
		if result.Err != nil {
			t.Fatalf("RunHTTP() error = %v", result.Err)
		}
		if attempts != 3 || result.Repeat.Attempts != 3 {
			t.Fatalf("attempts = %d/%d, want 3", attempts, result.Repeat.Attempts)
		}
		if got := time.Since(start); got != 300*time.Millisecond {
			t.Fatalf("retry duration = %s, want 300ms", got)
		}
	})
}

func TestRunHTTPResponseRetryHonorsRetryAfter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		attempts := 0
		client := repeatTestClient(func(req *http.Request) (*http.Response, error) {
			attempts++
			if attempts == 1 {
				return repeatTestResponse(
					req,
					http.StatusServiceUnavailable,
					"busy",
					http.Header{"Retry-After": {"1"}},
				), nil
			}
			return repeatTestResponse(req, http.StatusOK, "ok", nil), nil
		})
		start := time.Now()
		result := Runner{Hooks: HTTPHooks{
			EvaluatePredicate: statusPredicate(http.StatusServiceUnavailable),
		}}.RunHTTP(HTTPInput{
			Client:  client,
			Context: t.Context(),
			Req: repeatTestRequest(&restfile.RetrySpec{
				Count: 1,
				When:  &restfile.ResponsePredicate{Expression: "retry status"},
				Backoff: restfile.ExponentialBackoffSpec{
					Initial: 100 * time.Millisecond,
					Max:     2 * time.Second,
				},
			}),
			EffectiveTimeout: time.Second,
		})
		if result.Err != nil {
			t.Fatalf("RunHTTP() error = %v", result.Err)
		}
		if got := time.Since(start); got != time.Second {
			t.Fatalf("retry duration = %s, want Retry-After delay 1s", got)
		}
	})
}

func TestRunHTTPRetryBudgetResetsForEachPoll(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		codes := []int{http.StatusServiceUnavailable, http.StatusOK, http.StatusServiceUnavailable, http.StatusOK}
		bodies := []string{"busy", "pending", "busy", "completed"}
		attempts := 0
		client := repeatTestClient(func(req *http.Request) (*http.Response, error) {
			index := attempts
			attempts++
			return repeatTestResponse(req, codes[index], bodies[index], nil), nil
		})
		retry := &restfile.RetrySpec{
			Count: 1,
			When:  &restfile.ResponsePredicate{Expression: "retry status"},
			Backoff: restfile.ExponentialBackoffSpec{
				Initial: time.Millisecond,
				Max:     time.Millisecond,
			},
		}
		req := repeatTestRequest(retry)
		req.Metadata.Poll = &restfile.PollSpec{
			Every:   time.Second,
			Timeout: 5 * time.Second,
			Until:   restfile.ResponsePredicate{Expression: "completed"},
		}
		result := Runner{Hooks: HTTPHooks{
			EvaluatePredicate: func(in PredicateInput) (bool, error) {
				if in.Predicate.Expression == "retry status" {
					return in.HTTP.StatusCode == http.StatusServiceUnavailable, nil
				}
				return string(in.HTTP.Body) == "completed", nil
			},
		}}.RunHTTP(HTTPInput{
			Client:           client,
			Context:          t.Context(),
			Req:              req,
			EffectiveTimeout: time.Second,
		})
		if result.Err != nil {
			t.Fatalf("RunHTTP() error = %v", result.Err)
		}
		if attempts != 4 || result.Repeat.Attempts != 4 || result.Repeat.Polls != 2 {
			t.Fatalf("attempt counts = transport %d, result %+v", attempts, result.Repeat)
		}
	})
}

func TestRunHTTPPollTimeoutKeepsLastResponseWithoutFinalizing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		attempts := 0
		captures := 0
		client := repeatTestClient(func(req *http.Request) (*http.Response, error) {
			attempts++
			return repeatTestResponse(req, http.StatusOK, "pending", nil), nil
		})
		req := repeatTestRequest(nil)
		req.Metadata.Poll = &restfile.PollSpec{
			Every:   600 * time.Millisecond,
			Timeout: time.Second,
			Until:   restfile.ResponsePredicate{Expression: "false"},
		}
		start := time.Now()
		result := Runner{Hooks: HTTPHooks{
			EvaluatePredicate: func(PredicateInput) (bool, error) { return false, nil },
			ApplyCaptures: func(CaptureInput) error {
				captures++
				return nil
			},
		}}.RunHTTP(HTTPInput{
			Client:           client,
			Context:          t.Context(),
			Req:              req,
			EffectiveTimeout: time.Second,
		})
		if result.Err == nil || diag.ClassOf(result.Err) != diag.ClassTimeout {
			t.Fatalf("RunHTTP() error = %v, want timeout", result.Err)
		}
		if result.Response == nil || string(result.Response.Body) != "pending" {
			t.Fatalf("last response = %+v", result.Response)
		}
		if captures != 0 {
			t.Fatalf("capture calls = %d, want 0", captures)
		}
		if attempts != 2 || result.Repeat.Attempts != 2 {
			t.Fatalf("attempts = %d/%d, want 2", attempts, result.Repeat.Attempts)
		}
		if got := time.Since(start); got != time.Second {
			t.Fatalf("poll timeout duration = %s, want 1s", got)
		}
	})
}

func TestRunHTTPPollTimeoutDuringAttemptKeepsLastCompletedResponse(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		attempts := 0
		client := repeatTestClient(func(req *http.Request) (*http.Response, error) {
			attempts++
			if attempts == 1 {
				return repeatTestResponse(req, http.StatusAccepted, "pending", nil), nil
			}
			<-req.Context().Done()
			return nil, req.Context().Err()
		})
		req := repeatTestRequest(nil)
		req.Metadata.Poll = &restfile.PollSpec{
			Every:   200 * time.Millisecond,
			Timeout: time.Second,
			Until:   restfile.ResponsePredicate{Expression: "false"},
		}
		result := Runner{Hooks: HTTPHooks{
			EvaluatePredicate: func(PredicateInput) (bool, error) { return false, nil },
		}}.RunHTTP(HTTPInput{
			Client:           client,
			Context:          t.Context(),
			Req:              req,
			EffectiveTimeout: 10 * time.Second,
		})
		if result.Err == nil || diag.ClassOf(result.Err) != diag.ClassTimeout {
			t.Fatalf("RunHTTP() error = %v, want timeout", result.Err)
		}
		if result.Response == nil {
			t.Fatal("last response = nil, want the completed attempt")
		}
		if got := string(result.Response.Body); got != "pending" {
			t.Fatalf("last response body = %q, want %q", got, "pending")
		}
		if result.Response.StatusCode != http.StatusAccepted {
			t.Fatalf("last response status = %d, want %d", result.Response.StatusCode, http.StatusAccepted)
		}
	})
}

func TestRunHTTPPollTimeoutWithoutCompletedAttemptHasNoResponse(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := repeatTestClient(func(req *http.Request) (*http.Response, error) {
			<-req.Context().Done()
			return nil, req.Context().Err()
		})
		req := repeatTestRequest(nil)
		req.Metadata.Poll = &restfile.PollSpec{
			Every:   200 * time.Millisecond,
			Timeout: time.Second,
			Until:   restfile.ResponsePredicate{Expression: "false"},
		}
		result := Runner{Hooks: HTTPHooks{
			EvaluatePredicate: func(PredicateInput) (bool, error) { return false, nil },
		}}.RunHTTP(HTTPInput{
			Client:           client,
			Context:          t.Context(),
			Req:              req,
			EffectiveTimeout: 10 * time.Second,
		})
		if result.Err == nil || diag.ClassOf(result.Err) != diag.ClassTimeout {
			t.Fatalf("RunHTTP() error = %v, want timeout", result.Err)
		}
		if result.Response != nil {
			t.Fatalf("response = %+v, want nil", result.Response)
		}
	})
}

func TestRunHTTPRetryWhenExhaustionIsFailedTest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := repeatTestClient(func(req *http.Request) (*http.Response, error) {
			return repeatTestResponse(req, http.StatusTooManyRequests, "busy", nil), nil
		})
		result := Runner{Hooks: HTTPHooks{
			EvaluatePredicate: statusPredicate(http.StatusTooManyRequests),
		}}.RunHTTP(HTTPInput{
			Client:  client,
			Context: t.Context(),
			Req: repeatTestRequest(&restfile.RetrySpec{
				Count: 2,
				When:  &restfile.ResponsePredicate{Expression: "response.statusCode == 429"},
				Backoff: restfile.ExponentialBackoffSpec{
					Initial: time.Millisecond,
					Max:     time.Millisecond,
				},
			}),
			EffectiveTimeout: time.Second,
		})
		if result.Err != nil {
			t.Fatalf("RunHTTP() error = %v", result.Err)
		}
		if len(result.Tests) != 1 || result.Tests[0].Passed ||
			!strings.Contains(result.Tests[0].Message, "3 attempts") {
			t.Fatalf("retry exhaustion tests = %+v", result.Tests)
		}
	})
}

func TestRunHTTPDoesNotRetryStatusesOrAssertionsByDefault(t *testing.T) {
	t.Parallel()
	attempts := 0
	client := repeatTestClient(func(req *http.Request) (*http.Response, error) {
		attempts++
		return repeatTestResponse(req, http.StatusServiceUnavailable, "busy", nil), nil
	})
	result := Runner{Hooks: HTTPHooks{
		RunAsserts: func(AssertInput) ([]scripts.TestResult, error) {
			return []scripts.TestResult{{Name: "failed assertion", Passed: false}}, nil
		},
	}}.RunHTTP(HTTPInput{
		Client:  client,
		Context: t.Context(),
		Req: repeatTestRequest(&restfile.RetrySpec{
			Count: 3,
			Backoff: restfile.ExponentialBackoffSpec{
				Initial: time.Millisecond,
				Max:     time.Millisecond,
			},
		}),
		EffectiveTimeout: time.Second,
	})
	if result.Err != nil {
		t.Fatalf("RunHTTP() error = %v", result.Err)
	}
	if attempts != 1 || result.Repeat.Attempts != 1 {
		t.Fatalf("attempts = %d/%d, want 1", attempts, result.Repeat.Attempts)
	}
	if len(result.Tests) != 1 || result.Tests[0].Passed {
		t.Fatalf("assertion results = %+v", result.Tests)
	}
}

func TestRunHTTPPerAttemptTimeoutIsRetryable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		attempts := 0
		client := repeatTestClient(func(req *http.Request) (*http.Response, error) {
			attempts++
			<-req.Context().Done()
			return nil, context.Cause(req.Context())
		})
		start := time.Now()
		result := Runner{}.RunHTTP(HTTPInput{
			Client:  client,
			Context: t.Context(),
			Req: repeatTestRequest(&restfile.RetrySpec{
				Count: 2,
				Backoff: restfile.ExponentialBackoffSpec{
					Initial: time.Millisecond,
					Max:     time.Millisecond,
				},
			}),
			EffectiveTimeout: 10 * time.Millisecond,
		})
		if result.Err == nil || diag.ClassOf(result.Err) != diag.ClassTimeout {
			t.Fatalf("RunHTTP() error = %v, want timeout", result.Err)
		}
		if attempts != 3 || result.Repeat.Attempts != 3 {
			t.Fatalf("attempts = %d/%d, want 3", attempts, result.Repeat.Attempts)
		}
		if got := time.Since(start); got != 32*time.Millisecond {
			t.Fatalf("total duration = %s, want 32ms", got)
		}
	})
}

func TestRunHTTPCancellationInterruptsRetryWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		attempts := 0
		client := repeatTestClient(func(req *http.Request) (*http.Response, error) {
			attempts++
			return nil, &net.DNSError{Err: "temporary lookup failure", Name: "example.com", IsTemporary: true}
		})
		result := Runner{Hooks: HTTPHooks{
			OnRepeatProgress: func(progress RepeatProgress) {
				if progress.Phase == RepeatRetryWait {
					cancel()
				}
			},
		}}.RunHTTP(HTTPInput{
			Client:  client,
			Context: ctx,
			Req: repeatTestRequest(&restfile.RetrySpec{
				Count: 3,
				Backoff: restfile.ExponentialBackoffSpec{
					Initial: time.Second,
					Max:     time.Second,
				},
			}),
			EffectiveTimeout: time.Second,
		})
		if !errors.Is(result.Err, context.Canceled) {
			t.Fatalf("RunHTTP() error = %v, want context cancellation", result.Err)
		}
		if attempts != 1 {
			t.Fatalf("attempts = %d, want 1", attempts)
		}
	})
}

func TestInvalidRetryAfterWarnsOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		warnings := 0
		client := repeatTestClient(func(req *http.Request) (*http.Response, error) {
			return repeatTestResponse(
				req,
				http.StatusServiceUnavailable,
				"busy",
				http.Header{"Retry-After": {"later"}},
			), nil
		})
		result := Runner{Hooks: HTTPHooks{
			EvaluatePredicate: statusPredicate(http.StatusServiceUnavailable),
			Warn:              func(string) { warnings++ },
		}}.RunHTTP(HTTPInput{
			Client:  client,
			Context: t.Context(),
			Req: repeatTestRequest(&restfile.RetrySpec{
				Count: 2,
				When:  &restfile.ResponsePredicate{Expression: "retry"},
				Backoff: restfile.ExponentialBackoffSpec{
					Initial: time.Millisecond,
					Max:     time.Millisecond,
				},
			}),
			EffectiveTimeout: time.Second,
		})
		if result.Err != nil {
			t.Fatalf("RunHTTP() error = %v", result.Err)
		}
		if warnings != 1 {
			t.Fatalf("warnings = %d, want 1", warnings)
		}
	})
}

func TestRunHTTPFinalizesOutsideThePollDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bodies := []string{"pending", "completed"}
		attempt := 0
		client := repeatTestClient(func(req *http.Request) (*http.Response, error) {
			body := bodies[min(attempt, len(bodies)-1)]
			attempt++
			return repeatTestResponse(req, http.StatusOK, body, nil), nil
		})
		req := repeatTestRequest(nil)
		req.Metadata.Poll = &restfile.PollSpec{
			Every:   100 * time.Millisecond,
			Timeout: time.Second,
			Until:   restfile.ResponsePredicate{Expression: "completed"},
		}
		var assertCtxErr error
		result := Runner{Hooks: HTTPHooks{
			EvaluatePredicate: func(in PredicateInput) (bool, error) {
				return string(in.HTTP.Body) == "completed", nil
			},
			RunAsserts: func(in AssertInput) ([]scripts.TestResult, error) {
				time.Sleep(2 * time.Second)
				assertCtxErr = in.Context.Err()
				return nil, nil
			},
		}}.RunHTTP(HTTPInput{
			Client:           client,
			Context:          t.Context(),
			Req:              req,
			EffectiveTimeout: time.Second,
		})
		if result.Err != nil {
			t.Fatalf("RunHTTP() error = %v", result.Err)
		}
		if assertCtxErr != nil {
			t.Fatalf("assert context error = %v, want none", assertCtxErr)
		}
	})
}

func TestRunHTTPDecisionMatchesAPlainRequestUntilItRepeats(t *testing.T) {
	t.Parallel()
	response := func(req *http.Request) (*http.Response, error) {
		return repeatTestResponse(req, http.StatusOK, "ok", nil), nil
	}
	plain := Runner{}.RunHTTP(HTTPInput{
		Client:           repeatTestClient(response),
		Context:          t.Context(),
		Req:              repeatTestRequest(nil),
		EffectiveTimeout: time.Second,
	})
	if plain.Repeat != (RepeatCount{Attempts: 1, Polls: 1}) {
		t.Fatalf("plain request counts = %+v", plain.Repeat)
	}

	retried := Runner{}.RunHTTP(HTTPInput{
		Client:  repeatTestClient(response),
		Context: t.Context(),
		Req: repeatTestRequest(&restfile.RetrySpec{
			Count: 2,
			Backoff: restfile.ExponentialBackoffSpec{
				Initial: time.Millisecond,
				Max:     time.Millisecond,
			},
		}),
		EffectiveTimeout: time.Second,
	})
	if retried.Err != nil {
		t.Fatalf("RunHTTP() error = %v", retried.Err)
	}
	if retried.Repeat != plain.Repeat || retried.Decision != plain.Decision {
		t.Fatalf(
			"first attempt success reported as %q %+v, plain request as %q %+v",
			retried.Decision,
			retried.Repeat,
			plain.Decision,
			plain.Repeat,
		)
	}
}

func TestRunHTTPRejectsRepeatOnStreamingProtocols(t *testing.T) {
	t.Parallel()
	req := repeatTestRequest(&restfile.RetrySpec{Count: 1})
	req.SSE = &restfile.SSERequest{}
	result := Runner{}.RunHTTP(HTTPInput{
		Client:           httpx.NewClient(nil),
		Context:          t.Context(),
		Req:              req,
		EffectiveTimeout: time.Second,
	})
	if result.Err == nil || !strings.Contains(result.Err.Error(), "not supported for SSE requests") {
		t.Fatalf("RunHTTP() error = %v", result.Err)
	}
}

func TestRunHTTPTerminatesForAnyRetryCount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		count        int
		recoverAfter int
		wantAttempts int
		wantErr      bool
	}{
		{name: "negative", count: -1, recoverAfter: 99, wantAttempts: 1, wantErr: true},
		{name: "zero", count: 0, recoverAfter: 99, wantAttempts: 1, wantErr: true},
		{name: "one", count: 1, recoverAfter: 99, wantAttempts: 2, wantErr: true},
		{name: "max int", count: math.MaxInt, recoverAfter: 2, wantAttempts: 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			attempts := 0
			client := repeatTestClient(func(req *http.Request) (*http.Response, error) {
				attempts++
				if attempts > test.recoverAfter {
					return repeatTestResponse(req, http.StatusOK, "ok", nil), nil
				}
				return nil, &net.DNSError{
					Err:         "temporary lookup failure",
					Name:        "example.com",
					IsTemporary: true,
				}
			})
			result := Runner{}.RunHTTP(HTTPInput{
				Client:  client,
				Context: t.Context(),
				Req: repeatTestRequest(&restfile.RetrySpec{
					Count: test.count,
					Backoff: restfile.ExponentialBackoffSpec{
						Initial: time.Microsecond,
						Max:     time.Microsecond,
					},
				}),
				EffectiveTimeout: time.Second,
			})
			if (result.Err != nil) != test.wantErr {
				t.Fatalf("RunHTTP() error = %v, wantErr %v", result.Err, test.wantErr)
			}
			if attempts != test.wantAttempts || result.Repeat.Attempts != test.wantAttempts {
				t.Fatalf(
					"attempts = transport %d, result %d; want %d",
					attempts,
					result.Repeat.Attempts,
					test.wantAttempts,
				)
			}
		})
	}
}

func TestRunHTTPHugeRetryCountStopsOnCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		attempts := 0
		client := repeatTestClient(func(*http.Request) (*http.Response, error) {
			attempts++
			return nil, &net.DNSError{
				Err:         "temporary lookup failure",
				Name:        "example.com",
				IsTemporary: true,
			}
		})
		result := Runner{Hooks: HTTPHooks{
			OnRepeatProgress: func(progress RepeatProgress) {
				if progress.Phase == RepeatRetryWait {
					cancel()
				}
			},
		}}.RunHTTP(HTTPInput{
			Client:  client,
			Context: ctx,
			Req: repeatTestRequest(&restfile.RetrySpec{
				Count: math.MaxInt,
				Backoff: restfile.ExponentialBackoffSpec{
					Initial: time.Second,
					Max:     time.Second,
				},
			}),
			EffectiveTimeout: time.Second,
		})
		if !errors.Is(result.Err, context.Canceled) {
			t.Fatalf("RunHTTP() error = %v, want context cancellation", result.Err)
		}
		if attempts != 1 {
			t.Fatalf("attempts = %d, want 1", attempts)
		}
	})
}

func repeatTestClient(roundTrip transportFunc) *httpx.Client {
	return newHTTPClientWithFactory(func(httpx.Options) (*http.Client, error) {
		return &http.Client{Transport: roundTrip}, nil
	})
}

func repeatTestResponse(req *http.Request, status int, body string, header http.Header) *http.Response {
	if header == nil {
		header = make(http.Header)
	}
	return &http.Response{
		Status:        http.StatusText(status),
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		Header:        header,
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
}

func repeatTestRequest(retry *restfile.RetrySpec) *restfile.Request {
	return &restfile.Request{
		Method:   "GET",
		URL:      "https://example.com/jobs/123",
		Metadata: restfile.RequestMetadata{Retry: retry},
	}
}

func statusPredicate(status int) func(PredicateInput) (bool, error) {
	return func(in PredicateInput) (bool, error) {
		if in.HTTP == nil {
			return false, errors.New("response is nil")
		}
		return in.HTTP.StatusCode == status, nil
	}
}

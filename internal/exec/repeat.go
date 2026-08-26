package exec

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/delay"
	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

// ErrPollTimeout is returned when @poll reaches its timeout.
// The result may still contain the last response.
var ErrPollTimeout = errors.New("poll deadline exceeded")

// Each polling cycle gets a new retry budget.
type repeatRun struct {
	runner Runner
	in     HTTPInput
	plan   *httpx.AttemptPlan
	poll   *restfile.PollSpec
	retry  *restfile.RetrySpec

	counts RepeatCount
	warned bool
}

type cycle struct {
	resp      *httpx.Response
	at        time.Time
	exhausted bool
}

func (r Runner) runRepeatedHTTP(ctx context.Context, in HTTPInput) HTTPResult {
	if err := checkRepeatPolicy(in.Req); err != nil {
		return HTTPResult{Err: err, Decision: "HTTP repeat policy is invalid"}
	}
	plan, err := in.Client.PrepareAttempts(ctx, in.Req, in.Resolver, in.Options)
	if err != nil {
		return HTTPResult{Err: err, Decision: "HTTP request preparation failed"}
	}
	x := &repeatRun{
		runner: r,
		in:     in,
		plan:   plan,
		poll:   in.Req.Metadata.Poll,
		retry:  in.Req.Metadata.Retry,
	}
	return x.run(ctx)
}

// Requests can be built without the parser, so check these rules here too.
func checkRepeatPolicy(req *restfile.Request) error {
	if proto := req.RepeatUnsupported(); proto != "" {
		return diag.Newf(
			diag.ClassProtocol,
			"@poll and @retry are not supported for %s requests",
			proto,
		)
	}
	if req.Metadata.Profile != nil {
		return diag.New(diag.ClassConfig, "@poll and @retry cannot be combined with @profile")
	}
	poll := req.Metadata.Poll
	switch {
	case poll == nil:
		return nil
	case strings.TrimSpace(poll.Until.Expression) == "":
		return diag.New(diag.ClassConfig, "@poll until expression is empty")
	case poll.Every <= 0 || poll.Timeout <= 0:
		return diag.New(diag.ClassConfig, "@poll every and timeout must be positive")
	}
	return nil
}

// Use the polling timeout for attempts and waits. Use the caller's context for
// final response processing so it can finish after polling stops.
func (x *repeatRun) run(ctx context.Context) HTTPResult {
	pollCtx := ctx
	if x.poll != nil {
		var cancel context.CancelFunc
		pollCtx, cancel = context.WithTimeoutCause(ctx, x.poll.Timeout, ErrPollTimeout)
		defer cancel()
	}

	for x.counts.Polls = 1; ; x.counts.Polls++ {
		cyc, err := x.retryCycle(pollCtx)
		switch {
		case err != nil:
			return x.failed(cyc.resp, err)
		case cyc.exhausted:
			return x.exhausted(ctx, cyc.resp)
		case x.poll == nil:
			return x.finish(ctx, cyc.resp, decisionSent)
		}

		matched, err := x.evaluate(pollCtx, cyc.resp, x.poll.Until)
		if err != nil {
			return x.failed(cyc.resp, err)
		}
		if matched {
			return x.finish(ctx, cyc.resp, decisionSent)
		}
		if err := x.pollWait(pollCtx, cyc); err != nil {
			return x.failed(cyc.resp, err)
		}
	}
}

// Count the retry budget down so a large count cannot overflow.
func (x *repeatRun) retryCycle(ctx context.Context) (cycle, error) {
	left := 0
	if x.retry != nil {
		left = x.retry.Count
	}

	for attempt := 1; ; attempt++ {
		x.counts.Attempts++
		resp, err := x.send(ctx)
		at := time.Now()
		x.progress(RepeatAttempt, resp, 0, err)

		if ctx.Err() != nil {
			return cycle{resp: resp}, context.Cause(ctx)
		}
		if err != nil {
			if left <= 0 || !retriable(err) {
				return cycle{resp: resp}, err
			}
			left--
			if waitErr := x.retryWait(ctx, attempt, nil); waitErr != nil {
				return cycle{resp: resp}, waitErr
			}
			continue
		}
		if x.retry == nil || x.retry.When == nil {
			return cycle{resp: resp, at: at}, nil
		}

		again, err := x.evaluate(ctx, resp, *x.retry.When)
		switch {
		case err != nil:
			return cycle{resp: resp}, err
		case !again:
			return cycle{resp: resp, at: at}, nil
		case left <= 0:
			return cycle{resp: resp, at: at, exhausted: true}, nil
		}
		left--
		if waitErr := x.retryWait(ctx, attempt, resp); waitErr != nil {
			return cycle{resp: resp}, waitErr
		}
	}
}

// Each attempt gets its own timeout within the polling timeout.
func (x *repeatRun) send(ctx context.Context) (*httpx.Response, error) {
	ctx, cancel := context.WithTimeoutCause(
		ctx,
		x.in.EffectiveTimeout,
		fmt.Errorf(
			"HTTP attempt timed out after %s: %w",
			x.in.EffectiveTimeout,
			context.DeadlineExceeded,
		),
	)
	defer cancel()
	return x.plan.Execute(ctx)
}

// Measure the polling interval from the previous response.
func (x *repeatRun) pollWait(ctx context.Context, cyc cycle) error {
	wait := max(time.Until(cyc.at.Add(x.poll.Every)), 0)
	x.progress(RepeatPollWait, cyc.resp, wait, nil)
	return delay.Wait(ctx, wait)
}

func (x *repeatRun) retryWait(ctx context.Context, attempt int, resp *httpx.Response) error {
	wait := retryBackoff(x.retry.Backoff, attempt)
	if resp != nil {
		switch server, state := retryAfter(resp.Headers, time.Now()); state {
		case retryAfterValid:
			wait = max(wait, server)
		case retryAfterInvalid:
			x.warnOnce("invalid Retry-After header ignored")
		}
	}
	x.progress(RepeatRetryWait, resp, wait, nil)
	return delay.Wait(ctx, wait)
}

func (x *repeatRun) evaluate(
	ctx context.Context,
	resp *httpx.Response,
	pred restfile.ResponsePredicate,
) (bool, error) {
	if x.runner.Hooks.EvaluatePredicate == nil {
		return false, diag.New(
			diag.ClassInternal,
			"HTTP response predicate evaluator is not configured",
		)
	}
	ok, err := x.runner.Hooks.EvaluatePredicate(PredicateInput{
		Context:   ctx,
		Doc:       x.in.Doc,
		Req:       x.in.Req,
		BaseDir:   x.in.Options.BaseDir,
		Vars:      x.runner.collectVars(x.in.Doc, x.in.Req, x.in.ScriptVars),
		Locals:    x.in.Locals,
		HTTP:      resp,
		Predicate: pred,
	})
	// If evaluation hit the deadline, return the context error instead.
	if ctx.Err() != nil {
		return false, context.Cause(ctx)
	}
	return ok, err
}

func (x *repeatRun) finish(
	ctx context.Context,
	resp *httpx.Response,
	decision string,
) HTTPResult {
	res := x.runner.finalizeHTTP(ctx, x.in, resp)
	res.Repeat = x.counts
	res.Decision = x.counts.describe(decision)
	return res
}

// A response that still matches @retry-when is returned with a failed test.
func (x *repeatRun) exhausted(ctx context.Context, resp *httpx.Response) HTTPResult {
	res := x.finish(ctx, resp, "HTTP retries exhausted")
	if x.retry.When != nil && res.Err == nil {
		res.Tests = append(res.Tests, scripts.TestResult{
			Name:    directive.RetryWhen.Tag() + " " + str.FoldLines(x.retry.When.Expression),
			Message: fmt.Sprintf("condition remained true after %d attempts", x.retry.Count+1),
			Passed:  false,
		})
	}
	return res
}

func (x *repeatRun) failed(resp *httpx.Response, err error) HTTPResult {
	switch {
	case errors.Is(err, ErrPollTimeout):
		err = diag.WrapAs(
			diag.ClassTimeout,
			fmt.Errorf(
				"%w after %s (%d attempts across %d polling cycles)",
				ErrPollTimeout,
				x.poll.Timeout,
				x.counts.Attempts,
				x.counts.Polls,
			),
			"wait for poll condition",
		)
	case err != nil && diag.ClassOf(err) == diag.ClassUnknown:
		err = diag.Wrap(err, "repeat HTTP request")
	}
	return HTTPResult{
		Response: resp,
		Err:      err,
		Decision: x.counts.describe("HTTP request failed"),
		Repeat:   x.counts,
	}
}

func (x *repeatRun) progress(
	phase RepeatPhase,
	resp *httpx.Response,
	wait time.Duration,
	err error,
) {
	if x.runner.Hooks.OnRepeatProgress == nil {
		return
	}
	x.runner.Hooks.OnRepeatProgress(RepeatProgress{
		Phase:      phase,
		Attempt:    x.counts.Attempts,
		Poll:       x.counts.Polls,
		StatusCode: responseStatus(resp),
		Delay:      wait,
		Err:        err,
	})
}

func (x *repeatRun) warnOnce(message string) {
	if x.warned {
		return
	}
	x.warned = true
	if x.runner.Hooks.Warn != nil {
		x.runner.Hooks.Warn(message)
	}
}

func responseStatus(resp *httpx.Response) int {
	if resp == nil {
		return 0
	}
	return resp.StatusCode
}

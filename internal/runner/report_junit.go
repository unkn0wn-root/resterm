package runner

import (
	"encoding/xml"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/scripts"
)

type junitSuites struct {
	XMLName  xml.Name     `xml:"testsuites"`
	Tests    int          `xml:"tests,attr"`
	Failures int          `xml:"failures,attr"`
	Skipped  int          `xml:"skipped,attr"`
	Time     string       `xml:"time,attr,omitempty"`
	Suites   []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	Name      string      `xml:"name,attr"`
	Tests     int         `xml:"tests,attr"`
	Failures  int         `xml:"failures,attr"`
	Skipped   int         `xml:"skipped,attr"`
	Time      string      `xml:"time,attr,omitempty"`
	TestCases []junitCase `xml:"testcase"`
	SystemOut string      `xml:"system-out,omitempty"`
}

type junitCase struct {
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr,omitempty"`
	Time      string        `xml:"time,attr,omitempty"`
	Failure   *junitFailure `xml:"failure,omitempty"`
	Skipped   *junitSkipped `xml:"skipped,omitempty"`
	SystemOut string        `xml:"system-out,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr,omitempty"`
	Body    string `xml:",chardata"`
}

type junitSkipped struct {
	Message string `xml:"message,attr,omitempty"`
}

func (r *Report) WriteJUnit(w io.Writer) error {
	if w == nil {
		return ErrNilWriter
	}
	_, _ = io.WriteString(w, xml.Header)
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	return enc.Encode(r.junit())
}

func (r *Report) junit() junitSuites {
	out := junitSuites{
		Time:   junitTime(r.Duration),
		Suites: make([]junitSuite, 0, len(r.Results)),
	}
	for _, item := range r.Results {
		suite := item.junitSuite()
		out.Suites = append(out.Suites, suite)
		out.Tests += suite.Tests
		out.Failures += suite.Failures
		out.Skipped += suite.Skipped
	}
	return out
}

func (item Result) junitSuite() junitSuite {
	cases := item.junitCases()
	out := junitSuite{
		Name:      suiteName(item),
		Tests:     len(cases),
		Time:      junitTime(resultDuration(item)),
		TestCases: cases,
		SystemOut: item.Summary,
	}
	for _, tc := range cases {
		switch {
		case tc.Skipped != nil:
			out.Skipped++
		case tc.Failure != nil:
			out.Failures++
		}
	}
	return out
}

func (item Result) junitCases() []junitCase {
	if len(item.Steps) == 0 {
		return []junitCase{item.junitCase()}
	}
	out := make([]junitCase, 0, len(item.Steps))
	for _, step := range item.Steps {
		out = append(out, item.stepJUnitCase(step))
	}
	return out
}

func (item Result) junitCase() junitCase {
	tc := junitCase{
		Name:      resultName(item),
		ClassName: suiteName(item),
		Time:      junitTime(resultDuration(item)),
		SystemOut: resultLine(item),
	}
	if item.Skipped {
		tc.Skipped = &junitSkipped{Message: skipMessage(item.SkipReason, item.Summary)}
		return tc
	}
	if msg := resultFailureMessage(item); msg != "" && resultFailed(item) {
		tc.Failure = &junitFailure{Message: msg, Body: msg}
	}
	return tc
}

func (item Result) stepJUnitCase(step StepResult) junitCase {
	tc := junitCase{
		Name:      stepName(step),
		ClassName: suiteName(item),
		Time:      junitTime(step.Duration),
		SystemOut: stepLine(step),
	}
	if step.Skipped {
		tc.Skipped = &junitSkipped{Message: skipMessage(step.SkipReason, step.Summary)}
		return tc
	}
	if msg := stepFailureMessage(step); msg != "" && (step.Canceled || !step.Passed) {
		tc.Failure = &junitFailure{Message: msg, Body: msg}
	}
	return tc
}

func suiteName(item Result) string {
	return requestMethodValue(item.Method) + " " + resultName(item)
}

func resultFailureMessage(item Result) string {
	switch {
	case item.Err != nil:
		return item.Err.Error()
	case item.ScriptErr != nil:
		return item.ScriptErr.Error()
	}
	if failed := failedTests(item.Tests); len(failed) > 0 {
		return testFailureMessage(failed)
	}
	if msg := traceFailureText(item.Trace); msg != "" && resultFailed(item) {
		return msg
	}
	if msg := item.Summary; msg != "" && resultFailed(item) {
		return msg
	}
	if status := resultStatus(item); status != "" && resultFailed(item) {
		return status
	}
	return ""
}

func stepFailureMessage(step StepResult) string {
	switch {
	case step.Err != nil:
		return step.Err.Error()
	case step.ScriptErr != nil:
		return step.ScriptErr.Error()
	case step.Canceled:
		if msg := step.Summary; msg != "" {
			return msg
		}
		return "canceled"
	}
	if failed := failedTests(step.Tests); len(failed) > 0 {
		return testFailureMessage(failed)
	}
	if msg := traceFailureText(step.Trace); msg != "" && stepFailed(step) {
		return msg
	}
	if msg := step.Summary; msg != "" && !step.Passed {
		return msg
	}
	if status := stepStatus(step); status != "" && !step.Passed {
		return status
	}
	return ""
}

func testFailureMessage(tests []scripts.TestResult) string {
	if len(tests) == 0 {
		return ""
	}
	first := tests[0]
	name := first.Name
	msg := first.Message
	switch {
	case name != "" && msg != "":
		return name + ": " + msg
	case name != "":
		return name
	case msg != "":
		return msg
	default:
		return "test failed"
	}
}

func skipMessage(parts ...string) string {
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			return trimmed
		}
	}
	return "skipped"
}

func junitTime(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return strconv.FormatFloat(d.Seconds(), 'f', 3, 64)
}

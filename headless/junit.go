package headless

import (
	"encoding/xml"
	"io"
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

// WriteJUnit writes r as JUnit XML.
// If r is nil, WriteJUnit is a no-op. If w is nil, WriteJUnit returns ErrNilWriter.
func (r *Report) WriteJUnit(w io.Writer) error {
	if r == nil {
		return nil
	}
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
		Time:      junitTime(item.Duration),
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
		Time:      junitTime(item.Duration),
		SystemOut: resultLine(item),
	}
	if item.Status == StatusSkip {
		tc.Skipped = &junitSkipped{Message: skipMessage(item.SkipReason, item.Summary)}
		return tc
	}
	if msg := resultFailureMessage(item); msg != "" && item.Failed() {
		tc.Failure = &junitFailure{Message: msg, Body: msg}
	}
	return tc
}

func (item Result) stepJUnitCase(step Step) junitCase {
	tc := junitCase{
		Name:      stepName(step),
		ClassName: suiteName(item),
		Time:      junitTime(step.Duration),
		SystemOut: stepLine(step),
	}
	if step.Status == StatusSkip {
		tc.Skipped = &junitSkipped{Message: skipMessage(step.SkipReason, step.Summary)}
		return tc
	}
	if msg := stepFailureMessage(step); msg != "" && (step.Canceled || step.Failed()) {
		tc.Failure = &junitFailure{Message: msg, Body: msg}
	}
	return tc
}

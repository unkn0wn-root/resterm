package runfmt

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

func WriteJUnit(w io.Writer, rep *Report) error {
	_, _ = io.WriteString(w, xml.Header)
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	return enc.Encode(rep.junit())
}

func (rep Report) junit() junitSuites {
	out := junitSuites{
		Time:   junitTime(rep.Duration),
		Suites: make([]junitSuite, 0, len(rep.Results)),
	}
	for _, res := range rep.Results {
		suite := res.junitSuite()
		out.Suites = append(out.Suites, suite)
		out.Tests += suite.Tests
		out.Failures += suite.Failures
		out.Skipped += suite.Skipped
	}
	return out
}

func (res Result) junitSuite() junitSuite {
	cases := res.junitCases()
	out := junitSuite{
		Name:      suiteName(res),
		Tests:     len(cases),
		Time:      junitTime(res.Duration),
		TestCases: cases,
		SystemOut: res.Summary,
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

func (res Result) junitCases() []junitCase {
	if len(res.Steps) == 0 {
		return []junitCase{res.junitCase()}
	}
	out := make([]junitCase, 0, len(res.Steps))
	for _, step := range res.Steps {
		out = append(out, res.stepJUnitCase(step))
	}
	return out
}

func (res Result) junitCase() junitCase {
	tc := junitCase{
		Name:      resultName(res),
		ClassName: suiteName(res),
		Time:      junitTime(res.Duration),
		SystemOut: junitSystemOut(resultLine(res), res.Target, res.EffectiveTarget),
	}
	if res.Status == StatusSkip {
		tc.Skipped = &junitSkipped{Message: skipMessage(res.SkipReason, res.Summary)}
		return tc
	}
	if msg := resultFailureMessage(res); msg != "" && res.Status == StatusFail {
		tc.Failure = &junitFailure{Message: msg, Body: msg}
	}
	return tc
}

func (res Result) stepJUnitCase(step Step) junitCase {
	tc := junitCase{
		Name:      stepName(step),
		ClassName: suiteName(res),
		Time:      junitTime(step.Duration),
		SystemOut: junitSystemOut(stepLine(step), step.Target, step.EffectiveTarget),
	}
	if step.Status == StatusSkip {
		tc.Skipped = &junitSkipped{Message: skipMessage(step.SkipReason, step.Summary)}
		return tc
	}
	if msg := stepFailureMessage(step); msg != "" && (step.Canceled || step.Status == StatusFail) {
		tc.Failure = &junitFailure{Message: msg, Body: msg}
	}
	return tc
}

func junitSystemOut(text, target, effective string) string {
	source, resolved, ok := targetDetails(target, effective)
	if !ok {
		return text
	}
	details := "Source Target: " + source + "\nEffective Target: " + resolved
	if text == "" {
		return details
	}
	return text + "\n" + details
}

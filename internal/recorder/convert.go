package recorder

import (
	"bytes"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/mock"
	httpbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/http"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/restwriter"
)

// Leave room for the rest of @match within the parser's 1 MiB line limit.
const maxMatchBytes = 64 << 10

var directivePrefixes = []string{"#", "//", "@", "<", ">"}

func entryRequest(e Entry, base, upstream string, names map[string]struct{}) (*restfile.Request, Issue) {
	if e.Method != strings.ToUpper(e.Method) {
		return nil, issueRequestMethodCase
	}
	if !e.Request.Issue.ok() {
		return nil, e.Request.Issue
	}
	if strings.Contains(e.URL, "{{") {
		return nil, issueRequestTemplate
	}
	if _, ok, err := httpbuilder.ParseMethodLine(e.Method + " " + e.URL); !ok || err != nil ||
		e.Method == "WS" || e.Method == "WSS" {
		return nil, issueRequestMethod
	}
	for _, values := range e.Request.Headers {
		if slices.ContainsFunc(values, func(v string) bool { return strings.Contains(v, "{{") }) {
			return nil, issueHeaderTemplate
		}
	}

	u, err := url.Parse(e.URL)
	if err != nil || u.Scheme+"://"+u.Host != upstream || u.Opaque != "" {
		return nil, issueRequestURL
	}
	return &restfile.Request{
		Method:   e.Method,
		URL:      "{{" + base + "}}" + u.RequestURI(),
		Headers:  e.Request.Headers.Clone(),
		Metadata: restfile.RequestMetadata{Name: recordName(e.ID, names, "")},
	}, ""
}

func entryMock(e Entry, names map[string]struct{}) (*restfile.Mock, Issue) {
	if e.Method != strings.ToUpper(e.Method) {
		return nil, issueMockMethodCase
	}
	if !e.Response.Issue.ok() {
		return nil, e.Response.Issue
	}
	if !restfile.ValidMockStatus(e.Status) {
		return nil, issueMockStatus
	}

	u, err := url.Parse(e.URL)
	if err != nil || u.Host == "" {
		return nil, issueMockURL
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	if _, _, err := restfile.CompileMockPath(path); err != nil {
		return nil, issueMockPath
	}
	query, issue := mockQuery(u.RawQuery)
	if !issue.ok() {
		return nil, issue
	}

	name := recordName(e.ID, names, "-mock")
	m := &restfile.Mock{
		Name: name, Title: name, Method: e.Method, Path: path, DisableInterpolation: true,
		Responses: []restfile.MockResponse{{Status: e.Status, Headers: e.Response.Headers.Clone()}},
	}
	m.Match.Query = query
	if e.Request.Issue.ok() {
		// Omitting the matcher would make the mock accept more requests.
		if len(e.Request.MatchJSON) > maxMatchBytes {
			return nil, issueMockMatchLimit
		}
		m.Match.JSON = bytes.Clone(e.Request.MatchJSON)
	}
	if _, err := mock.Compile([]*restfile.Document{{Mocks: []*restfile.Mock{m}}}); err != nil {
		return nil, issueMockEngine
	}
	return m, ""
}

func recordName(id uint64, names map[string]struct{}, suffix string) string {
	return restfile.UniqueMockName(fmt.Sprintf("record-%06d%s", id, suffix), names)
}

func mockQuery(raw string) (map[string]restfile.MockQueryRule, Issue) {
	q, err := url.ParseQuery(raw)
	if err != nil {
		return nil, issueMockQuery
	}

	var rules map[string]restfile.MockQueryRule
	for name, values := range q {
		if name == "" || slices.Contains(values, redacted) {
			continue
		}
		if rules == nil {
			rules = make(map[string]restfile.MockQueryRule)
		}
		rules[name] = restfile.MockQueryRule{Op: restfile.MockOpExact, Values: values}
	}
	return rules, ""
}

// Use fixtures for data that could be parsed as Resterm syntax.
func inlinable(data []byte) bool {
	text := string(data)
	if strings.TrimSpace(text) == "" || strings.ContainsAny(text, "\r") || strings.Contains(text, "{{") {
		return false
	}
	if _, err := restwriter.CheckMockBody(text); err != nil {
		return false
	}
	for line := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimSpace(line)
		for _, prefix := range directivePrefixes {
			if strings.HasPrefix(trimmed, prefix) {
				return false
			}
		}
	}
	return true
}

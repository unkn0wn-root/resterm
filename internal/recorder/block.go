package recorder

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/restwriter"
)

// End each block so later appends cannot change how it is parsed.
const separatorLine = "###\n"

var (
	errInlineBody = errors.New("inline body does not round-trip")
	errBlock      = errors.New("block does not round-trip through the request parser")
)

// The parser removes the last newline. The writer already adds one,
// so bodies ending in a newline need another to preserve it.
func separator(body string) string {
	if strings.HasSuffix(body, "\n") {
		return "\n" + separatorLine
	}
	return separatorLine
}

func renderBlock(doc *restfile.Document) (string, error) {
	text, err := restwriter.Render(doc, restwriter.Options{})
	if err != nil {
		return "", err
	}
	text += separator(inlineBody(doc))
	if err := verifyBlock(doc, text); err != nil {
		return "", err
	}
	return text, nil
}

func inlineBody(doc *restfile.Document) string {
	var src restfile.BodySource
	switch {
	case len(doc.Mocks) > 0:
		responses := doc.Mocks[len(doc.Mocks)-1].Responses
		src = responses[len(responses)-1].Body
	case len(doc.Requests) > 0:
		src = doc.Requests[len(doc.Requests)-1].Body
	}
	if src.FilePath != "" {
		return ""
	}
	return src.Text
}

func verifyBlock(doc *restfile.Document, text string) error {
	parsed := parser.Parse(doc.Path, []byte(text))
	switch {
	case parser.Check(parsed) != nil,
		len(parsed.Requests) != len(doc.Requests),
		len(parsed.Mocks) != len(doc.Mocks):
		return errBlock
	}

	for i, want := range doc.Requests {
		got := parsed.Requests[i]
		if err := sameRequest(got, want); err != nil {
			return err
		}
		// Recorded data must not introduce executable directives.
		switch {
		case len(got.Metadata.Scripts) != 0,
			len(got.Metadata.Captures) != 0,
			got.Metadata.Auth != nil:
			return errBlock
		}
	}
	for i, want := range doc.Mocks {
		if err := sameMock(parsed.Mocks[i], want); err != nil {
			return err
		}
	}
	return nil
}

func sameRequest(got, want *restfile.Request) error {
	if err := sameBody(got.Body, want.Body); err != nil {
		return err
	}
	switch {
	case got.Method != want.Method,
		got.URL != want.URL,
		got.Metadata.Name != want.Metadata.Name,
		got.Metadata.Description != want.Metadata.Description,
		!slices.EqualFunc(got.Metadata.Asserts, want.Metadata.Asserts, sameAssert),
		!maps.EqualFunc(got.Headers, want.Headers, slices.Equal):
		return errBlock
	}
	return nil
}

func sameAssert(a, b restfile.AssertSpec) bool {
	return a.Expression == b.Expression && a.Message == b.Message
}

func sameMock(got, want *restfile.Mock) error {
	if len(got.Responses) != len(want.Responses) {
		return errBlock
	}
	for i, response := range want.Responses {
		if err := sameBody(got.Responses[i].Body, response.Body); err != nil {
			return err
		}
		switch {
		case got.Responses[i].Status != response.Status,
			!maps.EqualFunc(got.Responses[i].Headers, response.Headers, slices.Equal):
			return errBlock
		}
	}
	switch {
	case got.Name != want.Name,
		got.Method != want.Method,
		got.Path != want.Path,
		got.DisableInterpolation != want.DisableInterpolation,
		!sameMatch(got.Match, want.Match):
		return errBlock
	}
	return nil
}

// Only inline body differences can be fixed by moving the body to a fixture.
func sameBody(got, want restfile.BodySource) error {
	switch {
	case got.Text == want.Text && got.FilePath == want.FilePath:
		return nil
	case want.FilePath == "":
		return errInlineBody
	default:
		return errBlock
	}
}

func sameMatch(got, want restfile.MockMatch) bool {
	return maps.EqualFunc(got.Query, want.Query, sameRule) &&
		maps.EqualFunc(got.Headers, want.Headers, sameRule) &&
		sameJSON(got.JSON, want.JSON) && sameJSON(got.JSONRules, want.JSONRules)
}

func sameRule[R restfile.MockQueryRule | restfile.MockHeaderRule](a, b R) bool {
	x, y := restfile.MockRule(a), restfile.MockRule(b)
	return x.Op == y.Op && slices.Equal(x.Values, y.Values)
}

// The writer compacts JSON, so ignore whitespace when comparing.
func sameJSON(got, want []byte) bool {
	if len(got) == 0 || len(want) == 0 {
		return len(got) == len(want)
	}
	var a, b bytes.Buffer
	if json.Compact(&a, got) != nil || json.Compact(&b, want) != nil {
		return false
	}
	return bytes.Equal(a.Bytes(), b.Bytes())
}

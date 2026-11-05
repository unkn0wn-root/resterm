package ui

import (
	"strings"
	"testing"
)

func TestCompareHistoryResultMasksRuntimeSecrets(t *testing.T) {
	const secret = "ui-compare-runtime-secret"

	srv := remaskServer(t)
	m, store := newRemaskModel(t, `### probe
# @name probe
# @compare dev stage
# @script pre-request
> vars.global.set("token", "`+secret+`", {secret: true});
> request.setHeader("X-Trace", vars.global.get("token"));
GET `+srv.URL+`/seen?v=`+secret+`
`)
	req := m.doc.Requests[0]
	runToEnd(t, m, m.startCompareRun(m.doc, req, req.Metadata.Compare, m.runOptions()))

	es, err := store.Entries()
	if err != nil || len(es) != 1 || es[0].Compare == nil {
		t.Fatalf("entries = %+v, %v, want one compare entry", es, err)
	}
	texts := map[string]string{"URL": es[0].URL, "request text": es[0].RequestText}
	for _, res := range es[0].Compare.Results {
		texts[res.Environment+" request text"] = res.RequestText
		texts[res.Environment+" body snippet"] = res.BodySnippet
	}
	for field, text := range texts {
		if strings.Contains(text, secret) {
			t.Fatalf("%s contains the plaintext secret: %s", field, text)
		}
	}
}

package recorder

import (
	"slices"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/parser"
)

func TestExportPreservesBodies(t *testing.T) {
	var entries []Entry
	for i, body := range []string{"x", "hello\n", "a\n\n", "a\n\nb", "{\n  \"id\": 42\n}\n"} {
		entries = append(entries, textEntry(uint64(i+1), "/x", body))
	}
	for name, batchSize := range map[string]int{"batch": len(entries), "successive": 1} {
		t.Run(name, func(t *testing.T) {
			out := newTestOutput(t)
			for batch := range slices.Chunk(entries, batchSize) {
				p := buildExport(t, batch, ExportOptions{Mode: Both, Path: out.path, Existing: out.Existing()})
				if len(p.Fixtures) != 0 || len(p.Excluded) != 0 {
					t.Fatalf("plain bodies not exported inline: %+v", p)
				}
				if err := out.Append(t.Context(), p); err != nil {
					t.Fatal(err)
				}
			}
			doc := readTestOutput(t, out)
			if len(doc.Requests) != len(entries) || len(doc.Mocks) != len(entries) || len(doc.Variables) != 1 {
				t.Fatalf("wrong export counts: %+v", doc)
			}
			for i, e := range entries {
				request, response := doc.Requests[i].Body.Text, doc.Mocks[i].Responses[0].Body.Text
				if request != string(e.Request.Body) || response != string(e.Response.Body) {
					t.Fatalf("entry %d bodies changed: request %q, response %q", e.ID, request, response)
				}
			}
		})
	}
}

func TestAppendTextKeepsTheDocumentsLastBody(t *testing.T) {
	block := buildExport(t, []Entry{textEntry(1, "/y", "added\n")}, ExportOptions{Mode: Requests})
	for _, head := range []string{
		"### one\n# @name one\nPOST http://example.invalid/\nContent-Type: text/plain\n\n",
		"### m\n# @mock method=GET path=/x name=m\nHTTP/1.1 200 OK\nContent-Type: text/plain\n\n",
	} {
		for _, body := range []string{"body", "body\n", "body\n\n", "a\n\nb\n"} {
			existing := head + body
			before := parser.Parse("buffer.http", []byte(existing))
			text, err := AppendText("buffer.http", existing, block.Text)
			if err != nil {
				t.Fatalf("append to %q: %v", existing, err)
			}
			after := parser.Parse("buffer.http", []byte(text))
			if err := parser.Check(after); err != nil {
				t.Fatal(err)
			}
			if len(after.Requests) != len(before.Requests)+1 || len(after.Mocks) != len(before.Mocks) {
				t.Fatalf("append changed block counts: %s", text)
			}
			if len(before.Mocks) > 0 {
				if after.Mocks[0].Responses[0].Body.Text != before.Mocks[0].Responses[0].Body.Text {
					t.Fatalf("append changed mock body: %s", text)
				}
			} else if after.Requests[0].Body.Text != before.Requests[0].Body.Text {
				t.Fatalf("append changed request body: %s", text)
			}
		}
	}
}

package history_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/history/sqlite"
)

func TestBufferKeepsEntriesItDidNotWrite(t *testing.T) {
	const secret = "later-cap-77aa-bbcc"
	dir := t.TempDir()
	s := sqlite.New(filepath.Join(dir, "history.db"))
	t.Cleanup(func() { _ = s.Close() })
	at := time.Unix(100, 0)
	before := []history.Entry{
		{
			ID:          "1",
			ExecutedAt:  at,
			RequestName: "raw",
			URL:         "https://api.test/x?k=" + secret,
			BodySnippet: `{"t":"` + secret + `"}`,
			RequestText: "GET https://api.test/x?k=" + secret + "\n",
		},
		{
			ID:          "2",
			ExecutedAt:  at.Add(time.Second),
			RequestName: "stars",
			URL:         "https://api.test/x?k=***",
			BodySnippet: `{"t":"saw ***"}`,
			RequestText: "GET https://api.test/x\nAuthorization: ***\n",
		},
	}
	for _, e := range before {
		if err := s.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	export := func(name string) map[string]json.RawMessage {
		t.Helper()
		path := filepath.Join(dir, name)
		if _, err := s.ExportJSON(path); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var rows []json.RawMessage
		if err := json.Unmarshal(data, &rows); err != nil {
			t.Fatal(err)
		}
		out := map[string]json.RawMessage{}
		for _, r := range rows {
			var e history.Entry
			if err := json.Unmarshal(r, &e); err != nil {
				t.Fatal(err)
			}
			out[e.ID] = r
		}
		return out
	}
	stored, err := s.Entries()
	if err != nil {
		t.Fatal(err)
	}
	old := export("before.json")

	b := &history.Buffer{Store: s}
	if err := b.Append(
		history.Entry{ID: "3", ExecutedAt: at.Add(2 * time.Second), URL: "https://api.test/x?k=" + secret},
	); err != nil {
		t.Fatal(err)
	}
	if err := b.Flush(secret); err != nil {
		t.Fatal(err)
	}

	es, err := s.Entries()
	if err != nil || len(es) != 3 {
		t.Fatalf("entries = %d, %v, want 3", len(es), err)
	}
	if strings.Contains(es[0].URL, secret) {
		t.Errorf("new entry = %q, want it masked", es[0].URL)
	}
	if !reflect.DeepEqual(es[1:], stored) {
		t.Errorf("entries from before the buffer changed:\n%+v\nwant\n%+v", es[1:], stored)
	}
	now := export("after.json")
	for _, id := range []string{"1", "2"} {
		if string(now[id]) != string(old[id]) {
			t.Errorf("export of %s changed:\n%s\nwant\n%s", id, now[id], old[id])
		}
	}
}

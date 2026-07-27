package runner

import (
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestCloneDocNormalizesClonedRequests(t *testing.T) {
	src := &restfile.Document{Requests: []*restfile.Request{{
		Method: " get ",
		URL:    " https://example.com ",
	}}}

	got := cloneDoc(src)
	if got.Requests[0].Method != "GET" ||
		got.Requests[0].URL != "https://example.com" {
		t.Fatalf("request was not normalized: %#v", got.Requests[0])
	}
	if src.Requests[0].Method != " get " ||
		src.Requests[0].URL != " https://example.com " {
		t.Fatal("normalizing clone changed source document")
	}
}

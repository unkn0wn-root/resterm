package runner

import (
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestSelectByTagSkipsNilRequests(t *testing.T) {
	reqs := []*restfile.Request{
		nil,
		{Metadata: restfile.RequestMetadata{Tags: []string{"slow"}}},
	}

	got, err := selectByTag(reqs, "slow")
	if err != nil {
		t.Fatalf("selectByTag: %v", err)
	}
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("selectByTag = %v, want [1]", got)
	}
}

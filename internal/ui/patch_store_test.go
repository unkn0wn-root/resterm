package ui

import (
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestPatchStoreTracksGlobalProfiles(t *testing.T) {
	s := newPatchStore()
	xs := []restfile.PatchProfile{
		{Scope: restfile.PatchScopeGlobal, Name: "jsonApi"},
		{Scope: restfile.PatchScopeFile, Name: "skip"},
	}

	s.set("/tmp/a.http", xs)
	got := s.all()
	if len(got) != 1 {
		t.Fatalf("expected 1 cached patch, got %d", len(got))
	}
	if got[0].Name != "jsonApi" {
		t.Fatalf("unexpected patch name %q", got[0].Name)
	}

	s.set("/tmp/a.http", nil)
	if len(s.all()) != 0 {
		t.Fatalf("expected cache cleared when patch list is empty")
	}
}

package ui

import (
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestK8sStoreTracksGlobalProfiles(t *testing.T) {
	store := newK8sStore()
	profiles := []restfile.K8sProfile{
		{Scope: restfile.K8sScopeGlobal, Name: "api"},
		{Scope: restfile.K8sScopeFile, Name: "ignore"},
	}

	store.set("/tmp/a.http", profiles)
	all := store.all()
	if len(all) != 1 {
		t.Fatalf("expected 1 cached profile, got %d", len(all))
	}
	if all[0].Name != "api" {
		t.Fatalf("unexpected profile name %q", all[0].Name)
	}

	store.set("/tmp/a.http", nil)
	if len(store.all()) != 0 {
		t.Fatalf("expected cache to be cleared when profile list is empty")
	}
}

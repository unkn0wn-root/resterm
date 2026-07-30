package runtime

import "testing"

// A scope is named after the environment, not the workspace, so crossing a
// workspace boundary has to forget everything rather than clear one scope.
func TestResetSecretsForgetsEveryScope(t *testing.T) {
	rt := New(Config{})

	rt.Globals().Set("dev", "auth.token", "SECRET", true)
	rt.Globals().Set("prod", "auth.token", "OTHER", true)
	rt.Files().Set("dev", "/ws/a.http", "file.token", "SECRET", true)
	if rt.Cookies().Jar("dev") == nil {
		t.Fatal("expected a cookie jar")
	}

	rt.ResetSecrets()

	for _, scope := range []string{"dev", "prod"} {
		if got := rt.Globals().Snapshot(scope); len(got) != 0 {
			t.Fatalf("globals for %q survived: %v", scope, got)
		}
	}
	if got := rt.Files().Snapshot("dev", "/ws/a.http"); len(got) != 0 {
		t.Fatalf("file values survived: %v", got)
	}
	if got := rt.Globals().Entries(); len(got) != 0 {
		t.Fatalf("persisted globals survived: %v", got)
	}
	if got := rt.Files().Entries(); len(got) != 0 {
		t.Fatalf("persisted file values survived: %v", got)
	}
	if got := rt.AuthState().OAuth; len(got) != 0 {
		t.Fatalf("oauth tokens survived: %v", got)
	}
}

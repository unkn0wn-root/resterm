package runtime

import (
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/authcmd"
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/oauth"
)

type stubHistory struct {
	closed bool
}

func (s *stubHistory) Load() error                                { return nil }
func (s *stubHistory) Append(history.Entry) error                 { return nil }
func (s *stubHistory) Entries() ([]history.Entry, error)          { return nil, nil }
func (s *stubHistory) ByRequest(string) ([]history.Entry, error)  { return nil, nil }
func (s *stubHistory) ByWorkflow(string) ([]history.Entry, error) { return nil, nil }
func (s *stubHistory) ByFile(string) ([]history.Entry, error)     { return nil, nil }
func (s *stubHistory) Delete(string) (bool, error)                { return false, nil }
func (s *stubHistory) Close() error {
	s.closed = true
	return nil
}

func TestRuntimeStateRoundTrip(t *testing.T) {
	rt := New(Config{})
	rt.Globals().Set("dev", "token", "abc", true)
	rt.Files().Set("dev", "/tmp/a.http", "last", "200", false)

	got := rt.RuntimeState()
	if len(got.Globals) != 1 {
		t.Fatalf("expected one global entry, got %d", len(got.Globals))
	}
	if len(got.Files) != 1 {
		t.Fatalf("expected one file entry, got %d", len(got.Files))
	}

	cp := New(Config{})
	cp.LoadRuntimeState(got)

	gl := cp.Globals().Snapshot("dev")
	if len(gl) != 1 {
		t.Fatalf("expected restored globals, got %d", len(gl))
	}
	if gl["token"].Value != "abc" || !gl["token"].Secret {
		t.Fatalf("unexpected restored global: %+v", gl["token"])
	}

	fs := cp.Files().Snapshot("dev", "/tmp/a.http")
	if len(fs) != 1 {
		t.Fatalf("expected restored file vars, got %d", len(fs))
	}
	if fs["last"].Value != "200" {
		t.Fatalf("unexpected restored file entry: %+v", fs["last"])
	}
}

func TestAuthStateRoundTrip(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	st := engine.AuthState{
		OAuth: []oauth.SnapshotEntry{{
			Key: "dev|github",
			Config: oauth.Config{
				TokenURL:  "https://auth.local/token",
				GrantType: "client_credentials",
				CacheKey:  "github",
			},
			Token: oauth.Token{
				AccessToken:  "token",
				RefreshToken: "refresh",
				TokenType:    "Bearer",
				Expiry:       now.Add(time.Hour),
			},
		}},
		Command: []authcmd.SnapshotEntry{{
			Key:       "dev|github",
			Config:    authcmd.Config{CacheKey: "github"},
			Token:     "cmd-token",
			Type:      "Bearer",
			Expiry:    now.Add(time.Hour),
			FetchedAt: now,
		}},
	}

	rt := New(Config{})
	rt.LoadAuthState(st)
	got := rt.AuthState()

	if len(got.OAuth) != 1 {
		t.Fatalf("expected one oauth snapshot, got %d", len(got.OAuth))
	}
	if got.OAuth[0].Token.AccessToken != "token" {
		t.Fatalf("unexpected oauth token: %+v", got.OAuth[0].Token)
	}
	if len(got.Command) != 1 {
		t.Fatalf("expected one command snapshot, got %d", len(got.Command))
	}
	if got.Command[0].Token != "cmd-token" {
		t.Fatalf("unexpected command token: %+v", got.Command[0])
	}
}

func TestRuntimeCloseClosesHistory(t *testing.T) {
	hs := &stubHistory{}
	rt := New(Config{History: hs})
	if err := rt.Close(); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
	if !hs.closed {
		t.Fatal("expected history store to be closed")
	}
}

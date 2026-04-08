package authcmd

import (
	"strings"
	"testing"
	"time"
)

func TestExtractText(t *testing.T) {
	t.Parallel()

	cfg := Config{}
	res, err := extract(cfg, []byte("\n token-123 \n"), time.Now())
	if err != nil {
		t.Fatalf("extract() error = %v", err)
	}
	if res.Token != "token-123" {
		t.Fatalf("expected token, got %q", res.Token)
	}
	if res.Value != "Bearer token-123" {
		t.Fatalf("expected bearer header, got %q", res.Value)
	}
	if res.Header != "Authorization" {
		t.Fatalf("expected auth header, got %q", res.Header)
	}
}

func TestExtractTextRejectsMultipleValues(t *testing.T) {
	t.Parallel()

	_, err := extract(Config{}, []byte("a\nb\n"), time.Now())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExtractJSON(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0).UTC()
	cfg := Config{
		Format:        FormatJSON,
		TokenPath:     "access_token",
		TypePath:      "token_type",
		ExpiresInPath: "expires_in",
	}

	res, err := extract(
		cfg,
		[]byte(`{"access_token":"abc","token_type":"Token","expires_in":"60"}`),
		now,
	)
	if err != nil {
		t.Fatalf("extract() error = %v", err)
	}
	if res.Token != "abc" {
		t.Fatalf("expected token, got %q", res.Token)
	}
	if res.Type != "Token" {
		t.Fatalf("expected type, got %q", res.Type)
	}
	if res.Value != "Token abc" {
		t.Fatalf("expected header value, got %q", res.Value)
	}
	if !res.Expiry.Equal(now.Add(time.Minute)) {
		t.Fatalf("expected expiry %s, got %s", now.Add(time.Minute), res.Expiry)
	}
}

func TestExtractJSONRejectsTrailingGarbage(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Format:    FormatJSON,
		TokenPath: "token",
	}

	_, err := extract(cfg, []byte(`{"token":"abc"} trailing`), time.Now())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExtractJSONRejectsInvalidExpiresIn(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Format:        FormatJSON,
		TokenPath:     "token",
		ExpiresInPath: "expires_in",
	}

	_, err := extract(cfg, []byte(`{"token":"abc","expires_in":"soon"}`), time.Now())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `invalid expires_in value "soon"`) {
		t.Fatalf("unexpected error %q", err.Error())
	}
}

func TestExtractJSONCustomHeaderUsesRawToken(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Format:    FormatJSON,
		Header:    "X-API-Key",
		TokenPath: "token",
		TypePath:  "token_type",
	}

	res, err := extract(cfg, []byte(`{"token":"abc","token_type":"Bearer"}`), time.Now())
	if err != nil {
		t.Fatalf("extract() error = %v", err)
	}
	if res.Value != "abc" {
		t.Fatalf("expected raw token, got %q", res.Value)
	}
	if res.Type != "" {
		t.Fatalf("expected empty type for custom header, got %q", res.Type)
	}
}

func TestExtractJSONSchemeOverridesType(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Format:    FormatJSON,
		TokenPath: "token",
		TypePath:  "token_type",
		Scheme:    "Token",
	}

	res, err := extract(cfg, []byte(`{"token":"abc","token_type":"Bearer"}`), time.Now())
	if err != nil {
		t.Fatalf("extract() error = %v", err)
	}
	if res.Value != "Token abc" {
		t.Fatalf("expected explicit scheme, got %q", res.Value)
	}
	if res.Type != "Token" {
		t.Fatalf("expected scheme type, got %q", res.Type)
	}
}

func TestParseExpiry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want time.Time
	}{
		{
			name: "rfc3339",
			in:   "2026-04-07T12:00:00Z",
			want: time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC),
		},
		{
			name: "unix seconds",
			in:   "1712491200",
			want: time.Unix(1712491200, 0),
		},
		{
			name: "unix millis",
			in:   "1712491200123",
			want: time.UnixMilli(1712491200123),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseExpiry(tt.in)
			if err != nil {
				t.Fatalf("parseExpiry() error = %v", err)
			}
			if !got.Equal(tt.want) {
				t.Fatalf("parseExpiry() = %s, want %s", got, tt.want)
			}
		})
	}
}

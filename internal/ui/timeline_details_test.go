package ui

import (
	"testing"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
)

func TestTLSVerificationStatus(t *testing.T) {
	cases := []struct {
		name    string
		details *nettrace.TLSDetails
		want    tlsVerifyStatus
	}{
		{
			name:    "verified",
			details: &nettrace.TLSDetails{Verified: true},
			want:    tlsVerifyYes,
		},
		{
			name: "unverified-handshake",
			details: &nettrace.TLSDetails{
				Version: "TLS 1.3",
				Cipher:  "TLS_AES_128_GCM_SHA256",
			},
			want: tlsVerifyNo,
		},
		{
			name:    "alpn-only",
			details: &nettrace.TLSDetails{ALPN: "h2"},
			want:    tlsVerifyUnknown,
		},
		{
			name: "resumed",
			details: &nettrace.TLSDetails{
				Resumed: true,
				Version: "TLS 1.3",
			},
			want: tlsVerifyUnknown,
		},
		{
			name: "certs-unverified",
			details: &nettrace.TLSDetails{
				Certificates: []nettrace.TLSCert{{Subject: "example.com"}},
			},
			want: tlsVerifyNo,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tlsVerificationStatus(tc.details); got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

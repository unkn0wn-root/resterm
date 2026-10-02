package grpcx

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	runfail "github.com/unkn0wn-root/resterm/internal/runx/fail"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
)

// gRPC turns a TLS failure into an Unavailable status, the same code a
// network failure gets. The run must still fail with the TLS code.
func TestExecuteReportsTLSFailuresAsTLS(t *testing.T) {
	pki := newTestPKI(t)
	reflect := func(srv *grpc.Server) { reflection.RegisterV1(srv) }
	tlsAddr := startTestServerWith(t, reflect, grpc.Creds(credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{pki.server},
	})))
	mtlsAddr := startTestServerWith(t, reflect, grpc.Creds(credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{pki.server},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pki.pool,
	})))

	for _, tc := range []struct {
		name  string
		addr  string
		plain bool
		roots []string
		want  runfail.Code
	}{
		{name: "untrusted server", addr: tlsAddr, want: runfail.CodeTLS},
		{name: "missing client certificate", addr: mtlsAddr, roots: []string{pki.caFile}, want: runfail.CodeTLS},
		{name: "plaintext against TLS", addr: tlsAddr, plain: true, want: runfail.CodeNetwork},
		{name: "trusted server", addr: tlsAddr, roots: []string{pki.caFile}, want: runfail.CodeProtocol},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gr := baseStreamReq(tc.addr, "UnimplementedCall")
			gr.Plaintext = restfile.OptOf(tc.plain)
			req := &restfile.Request{GRPC: gr, Settings: map[string]string{}}
			opt := Options{RootCAs: tc.roots, DialTimeout: 2 * time.Second}

			_, err := NewClient().Execute(context.Background(), req, opt, nil)
			if got := runfail.FromError(err).Code; got != tc.want {
				t.Fatalf("failure code = %q, want %q: %v", got, tc.want, err)
			}
		})
	}
}

type testPKI struct {
	caFile string
	pool   *x509.CertPool
	server tls.Certificate
}

// newTestPKI makes a CA and a server certificate for 127.0.0.1 signed by it.
func newTestPKI(t *testing.T) testPKI {
	t.Helper()
	now := time.Now()
	caKey := newTestKey(t)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "resterm test CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA: %v", err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA: %v", err)
	}

	srvKey := newTestKey(t)
	srvTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	srvDER, err := x509.CreateCertificate(rand.Reader, srvTmpl, ca, &srvKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create server certificate: %v", err)
	}

	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(
		caFile,
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		0o600,
	); err != nil {
		t.Fatalf("write CA: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	return testPKI{
		caFile: caFile,
		pool:   pool,
		server: tls.Certificate{Certificate: [][]byte{srvDER}, PrivateKey: srvKey},
	}
}

func newTestKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

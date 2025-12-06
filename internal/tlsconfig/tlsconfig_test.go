package tlsconfig

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildIncludesCustomCAAndSystem(t *testing.T) {
	tmp := t.TempDir()
	caPath := filepath.Join(tmp, "ca.pem")
	writeTestCA(t, caPath)

	cfg, err := Build(Files{RootCAs: []string{"ca.pem"}, RootMode: RootModeAppend}, tmp)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if cfg.RootCAs == nil {
		t.Fatalf("expected RootCAs to be set")
	}
	if len(cfg.RootCAs.Subjects()) == 0 {
		t.Fatalf("expected at least one subject in pool")
	}
}

func TestBuildReplaceOnlyCustomCAs(t *testing.T) {
	tmp := t.TempDir()
	caPath := filepath.Join(tmp, "ca.pem")
	writeTestCA(t, caPath)

	cfg, err := Build(Files{RootCAs: []string{"ca.pem"}, RootMode: RootModeReplace}, tmp)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if cfg.RootCAs == nil {
		t.Fatalf("expected RootCAs to be set")
	}
	if len(cfg.RootCAs.Subjects()) != 1 {
		t.Fatalf("expected only custom CA to be present, got %d subjects", len(cfg.RootCAs.Subjects()))
	}
}

func TestBuildLoadsClientCert(t *testing.T) {
	tmp := t.TempDir()
	certPath := filepath.Join(tmp, "client.pem")
	keyPath := filepath.Join(tmp, "client.key")
	writeTestCert(t, certPath, keyPath)

	cfg, err := Build(Files{ClientCert: "client.pem", ClientKey: "client.key"}, tmp)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(cfg.Certificates) != 1 {
		t.Fatalf("expected client certificate to be loaded")
	}
}

func TestBuildRejectsPartialClientCert(t *testing.T) {
	_, err := Build(Files{ClientCert: "only-cert.pem"}, "")
	if err == nil {
		t.Fatalf("expected error for partial client cert/key")
	}
}

func writeTestCA(t *testing.T, path string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ca key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "resterm-test-ca",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create ca cert: %v", err)
	}
	pemData := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(path, pemData, 0o644); err != nil {
		t.Fatalf("write ca pem: %v", err)
	}
}

func writeTestCert(t *testing.T, certPath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName: "resterm-test-client",
		},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create client cert: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		t.Fatalf("write client cert: %v", err)
	}

	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal client key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write client key: %v", err)
	}
}

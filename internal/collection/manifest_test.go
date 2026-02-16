package collection

import (
	"bytes"
	"reflect"
	"testing"
	"time"
)

func TestManifestRoundTrip(t *testing.T) {
	baseTime := time.Date(2026, 2, 13, 1, 2, 3, 0, time.UTC)
	in := Manifest{
		Schema:    SchemaName,
		Version:   SchemaVersion,
		Name:      "  Team API  ",
		CreatedAt: baseTime,
		Files: []File{
			{
				Path: "rts/helpers.rts",
				Role: RoleScript,
				Size: 12,
				Digest: Digest{
					Alg:   AlgSHA256,
					Value: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				},
			},
			{
				Path: "requests.http",
				Role: RoleRequest,
				Size: 34,
				Digest: Digest{
					Alg:   AlgSHA256,
					Value: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				},
			},
		},
	}

	data, err := EncodeManifest(in)
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}

	got, err := DecodeManifest(data)
	if err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	want, err := in.Normalize()
	if err != nil {
		t.Fatalf("normalize manifest: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round-trip mismatch\n got=%+v\nwant=%+v", got, want)
	}
}

func TestManifestEncodeDeterministicOrder(t *testing.T) {
	fa := File{
		Path: "z.http",
		Role: RoleRequest,
		Size: 1,
		Digest: Digest{
			Alg:   AlgSHA256,
			Value: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}
	fb := File{
		Path: "a.http",
		Role: RoleRequest,
		Size: 1,
		Digest: Digest{
			Alg:   AlgSHA256,
			Value: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	}

	m1 := Manifest{Files: []File{fa, fb}}
	m2 := Manifest{Files: []File{fb, fa}}

	d1, err := EncodeManifest(m1)
	if err != nil {
		t.Fatalf("encode m1: %v", err)
	}
	d2, err := EncodeManifest(m2)
	if err != nil {
		t.Fatalf("encode m2: %v", err)
	}

	if !bytes.Equal(d1, d2) {
		t.Fatalf("manifest output is not deterministic\nm1:\n%s\nm2:\n%s", d1, d2)
	}
}

func TestManifestRejectsDuplicatePaths(t *testing.T) {
	m := Manifest{
		Files: []File{
			{
				Path: "requests.http",
				Role: RoleRequest,
				Size: 1,
				Digest: Digest{
					Alg:   AlgSHA256,
					Value: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				},
			},
			{
				Path: "./requests.http",
				Role: RoleRequest,
				Size: 1,
				Digest: Digest{
					Alg:   AlgSHA256,
					Value: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				},
			},
		},
	}

	if _, err := m.Normalize(); err == nil {
		t.Fatalf("expected duplicate path error")
	}
}

func TestNewManifestSetsCreatedAt(t *testing.T) {
	m := NewManifest("bundle", nil)
	if m.CreatedAt.IsZero() {
		t.Fatalf("expected CreatedAt to be set")
	}
	if !m.CreatedAt.Equal(m.CreatedAt.UTC()) {
		t.Fatalf("expected CreatedAt in UTC, got %s", m.CreatedAt.Location())
	}
}

func TestNewManifestAtUsesProvidedTimestamp(t *testing.T) {
	ts := time.Date(2026, 2, 13, 9, 10, 11, 0, time.FixedZone("custom", -5*60*60))
	m := NewManifestAt("bundle", nil, ts)

	if m.CreatedAt.IsZero() {
		t.Fatalf("expected CreatedAt to be set")
	}
	if !m.CreatedAt.Equal(ts.UTC()) {
		t.Fatalf("created_at=%s want %s", m.CreatedAt, ts.UTC())
	}
}

package collection

import (
	"slices"
	"strings"
	"time"
)

const (
	SchemaName    = "resterm.collection.bundle"
	SchemaVersion = 1

	AlgSHA256 = "sha256"
)

type FileRole string

const (
	RoleRequest     FileRole = "request"
	RoleScript      FileRole = "script"
	RoleAsset       FileRole = "asset"
	RoleEnvTemplate FileRole = "env_template"
)

func (r FileRole) valid() bool {
	switch r {
	case RoleRequest, RoleScript, RoleAsset, RoleEnvTemplate:
		return true
	default:
		return false
	}
}

type Digest struct {
	Alg   string `json:"alg"`
	Value string `json:"value"`
}

type File struct {
	Path   string   `json:"path"`
	Role   FileRole `json:"role"`
	Size   int64    `json:"size"`
	Digest Digest   `json:"digest"`
}

type Manifest struct {
	Schema    string    `json:"schema"`
	Version   int       `json:"version"`
	Name      string    `json:"name,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	Files     []File    `json:"files"`
}

func NewManifest(name string, files []File) Manifest {
	return NewManifestAt(name, files, time.Now().UTC())
}

func NewManifestAt(name string, files []File, createdAt time.Time) Manifest {
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	} else {
		createdAt = createdAt.UTC()
	}
	return Manifest{
		Schema:    SchemaName,
		Version:   SchemaVersion,
		Name:      strings.TrimSpace(name),
		CreatedAt: createdAt,
		Files:     slices.Clone(files),
	}
}

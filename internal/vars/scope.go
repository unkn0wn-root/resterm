package vars

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	str "github.com/unkn0wn-root/resterm/internal/util"
)

// Every scope key starts with one of these prefixes, so provenance is written
// by the code and can never be posed by an environment name. The letter says
// grouped or flat, the digit says whether the environment file is folded in.
// Scope keys own all persisted per-selection runtime state (globals, file
// variables, cookie jars, command auth, OAuth tokens), so changing a prefix
// discards every stored value.
const (
	scopeVersion           = "g1:"
	scopeSourceVersion     = "g2:"
	scopeFlatVersion       = "f1:"
	scopeFlatSourceVersion = "f2:"
)

// scopeDigestLen is fixed so the last scopeDigestLen characters of a sourced
// flat scope, behind the "@", are always the digest and the rest is the name.
const scopeDigestLen = 16

// withSource records the file a catalog was read from. The path is made
// absolute so different spellings of one file keep one identity.
func (c Catalog) withSource(path string) Catalog {
	path = str.Trim(path)
	if path == "" {
		return c
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	c.source = filepath.Clean(path)
	return c
}

// flatScope keeps the environment name readable and appends a fixed width digest
// of the file it came from, so no two (name, source) pairs render alike.
func flatScope(name, source string) string {
	if source == "" {
		return scopeFlatVersion + name
	}
	return fmt.Sprintf("%s%s@%s", scopeFlatSourceVersion, name, sourceDigest(source))
}

func groupScope(groups []Group, sel Selection, source string) string {
	h := sha256.New()
	for _, g := range groups {
		writePart(h, strings.ToLower(g.Name))
		writePart(h, strings.ToLower(sel.profiles[g.Name]))
	}
	if source == "" {
		return scopeVersion + hex.EncodeToString(h.Sum(nil))
	}

	writePart(h, source)
	return scopeSourceVersion + hex.EncodeToString(h.Sum(nil))
}

func sourceDigest(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])[:scopeDigestLen]
}

// SourcedScope reports whether scope names the environment file it came from.
// No other workspace can resolve such a scope, so runtime state held under it
// is safe to keep across a workspace change. Only the version prefix decides,
// so no environment name can pose as a sourced scope.
func SourcedScope(scope string) bool {
	return strings.HasPrefix(scope, scopeSourceVersion) ||
		strings.HasPrefix(scope, scopeFlatSourceVersion)
}

func writePart(w io.Writer, s string) {
	_, _ = io.WriteString(w, strconv.Itoa(len(s)))
	_, _ = io.WriteString(w, ":")
	_, _ = io.WriteString(w, s)
}

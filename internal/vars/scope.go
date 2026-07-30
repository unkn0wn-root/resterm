package vars

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	str "github.com/unkn0wn-root/resterm/internal/util"
)

// scopeVersion prefixes every grouped scope key. Scope keys own all persisted
// per-selection runtime state (globals, file variables, cookie jars, command
// auth, OAuth tokens), so bumping this discards every stored value.
const scopeVersion = "g1:"

// scopeSourceVersion prefixes grouped scopes that fold in the environment
// file. A catalog built in process has no file and keeps its old scopes.
const scopeSourceVersion = "g2:"

const (
	scopeSourceSep = "@"
	// scopeDigestLen is fixed so the last scopeDigestLen characters of a flat
	// scope are always the digest and the rest is the name.
	scopeDigestLen = 16
)

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
		return name
	}
	return name + scopeSourceSep + sourceDigest(source)
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

func writePart(w io.Writer, s string) {
	_, _ = io.WriteString(w, strconv.Itoa(len(s)))
	_, _ = io.WriteString(w, ":")
	_, _ = io.WriteString(w, s)
}

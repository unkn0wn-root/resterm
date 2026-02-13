package collection

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

func SumSHA256(data []byte) Digest {
	sum := sha256.Sum256(data)
	return Digest{
		Alg:   AlgSHA256,
		Value: hex.EncodeToString(sum[:]),
	}
}

func VerifyDigest(data []byte, got Digest) bool {
	got = normDigest(got)
	if got.Alg != AlgSHA256 || !ValidSHA256(got.Value) {
		return false
	}
	want := SumSHA256(data).Value
	return subtle.ConstantTimeCompare([]byte(want), []byte(got.Value)) == 1
}

func ValidSHA256(v string) bool {
	if len(v) != sha256.Size*2 {
		return false
	}
	for i := 0; i < len(v); i++ {
		b := v[i]
		if isHex(b) {
			continue
		}
		return false
	}
	return true
}

func normDigest(d Digest) Digest {
	d.Alg = strings.ToLower(strings.TrimSpace(d.Alg))
	d.Value = strings.ToLower(strings.TrimSpace(d.Value))
	return d
}

func isHex(b byte) bool {
	return (b >= '0' && b <= '9') ||
		(b >= 'a' && b <= 'f') ||
		(b >= 'A' && b <= 'F')
}

package dynamic

import (
	"crypto/rand"
	"encoding/binary"
)

const alphanum = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// Helper output routinely ends up in signup payloads, tokens and idempotency
// keys, so every value here comes from the system CSPRNG. crypto/rand.Read
// cannot fail on a supported platform, so these primitives report no errors.

func randUint64Full() uint64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return binary.BigEndian.Uint64(b[:])
}

// randUint64 returns a uniform value below n, which must be positive. Draws
// that 2^64 does not divide evenly into n are redrawn, so a range as wide as
// $randomInt(0, 9007199254740993) stays unbiased. Under half of draws are ever
// rejected.
func randUint64(n uint64) uint64 {
	// -n is 2^64-n unsigned, so this is 2^64 mod n: the leading block of
	// values one extra wrap of the modulo would favour.
	reject := -n % n
	for {
		v := randUint64Full()
		if v >= reject {
			return v % n
		}
	}
}

// pick returns a random element of items, which must not be empty.
func pick[T any](items []T) T {
	return items[randUint64(uint64(len(items)))]
}

// randomString returns n characters from alphanum, dropping the bytes that
// would skew the modulo.
func randomString(n int) string {
	const limit = 256 - 256%len(alphanum)

	out := make([]byte, 0, n)
	buf := make([]byte, n)
	for len(out) < n {
		_, _ = rand.Read(buf)
		for _, b := range buf {
			if int(b) >= limit {
				continue
			}
			out = append(out, alphanum[int(b)%len(alphanum)])
			if len(out) == n {
				break
			}
		}
	}
	return string(out)
}

func randomDigits(n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = byte('0' + randUint64(10))
	}
	return string(out)
}

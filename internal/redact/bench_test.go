package redact

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
)

func benchSecrets() []string {
	out := make([]string, 20)
	for i := range out {
		out[i] = fmt.Sprintf("s3cr3t-%02d-%x", i, i*7919)
	}
	return out
}

func jsonBody(hit string) []byte {
	type item struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
		Note  string `json:"note"`
	}
	var items []item
	for i := 0; len(items)*110 < 1<<20; i++ {
		it := item{
			ID:    i,
			Name:  fmt.Sprintf("user %d ü", i),
			Email: fmt.Sprintf("u%d@example.test", i),
			Note:  "line\nnext <b> & \"q\"",
		}
		if hit != "" && i%50 == 0 {
			it.Note = "token " + hit
		}
		items = append(items, it)
	}
	b, _ := json.Marshal(map[string]any{"items": items})
	return b
}

func BenchmarkJSON1MiB(b *testing.B) {
	secs := benchSecrets()
	s := New(secs...)
	var v any
	_ = json.Unmarshal(jsonBody(secs[3]), &v)
	for name, body := range map[string][]byte{
		"nohit":  jsonBody(""),
		"hits":   jsonBody(secs[3]),
		"python": asciiJSON(v, "\\u%04x"),
	} {
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(body)))
			for b.Loop() {
				s.JSON(body)
			}
		})
	}
}

func BenchmarkText1MiB(b *testing.B) {
	s := New(benchSecrets()...)
	for name, body := range map[string]string{
		"json":  string(jsonBody("")),
		"plain": strings.Repeat("plain log line with words and numbers 12345\n", 1<<20/44),
	} {
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(body)))
			for b.Loop() {
				s.Text(body)
			}
		})
	}
}

func BenchmarkTextHostile(b *testing.B) {
	s := New(benchSecrets()...)
	for name, body := range map[string]string{
		"nesting": "%" + strings.Repeat("25", 1<<19) + "41",
		"escapes": strings.Repeat(`ü%41&amp;`, 1<<20/15),
	} {
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(body)))
			for b.Loop() {
				s.Text(body)
			}
		})
	}
}

func BenchmarkTextMixed(b *testing.B) {
	s := New(benchSecrets()...)
	for name, unit := range map[string]string{
		"html-url": `<a href="/x?a=1&amp;b=%2F">x</a> `,
		"two":      `a%2Fb \"q\" `,
		"three":    `%41\x41&#65; `,
	} {
		body := strings.Repeat(unit, 1<<20/len(unit))
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(body)))
			for b.Loop() {
				s.Text(body)
			}
		})
	}
}

func BenchmarkJSONBase64(b *testing.B) {
	s := New(benchSecrets()...)
	raw := make([]byte, 3<<18)
	_, _ = rand.NewChaCha8([32]byte{}).Read(raw)
	body := []byte(`{"name":"photo.png","data":"` + base64.StdEncoding.EncodeToString(raw) + `"}`)
	b.SetBytes(int64(len(body)))
	for b.Loop() {
		s.JSON(body)
	}
}

func BenchmarkTextShort(b *testing.B) {
	s := New(benchSecrets()...)
	for b.Loop() {
		s.Text("GET https://api.example.test/v1/users/42?key=abc%2Fdef")
	}
}

func BenchmarkNew(b *testing.B) {
	secs := benchSecrets()
	for b.Loop() {
		New(secs...)
	}
}

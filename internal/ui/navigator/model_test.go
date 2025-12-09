package navigator

import "testing"

func TestWordsFromTextHandlesUnicode(t *testing.T) {
	tokens := wordsFromText("Bonjour-titre 你好/请求 München")
	want := []string{"bonjour-titre", "你好", "请求", "münchen"}
	for _, expected := range want {
		found := false
		for _, token := range tokens {
			if token == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected token %q in %v", expected, tokens)
		}
	}
}

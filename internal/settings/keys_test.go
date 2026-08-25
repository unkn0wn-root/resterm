package settings

import "testing"

func TestIsHTTPKey(t *testing.T) {
	ok := []string{
		"base-url",
		"timeout",
		"proxy",
		"followredirects",
		"insecure",
		"no-cookies",
		"http-version",
		"http-root-cas",
		"HTTP-CLIENT-CERT",
	}
	for _, k := range ok {
		if !IsHTTPKey(k) {
			t.Fatalf("expected http key %q to be supported", k)
		}
	}

	bad := []string{"grpc-timeout", "", "unsupported-setting", "httpx-root"}
	for _, k := range bad {
		if IsHTTPKey(k) {
			t.Fatalf("expected http key %q to be unsupported", k)
		}
	}
}

func TestMergeNormalizesKeysBeforeApplyingScopePrecedence(t *testing.T) {
	global := map[string]string{" BASE-URL ": "https://global.example/"}
	file := map[string]string{"Base-Url": "https://file.example/"}
	request := map[string]string{"base-url": "https://request.example/"}

	got := Merge(global, file, request)
	if len(got) != 1 || got["base-url"] != "https://request.example/" {
		t.Fatalf("Merge() = %#v, want canonical request override", got)
	}
}

func TestFromValuesReturnsCanonicalSettingKeys(t *testing.T) {
	got := FromValues(map[string]string{
		" SETTINGS.Base-URL ": "https://api.example/",
		"ordinary":            "value",
	})
	if len(got) != 1 || got["base-url"] != "https://api.example/" {
		t.Fatalf("FromValues() = %#v, want canonical base-url", got)
	}
}

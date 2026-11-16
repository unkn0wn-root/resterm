package settings

import (
	"maps"
	"testing"
)

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

func TestMergeKeepsOneScopePerOption(t *testing.T) {
	for _, tt := range []struct {
		name   string
		scopes []map[string]string
		want   map[string]string
	}{
		{
			name:   "request insecure replaces env http-insecure",
			scopes: []map[string]string{{"http-insecure": "true"}, nil, {"insecure": "false"}},
			want:   map[string]string{"insecure": "false"},
		},
		{
			name:   "request http-insecure replaces env insecure",
			scopes: []map[string]string{{"insecure": "true"}, nil, {"http-insecure": "false"}},
			want:   map[string]string{"http-insecure": "false"},
		},
		{
			name: "each scope replaces the one below",
			scopes: []map[string]string{
				{"http-insecure": "true"},
				{"insecure": "false"},
				{"http-insecure": "false"},
			},
			want: map[string]string{"http-insecure": "false"},
		},
		{
			name:   "a middle scope replaces the one below",
			scopes: []map[string]string{{"http-insecure": "true"}, {"insecure": "false"}, nil},
			want:   map[string]string{"insecure": "false"},
		},
		{
			name:   "same key",
			scopes: []map[string]string{{"insecure": "true"}, {"insecure": "false"}},
			want:   map[string]string{"insecure": "false"},
		},
		{
			name:   "http root ca replaces root cas",
			scopes: []map[string]string{{"http-root-cas": "a.pem"}, {"http-root-ca": "b.pem"}},
			want:   map[string]string{"http-root-ca": "b.pem"},
		},
		{
			name:   "http root cas replaces root ca",
			scopes: []map[string]string{{"http-root-ca": "a.pem"}, {"http-root-cas": "b.pem"}},
			want:   map[string]string{"http-root-cas": "b.pem"},
		},
		{
			name:   "empty root cas clears root ca",
			scopes: []map[string]string{{"http-root-ca": "a.pem"}, {"http-root-cas": ""}},
			want:   map[string]string{"http-root-cas": ""},
		},
		{
			name:   "grpc root ca replaces root cas",
			scopes: []map[string]string{{"grpc-root-cas": "a.pem"}, {"grpc-root-ca": "b.pem"}},
			want:   map[string]string{"grpc-root-ca": "b.pem"},
		},
		{
			name:   "grpc root cas replaces root ca",
			scopes: []map[string]string{{"grpc-root-ca": "a.pem"}, {"grpc-root-cas": "b.pem"}},
			want:   map[string]string{"grpc-root-cas": "b.pem"},
		},
		{
			name:   "same scope keeps both names",
			scopes: []map[string]string{{"insecure": "true", "http-insecure": "false"}},
			want:   map[string]string{"insecure": "true", "http-insecure": "false"},
		},
		{
			name:   "same scope keeps both root names",
			scopes: []map[string]string{{"http-insecure": "true"}, {"http-root-ca": "a.pem", "http-root-cas": "b.pem"}},
			want:   map[string]string{"http-insecure": "true", "http-root-ca": "a.pem", "http-root-cas": "b.pem"},
		},
		{
			name: "same scope replaces a lower name",
			scopes: []map[string]string{
				{"http-insecure": "true"},
				{"insecure": "false", "http-insecure": "false"},
			},
			want: map[string]string{"insecure": "false", "http-insecure": "false"},
		},
		{
			name: "higher scope replaces a lower conflict",
			scopes: []map[string]string{
				{"grpc-root-ca": "a.pem", "grpc-root-cas": "b.pem"},
				{"grpc-root-ca": "c.pem"},
			},
			want: map[string]string{"grpc-root-ca": "c.pem"},
		},
		{
			name:   "case and whitespace forms",
			scopes: []map[string]string{{" HTTP-Insecure ": "true"}, {"Insecure": "false"}},
			want:   map[string]string{"insecure": "false"},
		},
		{
			name: "other keys stay",
			scopes: []map[string]string{
				{"http-insecure": "true", "grpc-insecure": "true", "grpc-root-ca": "c.pem", "timeout": "1s"},
				{"insecure": "false", "http-root-cas": "a.pem"},
			},
			want: map[string]string{
				"insecure":      "false",
				"grpc-insecure": "true",
				"grpc-root-ca":  "c.pem",
				"http-root-cas": "a.pem",
				"timeout":       "1s",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := Merge(tt.scopes...); !maps.Equal(got, tt.want) {
				t.Fatalf("Merge() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPatchLayersOverBase(t *testing.T) {
	ptr := func(s string) *string { return &s }
	for _, tt := range []struct {
		name  string
		base  map[string]string
		patch map[string]*string
		want  map[string]string
	}{
		{
			name:  "insecure replaces http-insecure",
			base:  map[string]string{"http-insecure": "true"},
			patch: map[string]*string{"insecure": ptr("false")},
			want:  map[string]string{"insecure": "false"},
		},
		{
			name:  "http-insecure replaces insecure",
			base:  map[string]string{"insecure": "true"},
			patch: map[string]*string{"http-insecure": ptr("false")},
			want:  map[string]string{"http-insecure": "false"},
		},
		{
			name:  "http root ca replaces root cas",
			base:  map[string]string{"http-root-cas": "a.pem"},
			patch: map[string]*string{"http-root-ca": ptr("b.pem")},
			want:  map[string]string{"http-root-ca": "b.pem"},
		},
		{
			name:  "http root cas replaces root ca",
			base:  map[string]string{"http-root-ca": "a.pem"},
			patch: map[string]*string{"http-root-cas": ptr("b.pem")},
			want:  map[string]string{"http-root-cas": "b.pem"},
		},
		{
			name:  "grpc root ca replaces root cas",
			base:  map[string]string{"grpc-root-cas": "a.pem"},
			patch: map[string]*string{"grpc-root-ca": ptr("b.pem")},
			want:  map[string]string{"grpc-root-ca": "b.pem"},
		},
		{
			name:  "grpc root cas replaces root ca",
			base:  map[string]string{"grpc-root-ca": "a.pem"},
			patch: map[string]*string{"grpc-root-cas": ptr("b.pem")},
			want:  map[string]string{"grpc-root-cas": "b.pem"},
		},
		{
			name:  "same key",
			base:  map[string]string{"insecure": "true", "timeout": "1s"},
			patch: map[string]*string{"insecure": ptr("false")},
			want:  map[string]string{"insecure": "false", "timeout": "1s"},
		},
		{
			name:  "one name replaces a base that wrote both",
			base:  map[string]string{"insecure": "true", "http-insecure": "true"},
			patch: map[string]*string{"insecure": ptr("false")},
			want:  map[string]string{"insecure": "false"},
		},
		{
			name:  "null removes both names",
			base:  map[string]string{"insecure": "true", "http-insecure": "false", "timeout": "1s"},
			patch: map[string]*string{"http-insecure": nil},
			want:  map[string]string{"timeout": "1s"},
		},
		{
			name:  "null removes case forms",
			base:  map[string]string{"Insecure": "true", " HTTP-Root-CA ": "a.pem"},
			patch: map[string]*string{" HTTP-Insecure ": nil, "Http-Root-Cas": nil},
			want:  map[string]string{},
		},
		{
			name:  "null and the other name keep the set value",
			base:  map[string]string{"insecure": "true"},
			patch: map[string]*string{"insecure": nil, "http-insecure": ptr("false")},
			want:  map[string]string{"http-insecure": "false"},
		},
		{
			name:  "unpaired null removes only its key",
			base:  map[string]string{"timeout": "1s", "proxy": "http://p:1"},
			patch: map[string]*string{"timeout": nil},
			want:  map[string]string{"proxy": "http://p:1"},
		},
		{
			name:  "both names in one patch both stay",
			base:  map[string]string{"insecure": "true"},
			patch: map[string]*string{"insecure": ptr("false"), "http-insecure": ptr("true")},
			want:  map[string]string{"insecure": "false", "http-insecure": "true"},
		},
		{
			name:  "nil base",
			patch: map[string]*string{"insecure": ptr("true"), "timeout": nil},
			want:  map[string]string{"insecure": "true"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			base := maps.Clone(tt.base)
			if got := Patch(tt.base, tt.patch); !maps.Equal(got, tt.want) {
				t.Fatalf("Patch() = %v, want %v", got, tt.want)
			}
			if !maps.Equal(tt.base, base) {
				t.Fatalf("Patch() changed base to %v", tt.base)
			}
		})
	}
}

func TestSameOptionIsSymmetric(t *testing.T) {
	for k, v := range sameOption {
		if k == v || sameOption[v] != k {
			t.Errorf("sameOption[%q] = %q, but sameOption[%q] = %q", k, v, v, sameOption[v])
		}
	}
}

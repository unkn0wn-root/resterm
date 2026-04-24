package runfail

import "testing"

func TestClassifyMessageSharedRules(t *testing.T) {
	classifiers := []struct {
		name string
		fn   func(string, string) Failure
	}{
		{name: "generic", fn: classifyMessage},
		{name: "http", fn: classifyHTTPMessage},
	}
	for _, classifier := range classifiers {
		t.Run(classifier.name, func(t *testing.T) {
			got := classifier.fn("oauth token expired", "source")
			if got.Code != CodeAuth {
				t.Fatalf("classification = %+v, want code=%s", got, CodeAuth)
			}
		})
	}
}

func TestClassifyMessageContextSpecificRules(t *testing.T) {
	cases := []struct {
		name string
		fn   func(string, string) Failure
		msg  string
		code Code
	}{
		{
			name: "generic default unknown",
			fn:   classifyMessage,
			msg:  "unexpected failure",
			code: CodeUnknown,
		},
		{
			name: "generic profile is not filesystem",
			fn:   classifyMessage,
			msg:  "profile data unavailable",
			code: CodeUnknown,
		},
		{
			name: "generic mainstream is not protocol",
			fn:   classifyMessage,
			msg:  "mainstream response unavailable",
			code: CodeUnknown,
		},
		{
			name: "generic unexpected token is not auth",
			fn:   classifyMessage,
			msg:  "unexpected token in JSON",
			code: CodeUnknown,
		},
		{
			name: "generic input stream is not protocol",
			fn:   classifyMessage,
			msg:  "input stream closed",
			code: CodeUnknown,
		},
		{
			name: "generic file mention is not filesystem",
			fn:   classifyMessage,
			msg:  "profile file format rejected",
			code: CodeUnknown,
		},
		{
			name: "generic decode auth token stays auth",
			fn:   classifyMessage,
			msg:  "failed to decode authentication token",
			code: CodeAuth,
		},
		{
			name: "generic decode response is protocol",
			fn:   classifyMessage,
			msg:  "failed to decode response body",
			code: CodeProtocol,
		},
		{
			name: "generic network evidence beats auth token",
			fn:   classifyMessage,
			msg:  "oauth issuer connection refused",
			code: CodeNetwork,
		},
		{
			name: "http default protocol",
			fn:   classifyHTTPMessage,
			msg:  "unexpected failure",
			code: CodeProtocol,
		},
		{
			name: "http unexpected token defaults protocol",
			fn:   classifyHTTPMessage,
			msg:  "unexpected token in JSON",
			code: CodeProtocol,
		},
		{
			name: "generic filesystem",
			fn:   classifyMessage,
			msg:  "open config: permission denied",
			code: CodeFilesystem,
		},
		{
			name: "generic standalone file",
			fn:   classifyMessage,
			msg:  "file not found",
			code: CodeFilesystem,
		},
		{
			name: "http proxy network",
			fn:   classifyHTTPMessage,
			msg:  "proxy connection reset",
			code: CodeNetwork,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.fn(tc.msg, "source")
			if got.Code != tc.code {
				t.Fatalf("classification = %+v, want code=%s", got, tc.code)
			}
		})
	}
}

func TestMessageRuleConfiguration(t *testing.T) {
	groups := map[string][]messageRule{
		"shared":  sharedMessageRules,
		"generic": genericMessageRules,
		"http":    httpMessageRules,
	}
	for groupName, rules := range groups {
		for i, rule := range rules {
			if rule.code == "" {
				t.Fatalf("%s rule %d has no code", groupName, i)
			}
			if rule.priority <= 0 {
				t.Fatalf("%s rule %d (%s) has no priority", groupName, i, rule.code)
			}
			if len(rule.phrases) == 0 && len(rule.tokens) == 0 && len(rule.tokenGroups) == 0 {
				t.Fatalf("%s rule %d (%s) has no evidence", groupName, i, rule.code)
			}
		}
	}
}

func TestMessageClassifierDecision(t *testing.T) {
	t.Run("same confidence tie uses default", func(t *testing.T) {
		classifier := messageClassifier{
			defaultCode: CodeUnknown,
			rules: []messageRule{
				{code: CodeAuth, priority: 10, tokens: []string{"shared"}},
				{code: CodeProtocol, priority: 10, tokens: []string{"shared"}},
			},
		}
		if got := classifier.classify("shared", "source"); got.Code != CodeUnknown {
			t.Fatalf("classification = %+v, want code=%s", got, CodeUnknown)
		}
	})

	t.Run("explicit priority breaks same confidence", func(t *testing.T) {
		classifier := messageClassifier{
			defaultCode: CodeUnknown,
			rules: []messageRule{
				{code: CodeAuth, priority: 20, tokens: []string{"shared"}},
				{code: CodeProtocol, priority: 10, tokens: []string{"shared"}},
			},
		}
		if got := classifier.classify("shared", "source"); got.Code != CodeProtocol {
			t.Fatalf("classification = %+v, want code=%s", got, CodeProtocol)
		}
	})

	t.Run("weak matches are not additive", func(t *testing.T) {
		classifier := messageClassifier{
			defaultCode: CodeUnknown,
			rules: []messageRule{
				{
					code:     CodeAuth,
					priority: 10,
					tokens:   []string{"weak1", "weak2", "weak3"},
				},
				{
					code:     CodeProtocol,
					priority: 90,
					phrases:  []string{"strong signal"},
				},
			},
		}
		if got := classifier.classify("weak1 weak2 weak3 strong signal", "source"); got.Code != CodeProtocol {
			t.Fatalf("classification = %+v, want code=%s", got, CodeProtocol)
		}
	})
}

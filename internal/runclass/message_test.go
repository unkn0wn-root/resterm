package runclass

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
			if got.Code != FailureAuth {
				t.Fatalf("classification = %+v, want code=%s", got, FailureAuth)
			}
		})
	}
}

func TestClassifyMessageContextSpecificRules(t *testing.T) {
	cases := []struct {
		name string
		fn   func(string, string) Failure
		msg  string
		code FailureCode
	}{
		{
			name: "generic default unknown",
			fn:   classifyMessage,
			msg:  "unexpected failure",
			code: FailureUnknown,
		},
		{
			name: "generic profile is not filesystem",
			fn:   classifyMessage,
			msg:  "profile data unavailable",
			code: FailureUnknown,
		},
		{
			name: "generic mainstream is not protocol",
			fn:   classifyMessage,
			msg:  "mainstream response unavailable",
			code: FailureUnknown,
		},
		{
			name: "generic unexpected token is not auth",
			fn:   classifyMessage,
			msg:  "unexpected token in JSON",
			code: FailureUnknown,
		},
		{
			name: "generic input stream is not protocol",
			fn:   classifyMessage,
			msg:  "input stream closed",
			code: FailureUnknown,
		},
		{
			name: "generic file mention is not filesystem",
			fn:   classifyMessage,
			msg:  "profile file format rejected",
			code: FailureUnknown,
		},
		{
			name: "generic decode auth token stays auth",
			fn:   classifyMessage,
			msg:  "failed to decode authentication token",
			code: FailureAuth,
		},
		{
			name: "generic decode response is protocol",
			fn:   classifyMessage,
			msg:  "failed to decode response body",
			code: FailureProtocol,
		},
		{
			name: "generic network evidence beats auth token",
			fn:   classifyMessage,
			msg:  "oauth issuer connection refused",
			code: FailureNetwork,
		},
		{
			name: "http default protocol",
			fn:   classifyHTTPMessage,
			msg:  "unexpected failure",
			code: FailureProtocol,
		},
		{
			name: "http unexpected token defaults protocol",
			fn:   classifyHTTPMessage,
			msg:  "unexpected token in JSON",
			code: FailureProtocol,
		},
		{
			name: "generic filesystem",
			fn:   classifyMessage,
			msg:  "open config: permission denied",
			code: FailureFilesystem,
		},
		{
			name: "generic standalone file",
			fn:   classifyMessage,
			msg:  "file not found",
			code: FailureFilesystem,
		},
		{
			name: "http proxy network",
			fn:   classifyHTTPMessage,
			msg:  "proxy connection reset",
			code: FailureNetwork,
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
			defaultCode: FailureUnknown,
			rules: []messageRule{
				{code: FailureAuth, priority: 10, tokens: []string{"shared"}},
				{code: FailureProtocol, priority: 10, tokens: []string{"shared"}},
			},
		}
		if got := classifier.classify("shared", "source"); got.Code != FailureUnknown {
			t.Fatalf("classification = %+v, want code=%s", got, FailureUnknown)
		}
	})

	t.Run("explicit priority breaks same confidence", func(t *testing.T) {
		classifier := messageClassifier{
			defaultCode: FailureUnknown,
			rules: []messageRule{
				{code: FailureAuth, priority: 20, tokens: []string{"shared"}},
				{code: FailureProtocol, priority: 10, tokens: []string{"shared"}},
			},
		}
		if got := classifier.classify("shared", "source"); got.Code != FailureProtocol {
			t.Fatalf("classification = %+v, want code=%s", got, FailureProtocol)
		}
	})

	t.Run("weak matches are not additive", func(t *testing.T) {
		classifier := messageClassifier{
			defaultCode: FailureUnknown,
			rules: []messageRule{
				{
					code:     FailureAuth,
					priority: 10,
					tokens:   []string{"weak1", "weak2", "weak3"},
				},
				{
					code:     FailureProtocol,
					priority: 90,
					phrases:  []string{"strong signal"},
				},
			},
		}
		if got := classifier.classify("weak1 weak2 weak3 strong signal", "source"); got.Code != FailureProtocol {
			t.Fatalf("classification = %+v, want code=%s", got, FailureProtocol)
		}
	})
}

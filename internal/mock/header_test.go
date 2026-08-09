package mock

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func headerRuleOf(op restfile.MockMatchOp, values ...string) restfile.MockHeaderRule {
	return restfile.MockHeaderRule{Op: op, Values: values}
}

func TestCompileHeaderRulePredicates(t *testing.T) {
	tests := []struct {
		name string
		rule restfile.MockHeaderRule
		got  []string
		want bool
	}{
		{
			name: "exact ordered",
			rule: headerRuleOf(restfile.MockOpExact, "one", "two"),
			got:  []string{"one", "two"},
			want: true,
		},
		{
			name: "exact rejects reordered",
			rule: headerRuleOf(restfile.MockOpExact, "one", "two"),
			got:  []string{"two", "one"},
		},
		{
			name: "exact rejects a subset",
			rule: headerRuleOf(restfile.MockOpExact, "one", "two"),
			got:  []string{"one"},
		},
		{
			name: "exact rejects a missing header",
			rule: headerRuleOf(restfile.MockOpExact, "one"),
		},
		{
			name: "exact matches an empty value",
			rule: headerRuleOf(restfile.MockOpExact, ""),
			got:  []string{""},
			want: true,
		},
		{
			name: "prefix any value",
			rule: headerRuleOf(restfile.MockOpPrefix, "Bearer "),
			got:  []string{"Basic token", "Bearer token"},
			want: true,
		},
		{
			name: "prefix rejects a mid-value hit",
			rule: headerRuleOf(restfile.MockOpPrefix, "Bearer "),
			got:  []string{"token Bearer "},
		},
		{
			name: "prefix rejects a missing header",
			rule: headerRuleOf(restfile.MockOpPrefix, "Bearer "),
		},
		{
			name: "present accepts an empty value",
			rule: headerRuleOf(restfile.MockOpPresent),
			got:  []string{""},
			want: true,
		},
		{
			name: "present rejects a missing header",
			rule: headerRuleOf(restfile.MockOpPresent),
		},
		{
			name: "absent",
			rule: headerRuleOf(restfile.MockOpAbsent),
			want: true,
		},
		{
			name: "absent rejects an empty value",
			rule: headerRuleOf(restfile.MockOpAbsent),
			got:  []string{""},
		},
		{
			name: "contains anywhere in a value",
			rule: headerRuleOf(restfile.MockOpContains, "Chrome"),
			got:  []string{"Mozilla/5.0 Chrome/120 Safari/537"},
			want: true,
		},
		{
			name: "contains scans every repeated value",
			rule: headerRuleOf(restfile.MockOpContains, "Chrome"),
			got:  []string{"curl/8", "Chrome/120"},
			want: true,
		},
		{
			name: "contains is case sensitive",
			rule: headerRuleOf(restfile.MockOpContains, "Chrome"),
			got:  []string{"chrome/120"},
		},
		{
			name: "contains does not split a comma list",
			rule: headerRuleOf(restfile.MockOpContains, "gzip, br"),
			got:  []string{"gzip, br"},
			want: true,
		},
		{
			name: "contains rejects a missing header",
			rule: headerRuleOf(restfile.MockOpContains, "Chrome"),
		},
		{
			name: "regex unanchored matches a substring",
			rule: headerRuleOf(restfile.MockOpRegex, "v[0-9]+"),
			got:  []string{"api v12 beta"},
			want: true,
		},
		{
			name: "regex anchored requires the whole value",
			rule: headerRuleOf(restfile.MockOpRegex, "^v[0-9]+$"),
			got:  []string{"api v12 beta"},
		},
		{
			name: "regex anchored accepts the whole value",
			rule: headerRuleOf(restfile.MockOpRegex, "^v[0-9]+$"),
			got:  []string{"v12"},
			want: true,
		},
		{
			name: "regex scans every repeated value",
			rule: headerRuleOf(restfile.MockOpRegex, "^v[0-9]+$"),
			got:  []string{"beta", "v3"},
			want: true,
		},
		{
			name: "regex is case sensitive by default",
			rule: headerRuleOf(restfile.MockOpRegex, "^V[0-9]+$"),
			got:  []string{"v12"},
		},
		{
			name: "regex honors an inline case-insensitive flag",
			rule: headerRuleOf(restfile.MockOpRegex, "(?i)^v[0-9]+$"),
			got:  []string{"V12"},
			want: true,
		},
		{
			name: "regex can match an empty value",
			rule: headerRuleOf(restfile.MockOpRegex, "^$"),
			got:  []string{""},
			want: true,
		},
		{
			name: "regex rejects a missing header",
			rule: headerRuleOf(restfile.MockOpRegex, "^$"),
		},
		{
			name: "oneOf accepts an allowed value",
			rule: headerRuleOf(restfile.MockOpOneOf, "dev", "stage", "prod"),
			got:  []string{"stage"},
			want: true,
		},
		{
			name: "oneOf accepts any repeated value",
			rule: headerRuleOf(restfile.MockOpOneOf, "dev", "prod"),
			got:  []string{"qa", "prod"},
			want: true,
		},
		{
			name: "oneOf is exact, not a substring test",
			rule: headerRuleOf(restfile.MockOpOneOf, "dev", "prod"),
			got:  []string{"production"},
		},
		{
			name: "oneOf is case sensitive",
			rule: headerRuleOf(restfile.MockOpOneOf, "prod"),
			got:  []string{"PROD"},
		},
		{
			name: "oneOf ignores order, unlike exact",
			rule: headerRuleOf(restfile.MockOpOneOf, "a", "b"),
			got:  []string{"b", "a"},
			want: true,
		},
		{
			name: "oneOf rejects a missing header",
			rule: headerRuleOf(restfile.MockOpOneOf, "dev"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, match, err := compileHeaderRule(test.rule)
			if err != nil {
				t.Fatalf("compileHeaderRule() error = %v", err)
			}
			if got := match(test.got); got != test.want {
				t.Fatalf("predicate(%q) = %t, want %t", test.got, got, test.want)
			}
		})
	}
}

// programmatic callers never touch the parser, so compileHeaderRule has to
// reject bad rules on its own
func TestCompileHeaderRuleRejectsInvalidRules(t *testing.T) {
	tests := []struct {
		name string
		rule restfile.MockHeaderRule
		want string
	}{
		{name: "unknown op", rule: headerRuleOf(restfile.MockOpUnknown), want: "operation is invalid"},
		{name: "out of range op", rule: headerRuleOf(restfile.MockMatchOp(99)), want: "operation is invalid"},
		{
			name: "exact without values",
			rule: headerRuleOf(restfile.MockOpExact),
			want: "exact matcher requires at least one value",
		},
		{
			name: "prefix with two values",
			rule: headerRuleOf(restfile.MockOpPrefix, "a", "b"),
			want: "prefix matcher requires one non-empty value",
		},
		{
			name: "prefix with an empty value",
			rule: headerRuleOf(restfile.MockOpPrefix, ""),
			want: "prefix matcher requires one non-empty value",
		},
		{
			name: "contains with an empty value",
			rule: headerRuleOf(restfile.MockOpContains, ""),
			want: "contains matcher requires one non-empty value",
		},
		{
			name: "regex with an empty value",
			rule: headerRuleOf(restfile.MockOpRegex, ""),
			want: "regex matcher requires one non-empty value",
		},
		{
			name: "regex that does not compile",
			rule: headerRuleOf(restfile.MockOpRegex, "^v[0-9"),
			want: "not a valid regular expression",
		},
		{
			name: "oneOf without values",
			rule: headerRuleOf(restfile.MockOpOneOf),
			want: "oneOf matcher requires at least one value",
		},
		{
			name: "present with values",
			rule: headerRuleOf(restfile.MockOpPresent, "yes"),
			want: "present matcher cannot have values",
		},
		{
			name: "absent with values",
			rule: headerRuleOf(restfile.MockOpAbsent, "no"),
			want: "absent matcher cannot have values",
		},
		{
			name: "value with a newline",
			rule: headerRuleOf(restfile.MockOpContains, "a\nb"),
			want: "is not a valid header value",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := compileHeaderRule(test.rule)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("compileHeaderRule() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCompileHeaderRulesCanonicalizesAndValidatesNames(t *testing.T) {
	rules, err := compileHeaderRules(map[string]restfile.MockHeaderRule{
		"  x-tenant  ": headerRuleOf(restfile.MockOpExact, "acme"),
		"user-agent":   headerRuleOf(restfile.MockOpContains, "Chrome"),
	})
	if err != nil {
		t.Fatalf("compileHeaderRules() error = %v", err)
	}
	names := make([]string, 0, len(rules))
	for _, r := range rules {
		names = append(names, r.name)
	}
	// rules keep the source key's sort order, so "  x-tenant  " comes first
	if want := []string{"X-Tenant", "User-Agent"}; !slices.Equal(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}

	declared := rules.declared()
	if got := declared["X-Tenant"]; got.Op != restfile.MockOpExact ||
		!slices.Equal(got.Values, []string{"acme"}) {
		t.Fatalf("declared X-Tenant = %+v", got)
	}
}

func TestCompileHeaderRulesRejectsBadNames(t *testing.T) {
	tests := []struct {
		name  string
		src   map[string]restfile.MockHeaderRule
		want  string
		exact bool
	}{
		{
			name: "invalid name",
			src:  map[string]restfile.MockHeaderRule{"bad header": headerRuleOf(restfile.MockOpPresent)},
			want: `invalid mock header matcher "bad header"`,
		},
		{
			name: "empty name",
			src:  map[string]restfile.MockHeaderRule{"": headerRuleOf(restfile.MockOpPresent)},
			want: `invalid mock header matcher ""`,
		},
		{
			name: "repeated casing",
			src: map[string]restfile.MockHeaderRule{
				"x-tenant": headerRuleOf(restfile.MockOpPresent),
				"X-Tenant": headerRuleOf(restfile.MockOpAbsent),
			},
			want: "is repeated with different casing",
		},
		{
			name: "invalid rule names the header",
			src:  map[string]restfile.MockHeaderRule{"X-Env": headerRuleOf(restfile.MockOpOneOf)},
			want: `mock header matcher "X-Env": oneOf matcher requires at least one value`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := compileHeaderRules(test.src); err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("compileHeaderRules() error = %v, want %q", err, test.want)
			}
		})
	}
}

// the predicates capture the caller's values, so mutating the source map later
// must not change what a compiled mock set matches
func TestCompileHeaderRulesClonesValues(t *testing.T) {
	values := []string{"dev", "prod"}
	src := map[string]restfile.MockHeaderRule{
		"X-Env": {Op: restfile.MockOpOneOf, Values: values},
	}
	rules, err := compileHeaderRules(src)
	if err != nil {
		t.Fatalf("compileHeaderRules() error = %v", err)
	}
	values[0] = "stage"

	if !rules.matches(headerLookup(http.Header{"X-Env": []string{"dev"}}, "")) {
		t.Fatal("compiled predicate followed a later mutation of the source values")
	}
	if got := rules.declared()["X-Env"]; !slices.Equal(got.Values, []string{"dev", "prod"}) {
		t.Fatalf("declared values = %v, want the values as they were compiled", got.Values)
	}
}

func TestHeaderRulesMatchHostAndCanonicalNames(t *testing.T) {
	rules, err := compileHeaderRules(map[string]restfile.MockHeaderRule{
		"host":          headerRuleOf(restfile.MockOpContains, "example.com"),
		"CONTENT-type":  headerRuleOf(restfile.MockOpPrefix, "application/"),
		"X-Correlation": headerRuleOf(restfile.MockOpPresent),
	})
	if err != nil {
		t.Fatalf("compileHeaderRules() error = %v", err)
	}

	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("x-correlation", "abc")
	if !rules.matches(headerLookup(h, "api.example.com:8080")) {
		t.Fatal("expected the request to match")
	}
	if rules.matches(headerLookup(h, "api.other.test")) {
		t.Fatal("Host is read off the request, not the header map")
	}
}

// live routing, journal counting, and @expect share the compiled rules, so one
// source covers all three
func TestHeaderOperatorsAcrossRoutingJournalAndExpectations(t *testing.T) {
	handler := compileSource(t, `### Modern browser
# @mock method=GET path=/browser name=modern
# @match headers={"User-Agent":{"contains":"Chrome"},"X-Version":{"regex":"^v[0-9]+$"},"X-Env":{"oneOf":["dev","prod"]}}
# @expect calls=2
HTTP/1.1 200 OK

modern

### Everything else
# @mock method=GET path=/browser name=legacy default=true
HTTP/1.1 200 OK

legacy`)
	server, err := Start("127.0.0.1:0", handler, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close(context.Background()) })

	tests := []struct {
		name    string
		headers http.Header
		want    string
	}{
		{
			name: "every rule holds",
			headers: http.Header{
				"User-Agent": {"Mozilla/5.0 Chrome/120 Safari/537"},
				"X-Version":  {"v12"},
				"X-Env":      {"prod"},
			},
			want: "modern",
		},
		{
			name: "a repeated value only has to match once",
			headers: http.Header{
				"User-Agent": {"Chrome/121"},
				"X-Version":  {"v3"},
				"X-Env":      {"qa", "dev"},
			},
			want: "modern",
		},
		{
			name: "contains is case sensitive",
			headers: http.Header{
				"User-Agent": {"chrome/120"},
				"X-Version":  {"v12"},
				"X-Env":      {"prod"},
			},
			want: "legacy",
		},
		{
			name: "the regex is anchored",
			headers: http.Header{
				"User-Agent": {"Chrome/120"},
				"X-Version":  {"v12-beta"},
				"X-Env":      {"prod"},
			},
			want: "legacy",
		},
		{
			name: "oneOf is not a prefix test",
			headers: http.Header{
				"User-Agent": {"Chrome/120"},
				"X-Version":  {"v12"},
				"X-Env":      {"production"},
			},
			want: "legacy",
		},
		{
			name: "a missing header fails every value operator",
			headers: http.Header{
				"User-Agent": {"Chrome/120"},
				"X-Version":  {"v12"},
			},
			want: "legacy",
		},
	}

	client := &http.Client{Timeout: 2 * time.Second}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodGet, "http://"+server.Addr()+"/browser", nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Header = test.headers.Clone()
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(string(body)); got != test.want {
				t.Fatalf("scenario = %q, want %q", got, test.want)
			}
		})
	}

	count, err := server.Count(context.Background(), RequestPattern{
		Headers: map[string]restfile.MockHeaderRule{
			"User-Agent": headerRuleOf(restfile.MockOpContains, "Chrome"),
			"X-Version":  headerRuleOf(restfile.MockOpRegex, "^v[0-9]+$"),
			"X-Env":      headerRuleOf(restfile.MockOpOneOf, "dev", "prod"),
		},
	})
	if err != nil || count != 2 {
		t.Fatalf("journal Count() = %d, %v, want 2", count, err)
	}
	results := Verify(context.Background(), server, handler.Expectations())
	if len(results) != 1 || !results[0].Passed || results[0].Actual != 2 {
		t.Fatalf("Verify() = %+v", results)
	}
}

func TestJournalPatternReadsHostOutsideTheHeaderMap(t *testing.T) {
	journal, err := newRequestJournal(Options{})
	if err != nil {
		t.Fatal(err)
	}
	journal.add(requestRecord{
		method:  http.MethodGet,
		path:    "/x",
		host:    "api.example.com:8080",
		headers: http.Header{},
		size:    32,
	})

	for _, test := range []struct {
		name string
		rule restfile.MockHeaderRule
		want uint64
	}{
		{name: "contains", rule: headerRuleOf(restfile.MockOpContains, "example.com"), want: 1},
		{name: "regex", rule: headerRuleOf(restfile.MockOpRegex, `^api\.[a-z.]+:8080$`), want: 1},
		{name: "oneOf", rule: headerRuleOf(restfile.MockOpOneOf, "api.example.com:8080"), want: 1},
		{name: "oneOf without the port", rule: headerRuleOf(restfile.MockOpOneOf, "api.example.com")},
	} {
		t.Run(test.name, func(t *testing.T) {
			count, err := journal.count(context.Background(), RequestPattern{
				Headers: map[string]restfile.MockHeaderRule{"Host": test.rule},
			})
			if err != nil || count != test.want {
				t.Fatalf("count = %d, %v, want %d", count, err, test.want)
			}
		})
	}
}

func TestMatchRejectsSelectorHeaders(t *testing.T) {
	_, err := Compile([]*restfile.Document{parser.Parse("mocks.http", []byte(
		"# @mock method=GET path=/x\n"+
			"# @match headers={\"X-Resterm-Mock\":{\"present\":true}}\n"+
			"HTTP/1.1 200 OK"))})
	if err == nil || !strings.Contains(err.Error(), "cannot be used as a matcher") {
		t.Fatalf("Compile() error = %v", err)
	}
}

func TestMatchRejectsSelectorHeaderSpellings(t *testing.T) {
	for _, name := range []string{"  x-resterm-mock  ", "X-RESTERM-MOCK", "\tx-Resterm-Mock-Status"} {
		t.Run(name, func(t *testing.T) {
			_, err := newMatchers(restfile.MockMatch{
				Headers: map[string]restfile.MockHeaderRule{
					name: headerRuleOf(restfile.MockOpPresent),
				},
			})
			if err == nil || !strings.Contains(err.Error(), "cannot be used as a matcher") {
				t.Fatalf("newMatchers() error = %v", err)
			}
		})
	}
}

func TestMatchKeepsOtherHeaderMatchers(t *testing.T) {
	rules, err := compileMatchHeaders(map[string]restfile.MockHeaderRule{
		"  x-resterm-mocked  ": headerRuleOf(restfile.MockOpExact, "yes"),
	})
	if err != nil {
		t.Fatalf("compileMatchHeaders() error = %v", err)
	}
	if !rules.matches(headerLookup(http.Header{"X-Resterm-Mocked": []string{"yes"}}, "")) {
		t.Fatal("X-Resterm-Mocked did not match")
	}
}

// a broken matcher has to fail the whole reload, so the caller keeps serving the
// mock set that last compiled
func TestInvalidHeaderRuleFailsReloadAndKeepsTheLastHandler(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "mocks.http")
	writeFile(t, path, `# @mock method=GET path=/browser
# @match headers={"X-Version":{"regex":"^v[0-9]+$"}}
HTTP/1.1 200 OK

first`)

	reloader := NewReloader(Sources{Path: root})
	handler, err := reloader.Reload("", nil)
	if err != nil || handler == nil {
		t.Fatalf("initial reload = %v, %v", handler, err)
	}

	writeFile(t, path, `# @mock method=GET path=/browser
# @match headers={"X-Version":{"regex":"^v[0-9"}}
HTTP/1.1 200 OK

second`)
	next, err := reloader.Reload("", nil)
	if err == nil || next != nil {
		t.Fatalf("broken reload = %v, %v, want an error and no handler", next, err)
	}
	if !strings.Contains(err.Error(), "not a valid regular expression") {
		t.Fatalf("reload error = %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/browser", nil)
	request.Header.Set("X-Version", "v1")
	assertResponse(t, handler, request, http.StatusOK, "first")
}

func TestHeaderRulesMatchEmptyIsAlwaysTrue(t *testing.T) {
	rules, err := compileHeaderRules(nil)
	if err != nil {
		t.Fatalf("compileHeaderRules() error = %v", err)
	}
	if len(rules) != 0 || rules.declared() != nil {
		t.Fatalf("rules = %+v, want empty", rules)
	}
	if !rules.matches(headerLookup(http.Header{}, "")) {
		t.Fatal("an empty rule set must match everything")
	}
}

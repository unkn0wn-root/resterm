package mock

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestEqualJSONNumbers(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"1", "1", true},
		{"100", "1e2", true},
		{"1.5", "1.50", true},
		{"9007199254740993", "9.007199254740993e15", true}, // same value, different form
		{"9007199254740993", "9007199254740992", false},    // adjacent ints stay distinct
		{"9007199254740993", "9007199254740992.0", false},  // ...even when one side is a decimal
		{"1e100", "10e99", true},
		{"1", "2", false},
		{"1", "1e999999999", false}, // runaway exponent stays cheap (ParseFloat -> +Inf)
		{"1e999999999", "1", false},
		{"1e999999999", "1e999999999", true},  // byte-identical short-circuits before Inf
		{"1e999999999", "2e999999999", false}, // distinct overflows never compare equal
	}
	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			if got := equalJSONNumbers(json.Number(tt.a), json.Number(tt.b)); got != tt.want {
				t.Fatalf("equalJSONNumbers(%q, %q) = %t, want %t", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestMatchHeaderRule(t *testing.T) {
	tests := []struct {
		name string
		got  []string
		rule restfile.MockHeaderRule
		want bool
	}{
		{
			name: "exact ordered",
			got:  []string{"one", "two"},
			rule: restfile.MockHeaderRule{Op: restfile.MockHeaderOpExact, Values: []string{"one", "two"}},
			want: true,
		},
		{
			name: "exact rejects reordered",
			got:  []string{"two", "one"},
			rule: restfile.MockHeaderRule{Op: restfile.MockHeaderOpExact, Values: []string{"one", "two"}},
		},
		{
			name: "prefix any value",
			got:  []string{"Basic token", "Bearer token"},
			rule: restfile.MockHeaderRule{Op: restfile.MockHeaderOpPrefix, Values: []string{"Bearer "}},
			want: true,
		},
		{
			name: "present accepts empty value",
			got:  []string{""},
			rule: restfile.MockHeaderRule{Op: restfile.MockHeaderOpPresent},
			want: true,
		},
		{
			name: "absent",
			rule: restfile.MockHeaderRule{Op: restfile.MockHeaderOpAbsent},
			want: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := matchHeaderRule(test.got, test.rule); got != test.want {
				t.Fatalf("matchHeaderRule() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestMatchJSONNumberHugeExponentDoesNotBlowUp(t *testing.T) {
	handler := compileSource(t, `# @mock method=POST path=/n
# @match json={"n":1}
HTTP/1.1 200 OK

matched`)

	req := httptest.NewRequest(http.MethodPost, "/n", strings.NewReader(`{"n":1e999999999}`))
	req.Header.Set("Content-Type", "application/json")
	assertResponse(t, handler, req, http.StatusNotFound, "no mock scenario matched")
}

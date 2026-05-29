package ui

import "testing"

func TestCleanStatusUsername(t *testing.T) {
	tests := map[string]string{
		`DOMAIN\david`: "david",
		"domain/david": `david`,
		" david ":      "david",
		"":             "",
	}

	for input, want := range tests {
		if got := cleanStatusUsername(input); got != want {
			t.Fatalf("cleanStatusUsername(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCleanStatusHost(t *testing.T) {
	tests := map[string]string{
		"workstation.local": "workstation",
		"host":              "host",
		" host ":            "host",
		"":                  "",
	}

	for input, want := range tests {
		if got := cleanStatusHost(input); got != want {
			t.Fatalf("cleanStatusHost(%q) = %q, want %q", input, got, want)
		}
	}
}

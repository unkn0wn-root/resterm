package collection

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRedactGroupedEnvironmentPreservesSchema(t *testing.T) {
	raw := []byte(`{
		" $ShArEd ": {
			"region": "eu",
			"nested": {"secret": "shared"}
		},
		"$GROUPS": {
			"Api": {
				" $DeFaUlt ": "Dev",
				"Dev": {
					"url": "https://api",
					"flags": [true, 2]
				}
			}
		}
	}`)
	data, err := redactEnv(raw, "resterm.env.json")
	if err != nil {
		t.Fatalf("redact environment: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("decode redacted environment: %v", err)
	}

	shared := root[" $ShArEd "].(map[string]any)
	if shared["region"] != envPlaceholder ||
		shared["nested"].(map[string]any)["secret"] != envPlaceholder {
		t.Fatalf("shared values were not redacted: %#v", shared)
	}
	groups := root["$GROUPS"].(map[string]any)
	api := groups["Api"].(map[string]any)
	if api[" $DeFaUlt "] != "Dev" {
		t.Fatalf("default = %#v, want Dev", api[" $DeFaUlt "])
	}
	dev := api["Dev"].(map[string]any)
	if dev["url"] != envPlaceholder {
		t.Fatalf("profile URL = %#v, want placeholder", dev["url"])
	}
	flags := dev["flags"].([]any)
	if flags[0] != envPlaceholder || flags[1] != envPlaceholder {
		t.Fatalf("profile flags = %#v, want placeholders", flags)
	}
}

func TestBuildEnvTemplatePrefersHTTPClient(t *testing.T) {
	root := t.TempDir()
	// http-client.env.json, when present, feeds the exported template.
	if err := os.WriteFile(filepath.Join(root, "http-client.env.json"),
		[]byte(`{"dev":{"url":"https://api","token":"supersecret"}}`), 0o644); err != nil {
		t.Fatalf("write http-client env: %v", err)
	}

	data, err := buildEnvTemplate(root, root)
	if err != nil {
		t.Fatalf("build env template: %v", err)
	}
	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("decode template: %v", err)
	}
	dev := v["dev"].(map[string]any)
	if dev["url"] != envPlaceholder || dev["token"] != envPlaceholder {
		t.Fatalf("values not redacted: %#v", dev)
	}
}

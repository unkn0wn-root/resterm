package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restwriter"
)

func TestRenderCurlOutput(t *testing.T) {
	cmd := "curl https://example.com -H 'X-Test: 1' -u user:pass -k --silent"
	doc, warn, err := buildDoc(cmd)
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}

	hdr := buildHeader("", cmd, warn)
	got := restwriter.Render(doc, restwriter.Options{HeaderComment: hdr})

	wantPath := filepath.Join("testdata", "basic.http")
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}

	if strings.TrimSpace(got) != strings.TrimSpace(string(want)) {
		t.Fatalf("output mismatch.\nGot:\n%s\nWant:\n%s", got, string(want))
	}
}

func TestRenderCurlOutputAdvanced(t *testing.T) {
	cmd := `curl --compressed -G "https://api.example.com/search?existing=1" \
  -d 'q=hello' --data-urlencode 'note=hello world' \
  -H 'X-Flag: yes' -A 'resterm-agent' -e 'https://ref.example.com' -b 'a=b' -u user:pass \
  -k -L -x http://proxy.local --max-time 2.5 --connect-timeout 3 --max-redirs 7 --retry 2 \
  --retry-delay 1.5 --retry-max-time 9 --retry-connrefused \
  --http1.1 --http2 --http2-prior-knowledge --http3 \
  --resolve example.com:443:1.2.3.4 --connect-to example.com:443:alt.example.com:444 \
  --interface eth0 --dns-servers 1.1.1.1,8.8.8.8 \
  -o out.txt -D headers.txt --stderr err.log --trace trace.log --trace-ascii trace.txt \
  -i -v -S -O --cacert /tmp/ca.pem --cert /tmp/client.pem --key /tmp/client.key \
  --next https://api.example.com/json --json '{"ok":true}' -H 'X-Json: 1' \
  --next https://api.example.com/upload -T payload.bin -s

sudo -u root -p "prompt here" curl https://api.example.com/multi \
  -H 'X-Multi: 1' \
  -F file=@payload.json -F caption=hello --form-string memo='Testing resterm' --silent`
	doc, warn, err := buildDoc(cmd)
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}

	hdr := buildHeader("", cmd, warn)
	got := restwriter.Render(doc, restwriter.Options{HeaderComment: hdr})

	wantPath := filepath.Join("testdata", "advanced.http")
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}

	if strings.TrimSpace(got) != strings.TrimSpace(string(want)) {
		t.Fatalf("output mismatch.\nGot:\n%s\nWant:\n%s", got, string(want))
	}
}

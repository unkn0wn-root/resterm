package initcmd

import (
	"sort"
	"strings"
	"unicode/utf8"
)

var helpMD = buildHelpMD()

type tplCache struct {
	list  []template
	by    map[string]template
	names []string
	width int
}

var tc = newTplCache()

func newTplCache() tplCache {
	list := []template{
		{
			Name:        "minimal",
			Description: describeTemplate(fileRequests, fileEnv),
			Files: []fileSpec{
				{Path: fileRequests, Data: reqHTTPMinimal, Mode: filePerm},
				{Path: fileEnv, Data: envJSON, Mode: filePerm},
			},
			AddGitignore: true,
		},
		{
			Name:        "standard",
			Description: describeTemplate(fileRequests, fileEnv, fileEnvExample, fileRTSHelpers, fileHelp),
			Files: []fileSpec{
				{Path: fileRequests, Data: reqHTTPStandard, Mode: filePerm},
				{Path: fileEnv, Data: envJSON, Mode: filePerm},
				{Path: fileEnvExample, Data: envExampleJSON, Mode: filePerm},
				{Path: fileRTSHelpers, Data: helpersRTS, Mode: filePerm},
				{Path: fileHelp, Data: helpMD, Mode: filePerm},
			},
			AddGitignore: true,
		},
	}
	by := make(map[string]template, len(list))
	names := make([]string, 0, len(list))
	width := 0
	for _, t := range list {
		by[t.Name] = t
		names = append(names, t.Name)
		if w := utf8.RuneCountInString(t.Name); w > width {
			width = w
		}
	}
	sort.Strings(names)
	return tplCache{list: list, by: by, names: names, width: width}
}

func templateList() []template {
	return cloneTemplates(tc.list)
}

func findTemplate(name string) (template, bool) {
	t, ok := tc.by[name]
	if !ok {
		return template{}, false
	}
	return cloneTemplate(t), true
}

func templateNames() []string {
	out := make([]string, len(tc.names))
	copy(out, tc.names)
	return out
}

func templateWidth() int {
	return tc.width
}

func describeTemplate(files ...string) string {
	return strings.Join(files, " + ")
}

func cloneTemplates(src []template) []template {
	if len(src) == 0 {
		return nil
	}
	out := make([]template, len(src))
	for i, t := range src {
		out[i] = cloneTemplate(t)
	}
	return out
}

func cloneTemplate(t template) template {
	out := t
	if len(t.Files) == 0 {
		return out
	}
	files := make([]fileSpec, len(t.Files))
	copy(files, t.Files)
	out.Files = files
	return out
}

const reqHTTPMinimal = `### Health check
# @name Health
GET {{base.url}}/status/200

### Echo JSON
# @name Echo
POST {{base.url}}/post
Content-Type: application/json
Authorization: Bearer {{auth.token}}

{
  "hello": "resterm",
  "time": "{{$timestampISO8601}}"
}

### Capture value from response
# @name CaptureToken
# @capture file-secret auth.token {{response.json.uuid}}
GET {{base.url}}/uuid

### Reuse captured value
# @name UseToken
GET {{base.url}}/anything
Authorization: Bearer {{auth.token}}

### Query params
# @name Query
# @var request hello resterm
GET {{base.url}}/get?hello={{hello}}
`

const reqHTTPStandardSuffix = `
### Scripted header
# @name ScriptedHeader
GET {{base.url}}/anything
Authorization: {{= helpers.authHeader(vars.get("auth.token"), env.get("auth.token")) }}

### Use last response
# @name UseLast
# Uses last.* from the most recent request in this session.
# Run CaptureToken first, otherwise last.json("uuid") is empty.
GET {{base.url}}/anything
X-Last-UUID: {{= last.json("uuid") ?? "" }}
`

const reqHTTPStandard = "# @use ./rts/helpers.rts\n\n" + reqHTTPMinimal + reqHTTPStandardSuffix

const envJSON = `{
  "dev": {
    "base": {
      "url": "https://httpbin.org"
    },
    "auth": {
      "token": "dev-token-123"
    }
  },
  "prod": {
    "base": {
      "url": "https://api.example.com"
    },
    "auth": {
      "token": "prod-token-xyz"
    }
  }
}
`

const envExampleJSON = `{
  "dev": {
    "base": {
      "url": "https://httpbin.org"
    },
    "auth": {
      "token": "REPLACE_ME"
    }
  },
  "prod": {
    "base": {
      "url": "https://api.example.com"
    },
    "auth": {
      "token": "REPLACE_ME"
    }
  }
}
`

const helpersRTS = `module helpers

export fn authHeader(primary, fallback) {
  let token = primary ?? fallback
  return token ? "Bearer " + token : ""
}
`

func buildHelpMD() string {
	var b strings.Builder
	b.WriteString("# Resterm quickstart\n\n")
	b.WriteString("1. Run `resterm` in this folder.\n")
	b.WriteString("2. Press Ctrl+E to switch environments.\n")
	b.WriteString("3. Open `")
	b.WriteString(fileRequests)
	b.WriteString("`, place the cursor inside a request, then press Ctrl+Enter.\n")
	b.WriteString("4. Edit `")
	b.WriteString(fileEnv)
	b.WriteString("` or copy from `")
	b.WriteString(fileEnvExample)
	b.WriteString("`.\n\n")
	b.WriteString("Next steps:\n")
	b.WriteString("- Captures let you store response values for later requests.\n")
	b.WriteString("- Workflows chain requests with shared context.\n")
	b.WriteString("- `")
	b.WriteString(fileRTSHelpers)
	b.WriteString("` shows a RestermScript module example.\n")
	b.WriteString("- See docs in [docs/resterm.md](https://github.com/unkn0wn-root/resterm/blob/main/docs/resterm.md) for details.\n")
	return b.String()
}

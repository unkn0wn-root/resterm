package initcmd

import (
	"sort"
	"strings"
	"unicode/utf8"
)

type TemplateStore interface {
	Find(name string) (template, bool)
	List() []template
	Names() []string
	Width() int
}

type BuiltinTemplates struct{}

func (BuiltinTemplates) Find(name string) (template, bool) { return findTemplate(name) }
func (BuiltinTemplates) List() []template                  { return templateList() }
func (BuiltinTemplates) Names() []string                   { return templateNames() }
func (BuiltinTemplates) Width() int                        { return templateWidth() }

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
			Name: "standard",
			Description: describeTemplate(
				fileRequests,
				fileEnv,
				fileEnvExample,
				fileRTSHelpers,
				fileHelp,
			),
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

const reqHTTPMinimal = `# This starter is entirely local. In the TUI, press g Shift+M to start the
# mock server, then place the cursor in a request and press Ctrl+Enter.

### Mock: say hello
# This matcher checks one field and ignores other JSON fields.
# @mock method=POST path=/hello
# @match json={"kind":"greeting"}
HTTP/1.1 200 OK
Content-Type: application/json

{"message":"hello from the mock","name":{{json.body.name}}}

### Mock: create an adult member
# Both the literal JSON field and the numeric rule must match.
# @mock method=POST path=/users
# @match headers={"Authorization":{"prefix":"Bearer "}} json={"role":"member"}
# @match json-rules={"age":{"gte":18}}
HTTP/1.1 201 Created
Content-Type: application/json

{
  "id": {{json.body.id}},
  "name": {{json.body.name}},
  "role": {{json.body.role}},
  "displayName": {{json.body.displayName}},
  "status": {{json.body.status}},
  "authorization": {{json.headers.Authorization}}
}

### Mock: reject invalid users
# @mock method=POST path=/users default=true
HTTP/1.1 422 Unprocessable Entity
Content-Type: application/json

{"error":"Bearer auth and a member aged 18 or older are required"}

### Say hello
# @name Hello
# @assert response.statusCode == 200
# @assert response.json("name") == "Resterm"
POST {{base.url}}/hello
Content-Type: application/json

{"kind":"greeting","name":"Resterm"}

### Create a user
# @name CreateUser
# @auth bearer {{auth.token}}
# @assert response.statusCode == 201
# @assert response.json("id") == "user-1"
# @assert response.json("authorization") == "Bearer " + env.get("auth.token")
POST {{base.url}}/users
Content-Type: application/json

{
  "id": "user-1",
  "name": "Ada",
  "role": "member",
  "age": 36,
  "displayName": "Ada",
  "status": "active"
}
`

const reqHTTPStandardSuffix = `
### Create users from a list of names
# @name CreateUsersFromNames
# @for-each ["david","damian","bob"] as name
# @auth bearer {{auth.token}}
# @assert response.statusCode == 201
# @assert response.json("name") == name
POST {{base.url}}/users
Content-Type: application/json

{
  "id": "user-{{= name }}",
  "name": "{{= name }}",
  "role": "member",
  "age": 18,
  "displayName": "{{= name }}",
  "status": "active"
}

### Create users from RestermScript objects
# helpers.users() comes from rts/helpers.rts.
# @name CreateUsersFromModule
# @for-each helpers.users() as user
# @auth bearer {{auth.token}}
# @assert response.statusCode == 201
# @assert response.json("id") == user.id
# @assert response.json("displayName") == (user["nickname"] ?? user.name)
# @assert response.json("status") == (user.active ? "active" : "inactive")
POST {{base.url}}/users
Content-Type: application/json

{{= helpers.userPayload(user) }}
`

const reqHTTPStandard = "# @use ./rts/helpers.rts\n\n" + reqHTTPMinimal + reqHTTPStandardSuffix

const envJSON = `{
  "$shared": {
    "base": {
      "url": "http://127.0.0.1:8080"
    }
  },
  "dev": {
    "auth": {
      "token": "dev-token-123"
    }
  },
  "test": {
    "auth": {
      "token": "test-token-456"
    }
  }
}
`

const envExampleJSON = `{
  "$shared": {
    "base": {
      "url": "http://127.0.0.1:8080"
    }
  },
  "dev": {
    "auth": {
      "token": "REPLACE_ME"
    }
  },
  "test": {
    "auth": {
      "token": "REPLACE_ME"
    }
  }
}
`

const helpersRTS = `module helpers

export fn users() {
  return [
    {id: "user-2", name: "Grace", age: 28, active: true},
    {id: "user-3", name: "Linus", nickname: "Lin", age: 24, active: false}
  ]
}

export fn userPayload(user) {
  return {
    id: user.id,
    name: user.name,
    role: user["role"] ?? "member",
    age: user.age,
    displayName: user["nickname"] ?? user.name,
    status: user.active ? "active" : "inactive"
  }
}
`

func buildHelpMD() string {
	var b strings.Builder
	b.WriteString("# Resterm quickstart\n\n")
	b.WriteString("1. Run `resterm` in this directory.\n")
	b.WriteString("2. Press `g Shift+M` to start the local mock server.\n")
	b.WriteString("3. Open `")
	b.WriteString(fileRequests)
	b.WriteString("`, place the cursor inside a request, then press Ctrl+Enter.\n")
	b.WriteString("4. Press Ctrl+E to switch between the local `dev` and `test` environments.\n")
	b.WriteString("5. Edit `")
	b.WriteString(fileEnv)
	b.WriteString("` or copy from `")
	b.WriteString(fileEnvExample)
	b.WriteString("`.\n\n")
	b.WriteString(
		"For CLI runs, start `resterm mock requests.http` in one terminal, then run requests from another.\n\n",
	)
	b.WriteString("Next steps:\n")
	b.WriteString("- The mock scenarios demonstrate JSON matching, numeric rules, and response interpolation.\n")
	b.WriteString("- The requests demonstrate assertions, bearer auth, and two forms of `@for-each`.\n")
	b.WriteString("- `")
	b.WriteString(fileRTSHelpers)
	b.WriteString("` shows object lists, `??`, and the ternary operator.\n")
	b.WriteString(
		"- See docs in [docs/resterm.md](https://github.com/unkn0wn-root/resterm/blob/main/docs/resterm.md) for details.\n",
	)
	return b.String()
}

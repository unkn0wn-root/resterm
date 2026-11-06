# Request files in depth

Read this when a request file needs more than names, variables, captures and `@assert`.

## Environment files

- Resterm looks for `resterm.env.json` or `rest-client.env.json` in the request file's directory, then the workspace root, then the current directory, and loads the first one it finds.
- Each top-level key is an environment, chosen with `--env <name>`. Without `--env`, Resterm uses `dev`, `default` or `local`, in that order, when one exists. Pass `--env` so a run does not depend on that.
- Nested objects and arrays are flattened, so `{"services": {"api": {"base": "..."}}}` is `{{services.api.base}}` and the first item of an `addons` array is `{{addons[0]}}`.
- A `$shared` key holds values for every environment. An environment's own value wins over `$shared`.
- `"env:NAME"` reads the OS variable `NAME` at the start of each request and marks the value as a secret. A missing OS variable leaves the value undefined. It does not fall back to another source.
- An invalid environment file stops `resterm run` before any request is sent.

Grouped environments combine independent choices, such as API endpoint and credentials. A grouped file has a `$groups` object, each group has profiles and a `$default`, and `--env-group <group>=<profile>` picks a profile. Repeat the flag for more groups. `--env` and `--env-group` cannot be combined.

A dotenv file works for a single environment. Pass it with `--env-file path/to/.env`. Resterm never loads a dotenv file on its own.

## Variables

| Declaration | Scope |
| --- | --- |
| `# @const name value` | The whole file. It cannot change at runtime. |
| `# @file name value` | Every request in this file. |
| `# @request name value` | This request only. |
| `# @global name value` | Every request in every file, for the selected environment. |
| `# @run var name = value` | One value shared by every step of a workflow run. |

- `-secret` forms such as `# @file-secret`, `# @request-secret` and `# @global-secret` mask the value in output.
- Values are taken as written. Quotes are kept as part of the value.
- Names ignore case.
- Lookup order, first match wins: `@const`, values set by scripts, workflow step values and loop variables, `@run var`, request values and request captures, runtime globals, `@global`, file values and file captures, the selected environment, then OS environment variables.
- A prefix reads one source only, such as `{{file.token}}`, `{{request.id}}`, `{{environment.baseUrl}}` or `{{env.HOME}}`. `env.` reads the OS variable, and that value is not a secret.
- `{{global.token}}` reads globals saved by captures or scripts, not `# @global`. Read a `# @global` value with `{{document-global.token}}` or a plain `{{token}}`.
- A variable that refers to itself, directly or through others, fails the request with a `variable cycle` error and exit `26`.

Generated values, each new on every use:

- `{{$uuid}}`, `{{$timestamp}}`, `{{$timestampMs}}`, `{{$timestampISO8601}}`
- `{{$randomInt}}`, `{{$randomInt(1, 6)}}`, `{{$randomString(24)}}`, `{{$randomChoice("a", "b")}}`
- `{{$fake.email}}`, `{{$fake.person}}`, `{{$fake.company}}` and other `$fake` fields
- Offsets on timestamps: `{{$timestamp + 6d}}`, `{{$timestampISO8601 - 90m}}`

To reuse one generated value inside a request, declare it: `# @request trace.id {{$uuid}}`. To share it across a workflow, use `# @run var`.

`{{= expr }}` evaluates a RestermScript expression when the request runs, for example `{{= url.encode(vars.get("search")) }}`.

## Captures

`# @capture <scope> <name> = <expression>` saves a value after the response arrives.

- Scopes are `request`, `file` and `global`, each with a `-secret` form such as `file-secret`.
- The expression is RestermScript: `response.statusCode`, `response.header("X-Request-Id")`, `response.text()`, or `response.json.items[0].id`. The `response.json.path` shorthand works only in captures. Elsewhere write `response.json().path`.
- A template such as `{{response.json.token}}` or `{{response.headers.X-Request-Id}}` also works.
- Captures run top to bottom and can read earlier ones. Values are saved only when every capture in the request succeeds.
- A missing JSON path gives an empty string. `# @setting capture.strict true` makes a template capture such as `{{response.json.token}}` fail the request when the path is missing. It does not change RTS captures such as `response.json.token`, so check those values with `@assert`, for example `# @assert response.json().token != null`.

## Checks

- `# @assert <expression>` passes when the expression is true. `# @assert <expression> => "message"` shows the message on failure. Use `response` for the current response.
- `# @rts test "name"` starts a RestermScript test block. Each code line starts with `>`. A block can also be `> {%` to `%}` on their own lines, or `> < ./checks.rts` to load a file. `t.error(...)` records a failure and continues. `t.fatal(...)` stops the block.
- `# @script test` runs a JavaScript test block. Use `client.test("name", fn)`, `tests.assert(condition, "message")` and `tests.fail("message")`.
- Checks run in this order: `@assert`, RestermScript tests, JavaScript tests. A failed check makes `resterm run` exit `1`. Checks that run past the request timeout exit `20`, and other script errors exit `24`.
- `# @rts pre-request` and `# @script pre-request` blocks change the request before it is sent, for example `request.setHeader("X-Id", uuid())`.

## Conditions, retries and polling

- `# @when <expression>` runs the request only when the expression is true. `# @skip-if <expression>` skips it when true. A skipped request has `status` `skip` and a `skipReason`. These run before the request, so use `last` for the previous response, not `response`.
- `# @poll every=500ms timeout=30s until=response.json().status == "done"` repeats the request until the condition holds. `until` must come last.
- `# @retry count=3` retries after a network error or timeout. Add `# @retry-when contains([429, 503], response.statusCode)` to retry on a status, and `# @retry-backoff exponential(100ms, 2s) jitter=20%` to set the delay. Be careful with requests that change data, because the server may have handled an attempt whose response was lost.
- Options are `key=value` with no spaces around `=`, and each option may appear once.

## Workflows

A workflow runs named requests in order. Run it with `resterm run --workflow <name> <file>`.

```http
### Sign up
# @workflow sign-up on-failure=stop
# @run var email = user-{{$randomInt(1000, 9999)}}@example.com
# @step Register using=register expect.statuscode=201
# @step Fetch using=getUser expect.statuscode=200

### Register
# @name register
# @capture file userId = response.json.id
POST {{baseUrl}}/users
Content-Type: application/json

{"email": "{{email}}"}

### Get user
# @name getUser
# @assert response.json().email == vars.get("email")
GET {{baseUrl}}/users/{{userId}}
```

- `# @workflow <name>` starts the block. `on-failure=continue` keeps going after a failed step. The default is to stop.
- `# @step <alias> using=<request>` runs the request with that `# @name`. `expect.statuscode=201` and `expect.status="201 Created"` check the status. `vars.request.<key>=<value>` sets `{{vars.request.<key>}}` for that step, and `vars.workflow.<key>=<value>` sets `{{vars.workflow.<key>}}` for later steps.
- `# @if <expression> run=<request>`, `# @elif` and `# @else fail="message"` choose a request to run. `# @switch <expression>` with `# @case <value> run=<request>` and `# @default` does the same by value.
- Put `# @when`, `# @skip-if` or `# @for-each` directly above a `# @step` to apply them to that step.
- Between `@workflow` and the next request, an unknown directive is a parse error, not a warning. A `# @name` line or any plain text ends the workflow block.
- In the JSON report, a workflow is one result with `kind` `workflow`, and each step is in `steps[]`.

## Loops, compare runs and profiling

`# @for-each <list> as <name>` sends the request once per item. The list is a RestermScript expression, such as a literal or `json.file("users.json")`. Read the item with `{{= name }}`.

```http
### Create users
# @name createUsers
# @for-each [{"name": "Ada"}, {"name": "Lin"}] as user
# @assert response.statusCode == 201
POST {{baseUrl}}/users
Content-Type: application/json

{"name": "{{= user.name }}"}
```

`# @compare dev stage base=dev` sends the request once per environment and compares the results with the `base` one. `--compare dev,stage` and `--compare-base dev` do the same from the command line.

```http
### Health in every environment
# @name health
# @compare dev stage prod base=dev
GET {{baseUrl}}/health
```

`# @profile count=50 warmup=5 delay=100ms` sends the request many times and reports latency. A failed run or a status of 400 or more fails the profile. Profiling does not work on gRPC requests.

## Authentication

| Directive | Effect |
| --- | --- |
| `# @auth bearer {{token}}` | Sends `Authorization: Bearer ...`. |
| `# @auth basic {{user}} {{password}}` | Sends HTTP Basic credentials. |
| `# @auth apikey header X-API-Key {{key}}` | Sends a key in a header. Use `query` instead of `header` for a query parameter. |
| `# @auth oauth2 token_url=... client_id=... client_secret=...` | Fetches, caches and refreshes an OAuth 2.0 token. |
| `# @auth command cmd="gh auth token"` | Runs a program and sends what it prints as the token. |
| `# @auth none` | Turns off inherited auth for one request. |

- `# @auth file ...` above the first `###` applies to every request in the file.
- `@auth command` runs a program on the user's machine. Check the command before you run the file.
- `resterm run` never opens a browser. An OAuth 2.0 `authorization_code` request needs a cached token that is still valid or can be refreshed. Otherwise it fails with the reason.
- `--persist-auth` keeps tokens between runs. See [run.md](run.md).

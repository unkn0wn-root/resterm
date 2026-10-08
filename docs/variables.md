# Variables and environments

## Environment files

Resterm automatically searches, in order:

1. The directory of the opened file.
2. The workspace root.
3. The current working directory.

It loads the first `resterm.env.json` or `rest-client.env.json` it finds. Each named environment must be an object. Values inside it can contain nested objects and arrays, which are flattened using dot and bracket notation (`services.api.base`, `plans.addons[0]`).

Resterm uses one environment file per workspace. Opening another request *file* inside that workspace keeps the same environment file. This keeps globals, file variables, cookies, and history tied to the same selection.

For example, `resterm requests/api.http` picks up `requests/resterm.env.json`. Opening that file from a workspace root that has its own environment file keeps the root's environment instead. In a recursive workspace, Resterm warns at startup about environment files it will not load. To use one of those files, start Resterm in its directory or pass `--env-file`.

Opening a *workspace*, or a request file that lives outside the current one, moves the workspace and re-resolves the environment for the new root:

- Resterm looks for the environment you last chose. A session started with `--env prod` looks for `prod` in the new workspace. If you later choose one with `Ctrl+E`, that choice carries across instead. Resterm remembers the requested choice even if it could not select it in the previous workspace.
- If that environment is missing, Resterm loads the available environments but leaves nothing selected. Using the new workspace's default could accidentally switch a `dev` session to `prod`. The header reads `ENV: none selected`. Use `Ctrl+E` to choose an environment before sending a request.
- The last response is cleared. Globals, file variables, cookie jars, OAuth tokens, and command auth are kept for the workspace they came from. Resterm separates runtime values by both the full environment selection and the environment file. The new workspace cannot read the old workspace's values; switching back restores the old session without another login.

  Two workspaces using the same `--env-file` share those values. A workspace without an environment file loses its runtime values when you leave, because there is no environment file to identify them by. Environments supplied through the Go API work the same way.
- A file passed with `--env-file` remains active when the workspace changes. Resterm warns if the new workspace has its own environment file. Without `--env-file`, moving to a workspace with no environment file clears the current environment. If the new workspace's environment file cannot load, the move is refused and the current workspace and environment remain active.
- A move is refused while a request is running, because that request can still write runtime values back and undo the reset. Finish or cancel it first.

Example environment (`_examples/resterm.env.json`):

```json
{
  "dev": {
    "settings.http-root-cas": "dev-ca.pem",
    "settings.grpc-insecure": "false",
    "services": {
      "api": {
        "base": "https://httpbin.org/anything/api"
      }
    },
    "auth": {
      "token": "dev-token-123"
    }
  }
}
```

Switch environments with `Ctrl+E`. If multiple environments exist, Resterm defaults to `dev`, `default`, or `local` when available.

### Values from OS environment variables

Use `env:NAME` when a value should come from an OS environment variable. This keeps the value itself out of `resterm.env.json`:

```json
{
  "dev": {
    "auth": {
      "token": "env:RESTERM_TOKEN"
    }
  }
}
```

Resterm reads the OS variable once at the start of each request and exposes it under the declared name. In this example, `{{auth.token}}`, `env.get("auth.token")`, and `vars.get("auth.token")` return the same value. `RESTERM_TOKEN` itself is not added to `env` or `vars`.

You can also use `env:NAME` in request files:

```http
# @file token env:RESTERM_TOKEN

### Reports
# @name Reports
GET https://api.example.com/reports
Authorization: Bearer {{token}}
```

`@const`, `@request`, `@global`, and `@file` all accept this form. The mapped value is available through templates and `vars`. Constants stay template-only. File declarations do not appear in `env`, which only contains values from the selected environment.

If the OS variable is missing, the declared value stays undefined. Resterm does not fall back to another source with the same name. It first tries the OS variable's name as written, then its uppercase form.

Values loaded through `env:NAME` are secrets. Resterm hides them from previews and redacts them from results, explain output, and history. References are resolved once, so an OS value that contains `env:OTHER` stays unchanged. Only declarations are interpreted as references. Values from captures, workflows, and scripts are plain data.

An empty `env:` reference is an error. In a request file, Resterm reports the line where it appears. In an environment file, it stops `resterm run`. The TUI still starts so you can fix the file, but it blocks all requests until the file loads successfully.

A reference name may contain a template, as in `env:{{picked}}`. Only declarations can supply `picked`. Runtime data cannot choose which OS variable Resterm reads. If a capture replaces the declaration that supplied the name, nothing declares it any more and the reference becomes undefined rather than following the captured value.

### Shared variables (`$shared`)

Use the reserved `$shared` key to define variables that apply to **all** environments. This avoids repeating common values, such as auth credentials and token URLs, in every environment. Environment-specific values override `$shared` when names collide.

```json
{
  "$shared": {
    "api": { "version": "v2" },
    "auth": { "clientId": "demo-client" }
  },
  "dev": {
    "base": { "url": "https://dev.example.com" }
  },
  "prod": {
    "base": { "url": "https://prod.example.com" },
    "auth": { "clientId": "prod-client" }
  }
}
```

In this example `dev` inherits `auth.clientId=demo-client` from `$shared`, while `prod` overrides it with `prod-client`. Both environments receive `api.version=v2`. The `$shared` key itself never appears in the environment selector.

### Grouped environments

Use groups when you want to combine independent choices, like API endpoint, app, and credentials, without writing out every combination as its own environment. A file either defines named environments or groups. The two forms cannot be mixed.

A runnable sample is in `_examples/grouped/`. Its `resterm.env.json` declares three groups of 3 profiles, so 9 declarations cover 27 combinations, and `grouped-environments.http` has one request per group plus one that reads from all three.

```json
{
  "$shared": {
    "region": "eu"
  },
  "$groups": {
    "api": {
      "$default": "dev",
      "dev": {
        "services.api.base": "https://dev.example.com"
      },
      "prod": {
        "services.api.base": "https://api.example.com"
      }
    },
    "app": {
      "$default": "dev app 1",
      "dev app 1": {
        "app.id": "one"
      },
      "dev app 2": {
        "app.id": "two"
      }
    },
    "credentials": {
      "$default": "personal",
      "personal": {
        "auth.token": "local-token"
      },
      "ci": {
        "auth.token": "ci-token"
      }
    }
  }
}
```

`$shared`, `$groups`, and `$default` are matched case-insensitively and surrounding whitespace is ignored. Group and profile names cannot be empty or reuse a reserved name, and group names cannot contain `=`. Duplicate names are rejected, even when they only differ in case.

A group with more than one profile needs a `$default`. A group with a single profile uses that profile automatically. The default is matched case-insensitively and keeps the name as written on the profile. The example above defaults to `api=dev, app=dev app 1, credentials=personal`.

When the file loads, Resterm checks that no variable name appears in two different groups, since profiles from different groups can be active at the same time. The check is case-insensitive. Reusing a variable across profiles of the same group is fine because only one of them is ever active. Profile values override `$shared`, even when the key only differs in case. A collision error names the variable and both `group=profile` sources but never prints the values.

In the TUI, `Ctrl+E` opens a searchable list with one `group = profile` row per choice. Active profiles are marked. Picking a row switches only that group, and the header and status line always show the full selection.

From the CLI, select profiles with a repeatable flag:

```bash
resterm \
  --env-group api=prod \
  --env-group 'app=dev app 2' \
  --env-group credentials=ci
```

`--env` still selects named environments and cannot be combined with `--env-group`. Groups you do not pass keep their defaults.

`resterm run` and headless execution stop when an environment file is invalid. The TUI can recover from parse errors because the file can be fixed there. It starts without an active environment, shows the error in a modal, displays `ENV: not loaded` in the header, and blocks all requests. Saving a valid file reloads the environment and reapplies the current selection. Changes made outside Resterm are also reloaded while the environment file is open.

Missing or unreadable environment files still prevent the TUI from starting.

The public Go API accepts the same model:

```go
opt.Environment = headless.EnvironmentOptions{
    Grouped: &headless.GroupedEnvironmentSet{
        Shared: map[string]string{"region": "eu"},
        Groups: headless.EnvironmentGroups{
            "api": {
                Default: "dev",
                Profiles: headless.EnvironmentSet{
                    "dev":  {"services.api.base": "https://dev.example.com"},
                    "prod": {"services.api.base": "https://api.example.com"},
                },
            },
        },
    },
    Selection: headless.EnvironmentSelection{"api": "prod"},
}
```

You cannot combine `EnvironmentOptions.Set` with `Grouped`, or `Name` with `Selection`. Definitions supplied by your code take priority over `FilePath`. Groups left out of a selection use their defaults.

Resterm keeps separate runtime state for each full group selection. Changing just the credentials profile gives you separate cookies, runtime globals, file captures, command-auth entries, OAuth tokens, and saved runtime state. The selection identifies that state; the variable values and secrets are never part of the key.

A workspace uses one environment file. There is no group-local `$shared`, and the active selection is not remembered across restarts.

### Dotenv files via `--env-file`

Use JSON when you need several environments. For a single environment, you can load a dotenv file instead:

- Pass `--env-file path/to/.env`. Names like `.env.prod` and `prod.env` work too. Dotenv files are **never** auto-discovered. You have to opt in, so they never override values by surprise.
- Supported syntax matches common `.env` loaders: optional `export` prefixes, `KEY=value` pairs, `#`/`;` comments, single- and double-quoted values (with escapes), and `${VAR}` or `$VAR` interpolation. References expand using earlier keys from the same file and the current OS environment.
- The environment name is derived from a `workspace` entry (case-insensitive). If that key is missing or blank, Resterm uses the file name instead. `.env.prod` and `prod.env` both become `prod`, and a bare `.env` becomes `default`.
- Each dotenv file gives exactly one environment. If you need more than one, use `resterm.env.json`.
- There is no multi-workspace support and no auto-discovery. Interpolation only sees keys declared above the current line, plus OS environment variables.

## Variable resolution order

When expanding `{{variable}}` templates, Resterm looks in:

1. *File constants* (`@const`).
2. Values set by scripts for the current execution (`vars.set` in pre-request scripts).
3. Workflow step variables and the `@for-each` value bound for the current iteration.
4. Values declared with `@run var` for the current run. See [Run variables](workflows.md#run-variables).
5. *Request-scope* variables (`@var request`, `@capture request`).
6. *Runtime globals* stored via captures or scripts (per environment).
7. *Document globals* (`@global`, `@var global`).
8. *File scope* declarations and `@capture file` values.
9. Selected environment JSON.
10. OS environment variables (case-sensitive with an uppercase fallback).

Templates, RestermScript expressions, the RestermScript `vars` object, and the JavaScript `vars` API all use this order. `@const` and unmapped OS environment variables are available only to templates. They are not exposed through `vars` because scripts cannot override them. If a `@const` and another source use the same name, `vars` skips the constant and returns the value from the next source in the list.

Declarations other than `@run var`, and values in the selected environment, may use `env:NAME`. The value is exposed under the declared name. A missing reference stays undefined and continues to shadow lower sources, including the OS fallback in step 10. See [Values from OS environment variables](#values-from-os-environment-variables).

In templates, put a source name in front of a variable to read it from that source and skip the ones above it. `{{file.token}}` reads the `@file` value even when a request variable or a capture also defines `token`, or when a missing `env:NAME` reference hides `{{token}}`. The source names are `const`, `script`, `workflow`, `run`, `request`, `global` (runtime globals), `document-global` (`@global`), `file`, `environment`, and `env` (OS variables). A variable whose full name matches comes first, so one declared as `file.token` wins over this lookup.

Scripts receive declared values with ordinary variable references already expanded. For example, `vars.get("name")` returns the same value as `{{name}}`. Dynamic helpers in declarations run once when the request starts, so the script reads the same generated value that the request sends.

Expressions such as `{{= ... }}` stay as text until the request runs. Inside a `{{= ... }}` template expression, `vars` returns the same value as `{{name}}`, with helpers already evaluated. After sending, `@capture`, `@assert`, `@poll until=`, and `@retry-when` read the value the request used.

Captured values and values written by scripts are data and are not expanded again. Request getters, such as `request.getURL()` in JavaScript and `request.url` in RestermScript, also evaluate `{{= ... }}` expressions using the current variables.

Variable names are case-insensitive and ignore surrounding whitespace. A file variable named `token`, for example, takes precedence over an environment variable named `TOKEN`. When the same source defines a name more than once, the last declaration or script write wins. Global deletes also ignore case.

Blank names are invalid. RestermScript and `@apply` report an error, while JavaScript and runtime stores ignore the write.

## Dynamic helpers

Helper names are case-insensitive and need no declaration.

| Helper | Value |
| --- | --- |
| `{{$uuid}}` (alias `{{$guid}}`) | Random UUID v4 |
| `{{$timestamp}}` | Unix time in seconds |
| `{{$timestampMs}}` | Unix time in milliseconds |
| `{{$timestampISO8601}}` | Current time, RFC3339 UTC |
| `{{$randomInt}}` | Random integer. `{{$randomInt(100)}}` gives 0 to 100, `{{$randomInt(1, 6)}}` gives 1 to 6, both ends included |
| `{{$randomString}}` | Random alphanumeric string of 16 characters. `{{$randomString(24)}}` sets the length, up to 4096 |
| `{{$randomChoice("a", "b", "c")}}` | Random value from the given list |
| `{{$randomName}}` | Random full name |
| `{{$randomEmail}}` | Random email address |
| `{{$fake.person}}` | Random full name |
| `{{$fake.firstName}}`, `{{$fake.lastName}}` | Random name parts |
| `{{$fake.email}}`, `{{$fake.username}}` | Random account details |
| `{{$fake.company}}`, `{{$fake.domain}}` | Random company name and hostname |
| `{{$fake.city}}`, `{{$fake.country}}`, `{{$fake.phone}}` | Random address details |
| `{{$fake.word}}`, `{{$fake.sentence}}` | Random filler text |

Every reference is resolved on its own, so two `{{$uuid}}` references in one body give two values. To reuse a value within one request, declare it with `# @request trace.id {{$uuid}}`. To share it across a workflow, use `# @run var`. See [Run variables](workflows.md#run-variables).

Generated addresses and hostnames stay under the reserved `example.com`, `example.net`, and `example.org` domains, and phone numbers come from a range reserved for fiction, so no helper output points at a real host, mailbox, or line.

Arguments accept either quote form and may be left unquoted when they contain no comma: `{{$randomChoice(red, green)}}`. Inside a JSON body, prefer single quotes, since a backslash-escaped `\"` is part of the argument rather than a quote. A helper used the wrong way, such as `{{$randomChoice()}}`, fails the request with an error that names the helper instead of reporting a missing variable.

Timestamp helpers accept optional offsets: `{{$timestamp + 6d}}`, `{{$timestampISO8601 - 90m}}`, `{{$timestampMs + 2h}}`. Supported units are the standard Go duration units plus `d` (days) and `w` (weeks).

## Variable declarations

`@const`, `@var`, and `@global` provide static values evaluated before the request is sent. Constants resolve when the file is parsed and cannot be overridden by captures or scripts. Variables follow the usual resolution order and can change at runtime.

| Scope | Syntax | Visibility |
| --- | --- | --- |
| Constant | `# @const api.root https://api.example.com` | Immutable for the lifetime of the document. Available to every request in the file. |
| Global | `# @global api.token value` / `# @global-secret api.token value` / `# @var global api.token value` | Visible to every request and every file (per environment). |
| File | `# @file upload.root https://storage.example.com` / `# @file-secret upload.root ...` / `# @var file upload.root ...` | Visible to all requests in the same document only. |
| Request | `# @request trace.id {{$uuid}}` / `# @request-secret trace.id ...` / `# @var request trace.id ...` | Visible only to the current request (useful for tests). |
| Run | `# @run var order.ref = {{$uuid}}` | One value shared across a workflow or for-each run. See [Run variables](workflows.md#run-variables). |

Values are taken as written. Quotes are not special, so `# @file greeting "hello world"` stores the quotes as part of the value. If you need spaces, write them directly: `# @file greeting hello world`.

Declared values can refer to other variables and dynamic helpers. For example, `# @request trace.id {{$uuid}}` generates a new value each time the request runs. Every `{{trace.id}}` reference in that request, including reads from scripts, uses the same value.

Captures and values written with `vars.set` stay as written. Use [`vars.interpolate`](scripting.md#interpolating-text) to fill placeholders in a script string.

A self-reference or a cycle between variables fails the request with a `variable cycle` error that lists the names in the cycle. See [Variable resolution order](#variable-resolution-order) for how scripts read declared values.

You can also use shorthand assignments outside comment blocks: `@requestId = {{$uuid}}`. Shorthand defaults to request scope while you're inside a request block and to file scope elsewhere. Add a prefix to override it (`@global api.token abc`, `@request trace.id {{$uuid}}`, or `@file base.url https://example.com`).

Append `-secret` (`global-secret`, `file-secret`, `request-secret`) to mask stored values in summaries. This works for both comment directives and shorthand lines (`@global-secret token xyz`, `@file-secret base.url ...`, `@request-secret trace.id ...`).

## Secret redaction

Secret values are masked in errors, script failures, and failed test output before those messages reach the response pane, workflow summaries, or workflow history. A value remains masked for the rest of the run if it is deleted, replaced with a public value, or produced by a script or capture that later fails.

Diagnostic source excerpts show the request file as written. Avoid placing literal secrets in script source. Response bodies are also stored as received unless the request uses `@no-log` or the payload is redacted before it is returned.

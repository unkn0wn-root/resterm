# resterm run in depth

`resterm run [flags] <file|->` runs requests without the TUI. Flags may come before or after the file. Everything after `--` is a file name, even when it starts with `-`.

## Selecting what runs

| Flag | Short | Effect |
| --- | --- | --- |
| `--request <name>` | `-r` | Run the request with this `# @name`. Case does not matter. |
| `--workflow <name>` | `-W` | Run one `@workflow`. |
| `--tag <tag>` | `-g` | Run every request with this `# @tag`. |
| `--line <n>` | `-l` | Run the request or workflow that contains line `n`. |
| `--all` | `-a` | Run every request in the file. |
| `--profile` | `-p` | Run the selected request in profile mode. |

- `--request` cannot be combined with `--tag` or `--line`.
- `--all` cannot be combined with `--request`, `--tag` or `--line`.
- `--line` cannot be combined with another selector.
- `--workflow` cannot be combined with request selectors, `--compare` or `--profile`.
- Without a selector, a file with one request runs it. A file with several requests prints a numbered list and exits `2` unless both stdin and stdout are terminals.

## Output

| Flag | Short | Effect |
| --- | --- | --- |
| `--format <mode>` | `-f` | `auto` (default), `text`, `json`, `junit`, `pretty` or `raw`. |
| `--response` | | Add each response's headers, body and captures to `--format json`. |
| `--body` | `-b` | Print only the response body. Needs exactly one request result. |
| `--headers` | `-H` | Show request and response headers in the single-request view. |
| `--color <mode>` | `-c` | `auto`, `always` or `never`, for the pretty view. |

- `auto` shows a single-request view when exactly one request ran, and the text report otherwise. `pretty`, `raw` and `--body` need exactly one request result.
- `--body` works with `--format auto`, `pretty` or `raw`. It keeps the exit code, so a failed check still exits `1`.
- `--response` works only with `--format json`.
- Use `--format json` when you need to read the result. The text and single-request views are for people.

## Running

| Flag | Short | Effect |
| --- | --- | --- |
| `--env <name>` | `-e` | Choose a named environment. |
| `--env-group <group=profile>` | | Choose a profile in a grouped environment file. Repeat for more groups. |
| `--env-file <path>` | `-E` | Use this environment file, JSON or dotenv. |
| `--workspace <dir>` | `-w` | Workspace root for finding files. |
| `--recursive` | `-R` | Scan the workspace recursively. |
| `--timeout <duration>` | `-t` | Request timeout, which also limits the checks. The default is `30s`. |
| `--fail-fast` | `-ff` | Stop after the first failed result and mark the rest as skipped. |
| `--exit-code-mode <mode>` | `-m` | `detailed` (default) or `summary`. |
| `--insecure` | `-k` | Skip TLS certificate checks. |
| `--follow` | `-L` | Follow redirects. On by default. Pass `--follow=false` to turn it off. |
| `--max-redirects <n>` | | The most redirects to follow. The default is 10. |
| `--proxy <url>` | `-x` | Send requests through an HTTP proxy. |
| `--max-response-size <size>` | | The largest body to read, such as `100mb`. The default is 32 MiB. `none` removes the limit. |
| `--compare <envs>` | `-C` | Compare runs across these environments, separated by commas. |
| `--compare-base <env>` | `-B` | The baseline for a compare run. |
| `--compare-group <group>` | | In a grouped file, the group a compare run changes. |
| `--trace-otel-endpoint <url>` | `-toe` | Send `@trace` spans to this OTLP collector. |
| `--trace-otel-insecure` | `-toi` | Send spans without TLS. |
| `--trace-otel-service <name>` | `-tos` | Set the exported service name. |

## The JSON report

`resterm run --env dev --all --format json --response api.http` on the two requests in SKILL.md, when the server rejects the token:

```json
{
  "schemaVersion": "1",
  "version": "v1.18.0",
  "filePath": "api.http",
  "envName": "dev",
  "startedAt": "2026-11-06T09:30:00.1Z",
  "endedAt": "2026-11-06T09:30:00.304Z",
  "durationMs": 204,
  "summary": {
    "total": 2,
    "passed": 1,
    "failed": 1,
    "skipped": 0,
    "exitCode": 1,
    "failureCodes": ["assertion"]
  },
  "results": [
    {
      "kind": "request",
      "name": "login",
      "method": "POST",
      "target": "{{baseUrl}}/login",
      "effectiveTarget": "http://localhost:8080/login",
      "environment": "dev",
      "status": "pass",
      "durationMs": 182,
      "http": {"status": "200 OK", "statusCode": 200, "protocol": "HTTP/1.1"},
      "response": {
        "headers": {"Content-Type": ["application/json"], "Set-Cookie": ["•••"]},
        "body": "{\"token\":\"•••\",\"expiresIn\":3600}",
        "bodySize": 65,
        "captures": [{"scope": "file", "name": "token", "value": "•••", "secret": true}]
      },
      "tests": [{"name": "response.statusCode == 200", "passed": true}]
    },
    {
      "kind": "request",
      "name": "me",
      "method": "GET",
      "target": "{{baseUrl}}/me",
      "effectiveTarget": "http://localhost:8080/me",
      "environment": "dev",
      "status": "fail",
      "failure": {
        "code": "assertion",
        "category": "semantic",
        "exitCode": 1,
        "message": "response.statusCode == 200",
        "source": "tests"
      },
      "durationMs": 9,
      "http": {"status": "401 Unauthorized", "statusCode": 401, "protocol": "HTTP/1.1"},
      "response": {
        "headers": {"Content-Type": ["application/json"], "Www-Authenticate": ["Bearer"]},
        "body": "{\"error\":\"invalid token\"}",
        "bodySize": 25
      },
      "tests": [
        {"name": "response.statusCode == 200", "passed": false},
        {
          "name": "response.json().email == vars.get(\"user\")",
          "message": "logged in as someone else",
          "passed": false
        }
      ]
    }
  ]
}
```

- `schemaVersion` is `"1"`. Check it before you rely on other fields.
- The `401` arrived as a normal response, so the request failed only because its `@assert` failed. That gives `failure.code` `assertion` and exit code `1`. A request without checks passes with any status.
- `failure.code` is one of `assertion`, `trace_budget`, `timeout`, `network`, `tls`, `auth`, `script`, `filesystem`, `protocol`, `route`, `canceled`, `internal` or `unknown`. `failure.message` is the text to show the user.
- `failure.source` says where the failure came from, such as `tests`, `error` or `scriptError`.
- `effectiveTarget` is the URL after variables were filled in. It is left out when it equals `target`.
- A workflow, `@for-each` or compare run is one result with its parts in `steps[]`. A profile run is one result with its numbers in `profile`.
- For gRPC requests, `grpc.statusDetails` holds the status details the server sent, as JSON.
- `stream.transcriptPath` and `trace.artifactPath` appear when `--artifact-dir` wrote those files.

## What is masked

Text, JSON and JUnit reports, stream transcripts, trace files, history and errors show secrets as `•••`:

- values declared with a `-secret` form or loaded through `env:NAME`
- `-secret` captures, values a script marks as secret, and auth tokens, including the encoded Basic credential
- with `--response`, `Set-Cookie` and credential headers such as `Authorization` and `X-API-Key`

Resterm masks every secret the run knew by its end, so a token captured by a later request is masked in earlier responses too. A secret shorter than 16 characters is masked only where it stands as a whole word.

`--body`, `--format pretty`, `--format raw` and the `auto` single-request view show the response body as it arrived, without masking. Use `--format json --response` when a body may hold a secret.

OS values read through `{{env.NAME}}`, through a plain `{{NAME}}` that falls back to the OS, or through `${NAME}` in a dotenv file are not secrets, so they are not masked.

## Artifacts and saved state

| Flag | Short | Effect |
| --- | --- | --- |
| `--artifact-dir <dir>` | `-A` | Write stream transcripts to `<dir>/streams/` and trace summaries to `<dir>/traces/`. |
| `--state-dir <dir>` | `-s` | Where saved state lives. By default, one folder per workspace under the Resterm config directory. |
| `--persist-globals` | `-G` | Keep captured globals between runs in `runtime.json`. |
| `--persist-auth` | `-P` | Keep cached auth tokens between runs in `auth.json`. |
| `--history` | `-y` | Write the run to `history.db` with secrets masked. |

Without these flags, each `resterm run` starts fresh. Globals captured and auth tokens cached by an earlier run are gone. To reuse a token across runs, capture it with a `global` scope and pass `--persist-globals`, or let `@auth` cache it and pass `--persist-auth`.

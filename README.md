<h1 align="center">
  <img src="_media/resterm_logo.png" alt="Resterm" width="200" />
  <br>
  Resterm
</h1>

<p align="center">
  <em>An API-as-code workbench for the terminal.</em>
</p>

<p align="center">
  <img src="_media/resterm_base.png" alt="Screenshot of Resterm TUI base" width="720" />
</p>

<p align="center">
  <img src="_media/resterm_trace_timeline.png" alt="Screenshot of Resterm with timeline" width="720" />
  <br>
  <sub>Trace and timeline view</sub>
</p>

Resterm is an API client that stores requests in plain `.http` and `.rest` files that can live side by side in your repo like the rest of your code. You can use the terminal UI, or run the same files in CI with `resterm run`.

Quick links: [Screenshots](#screenshots), [Install](#install), [Quick start](#quick-start), [Request files](#request-files), [Documentation](#documentation).

## Screenshots

<details>
<summary>See the UI in action (click to expand)</summary>

<p align="center">
  <strong>Workflows</strong>
</p>

<p align="center">
  <img src="_media/resterm_workflow.png" alt="Screenshot of Resterm with Workflow" width="720" />
</p>

<p align="center">
  <strong>Profiler</strong>
</p>

<p align="center">
  <img src="_media/resterm_profiler.png" alt="Screenshot of Resterm profiler" width="720" />
</p>

<p align="center">
  <strong>Explain</strong>
</p>

<p align="center">
  <img src="_media/resterm_explain.png" alt="Screenshot of Resterm Explain Tab" width="720" />
</p>

<p align="center">
  <strong>RestermScript</strong>
</p>

<p align="center">
  <img src="_media/resterm_script.png" alt="Screenshot of Resterm with RestermScript" width="720" />
</p>

</details>

## Why Resterm

- **HTTP, GraphQL, gRPC, WebSocket and SSE** support.
- **Automation in request files:** conditions (`@when`, `@if`/`@elif`/`@else`, `@for-each`), multi-step workflows (`@workflow` / `@step`), captures, variables and assertions (`@capture`, `@var`, `@assert`).
- **Tunnels in the request file:** `@ssh` and `@k8s` route a request through an SSH bastion or a Kubernetes port-forward that Resterm opens and closes for you, with profiles per file or workspace.
- **Record HTTP traffic** and export it to Resterm `.http` files as requests or mock responses.
- **RestermScript**, a small expression language built for Resterm, with JavaScript hooks when you want them.
- **Vim-like controls** with shortcut hints, searchable offline help, `Shift+k` help under the cursor, `/` search and commands like `:w`, `:q`, `:help` and `:docs`.
- **Auth:** OAuth 2.0 (client credentials, password, authorization code with PKCE) and `@auth command` to reuse tokens from CLIs you already have installed, like `gh auth token`.
- **CLI runner:** `resterm run` for scripted runs and CI, with JSON and JUnit output.
- **Mock servers** declared next to the requests they mimic, with matching rules, sequences, call verification and hot reload.
- **Timeline tracing, profiling and compare runs** across environments.
- **Streaming transcripts** and an interactive console for WebSocket and SSE.
- **No AI integration**

## Install

macOS and Linux:

```bash
brew install resterm
# or
curl -fsSL https://raw.githubusercontent.com/unkn0wn-root/resterm/main/install.sh | bash
```

Windows:

```powershell
iwr -useb https://raw.githubusercontent.com/unkn0wn-root/resterm/main/install.ps1 | iex
```

From source, with Go 1.25 or newer:

```bash
go install github.com/unkn0wn-root/resterm/cmd/resterm@latest
```

> [!IMPORTANT]
> Prebuilt Linux binaries depend on glibc 2.32 or newer. On an older distro, build from source with a newer glibc toolchain or upgrade glibc before using the release archives.

Homebrew installs are updated with `brew upgrade resterm`. Binaries from the releases page or the install scripts use `resterm --check-update` and `resterm --update`, which downloads, verifies and installs in place. On Windows the old binary stays next to the new one as `resterm.exe.old` and is cleaned up on the next update.

### Manual install

Binaries for macOS, Linux and Windows (amd64 and arm64) are on the [releases page](https://github.com/unkn0wn-root/resterm/releases). The commands below does the same as downloading manually from release page. The Unix version needs `curl` and `jq`.

```bash
# Find the latest release tag
LATEST_TAG=$(curl -fsSL https://api.github.com/repos/unkn0wn-root/resterm/releases/latest | jq -r .tag_name)

# Download the matching binary (Darwin/Linux + amd64/arm64)
curl -fL -o resterm "https://github.com/unkn0wn-root/resterm/releases/download/${LATEST_TAG}/resterm_$(uname -s)_$(uname -m)"

# Install on PATH
chmod +x resterm
sudo install -m 0755 resterm /usr/local/bin/resterm
```

```powershell
$latest = Invoke-RestMethod https://api.github.com/repos/unkn0wn-root/resterm/releases/latest
$asset  = $latest.assets | Where-Object { $_.name -like 'resterm_Windows_*' } | Select-Object -First 1
Invoke-WebRequest -Uri $asset.browser_download_url -OutFile resterm.exe
# Optionally move to a directory on PATH:
Move-Item resterm.exe "$env:USERPROFILE\bin\resterm.exe"
```

## Quick start

```bash
mkdir my-api && cd my-api
resterm init
resterm
```

`resterm init` creates a small project that you can start using right away. The generated `requests.http` has local mock scenarios and a few requests that build on each other, covering assertions, bearer auth, JSON matching, `json-rules` and `@for-each`. Press `g Shift+m` to start the mock server, then `Ctrl+Enter` to send the request under the cursor.

You can also open Resterm directly without `init`. Run `resterm`, type a URL and press `Ctrl+Enter`. You can also paste curl command - that's works too.

The same file runs without the TUI:

```bash
resterm run --request CreateUser requests.http
```

## Request files

Resterm supports standard HTTP syntax, but it goes far beyond that with `# @` directives for configuration and automation:

```http
# @setting base-url https://api.example.com/v1/

### Create users
// Send this request once for each name in the list.
# @for-each ["david", "tom"] as name
# @when env.mode == "development"
# @assert response.statusCode == 201
POST users
Content-Type: application/json

{"name":"{{= name }}"}
```

Placing `@setting` before the first request apply to the whole file. `###` starts a new request, and directives can repeat, limit or check the request below them. More in [`_examples/`](_examples/) and the [directive reference](docs/resterm.md#request-file-anatomy).

## Mock servers

Mock responses are defined in the same files as the requests (but they don't have to). Example, two scenarios on one route:

```http
### Payment accepted
# @mock method=POST path=/payments name=accepted default=true latency=150ms
HTTP/1.1 202 Accepted
Content-Type: application/json

{"id":"pay_123","status":"pending"}

### Payment declined
# @mock method=POST path=/payments name=declined
# @match query={"mode":"decline"} headers={"X-Tenant":"demo"} json={"amount":0}
HTTP/1.1 422 Unprocessable Entity
Content-Type: application/json

{"error":"amount must be positive"}
```

Serve one file or a whole directory:

```bash
resterm mock ./requests.http
resterm mock --recursive --addr 127.0.0.1:9090 ./requests
```

Matching on query, headers and body, response sequences for polling tests, call verification and hot reload are covered in the [mock server reference](docs/resterm.md#mock-servers). Working example: [`_examples/mocks.http`](_examples/mocks.http).

## Recording traffic

You can route your application through the Resterm proxy and it captures the traffic into a Resterm `.http` file, as requests, mocks or both.

```bash
resterm record --upstream https://api.example.com --out captured.http --mode both
```

Point your application's API base URL at `http://127.0.0.1:9000`, then stop recording with `Ctrl+C`. The TUI does the same thing with `:record start --upstream <origin>`, plus `:record as-request` and `:record as-mock` to insert captures into the open file.

More in the [recording reference](docs/resterm.md#recording-traffic).

## More

| Area | Docs |
| --- | --- |
| **Automation** | [workflows](docs/resterm.md#workflows), [polling and retries](docs/resterm.md#polling-and-retries), [compare runs](docs/resterm.md#compare-runs), [timeline and tracing](docs/resterm.md#timeline--tracing), [profiling](docs/resterm.md#profiling-requests) |
| **Transports** | [gRPC](docs/resterm.md#grpc), [GraphQL](docs/resterm.md#graphql), [WebSocket and SSE](docs/resterm.md#streaming-sse--websocket) |
| **Auth and connectivity** | [OAuth 2.0](docs/resterm.md#oauth-20-directive), [auth from your own CLI](docs/resterm.md#command-backed-auth), [SSH tunnels](docs/resterm.md#ssh-tunnels), [Kubernetes port-forwards](docs/resterm.md#kubernetes-port-forwards) |
| **Scripting** | [RestermScript](docs/restermscript.md), [JavaScript hooks](docs/resterm.md#scripting-api), [headless Go API](./headless), [resterm-runner](https://github.com/unkn0wn-root/resterm-runner) |
| **In and out** | [curl import](docs/resterm.md#inline-requests), [OpenAPI import](docs/cli.md#import-examples), [collection sharing](docs/resterm.md#collection-sharing), [response history and diffing](docs/resterm.md#response-history--diffing) |
| **Setup** | [environments and variables](docs/resterm.md#variables-and-environments), [configuration](docs/resterm.md#configuration), [themes](docs/resterm.md#theming), [key bindings](docs/resterm.md#custom-bindings) |

## Keys

Press `?` for general Resterm help and `Shift+k` for help on whatever is under the cursor. The full table is in the [UI tour](docs/resterm.md#ui-tour). For quick start, you only need:

- `Ctrl+Enter` sends the request
- `Tab` / `Shift+Tab` switches panes
- `g p` jumps to the response

## Documentation

- [`docs/resterm.md`](./docs/resterm.md) covers request syntax, directives, scripting and transports.
- [`docs/cli.md`](./docs/cli.md) covers `resterm run`, importers, collections and history.
- [`docs/restermscript.md`](./docs/restermscript.md) is the RestermScript reference.
- [Compatibility](./docs/resterm.md#compatibility) lists what stays stable through v1.

Inside the TUI, `:help <topic>` opens the embedded manual and `:docs <topic>` opens the web copy for the installed release.

## License

[Apache License 2.0](./LICENSE).

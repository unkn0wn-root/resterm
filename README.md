<h1 align="center">
  <img src="_media/resterm_logo.png" alt="Resterm" width="200" />
  <br>
  Resterm
</h1>

<p align="center">
  <em>A terminal API client for REST, GraphQL, gRPC, WebSocket and SSE.</em>
</p>

<p align="center">
  <img src="_media/resterm_base.png" alt="Screenshot of Resterm TUI base" width="720" />
</p>

Resterm is a keyboard-driven API client that runs in your terminal. Requests are plain `.http` files that you can diff, review and version like code. Everything stays on your machine. No accounts, no cloud sync, no telemetry.

Quick links: [Screenshots](#screenshot-tour), [Installation](#installation), [Quick Start](#quick-start), [Documentation](#documentation).

## Screenshot tour

<details>
<summary>See the UI in action (click to expand)</summary>

<p align="center">
  <strong>Workflows</strong>
</p>

<p align="center">
  <img src="_media/resterm_workflow.png" alt="Screenshot of Resterm with Workflow" width="720" />
</p>

<p align="center">
  <strong>Trace and Timeline</strong>
</p>

<p align="center">
  <img src="_media/resterm_trace_timeline.png" alt="Screenshot of Resterm with timeline" width="720" />
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

<p align="center">
  <strong>Light Theme</strong>
</p>

<p align="center">
  <img src="_media/resterm-lighttheme.png" alt="Screenshot of Resterm in Lighttheme" width="720" />
</p>

<p align="center">
  <strong>OAuth browser demo (old UI design)</strong>
</p>

<p align="center">
  <img src="_media/oauth.gif" alt="Resterm OAuth flow" width="720" />
</p>

</details>

## Why Resterm

- **HTTP, GraphQL, gRPC, WebSocket and SSE** out of the box.
- **Automation lives in the request files:** conditions (`@when`, `@if`/`@elif`/`@else`, `@for-each`), multi-step workflows (`@workflow` / `@step`), captures, variables and assertions (`@capture`, `@var`, `@assert`).
- **RestermScript**, a small expression language built for Resterm, with JavaScript hooks when you want them.
- **Vim-style controls** with familiar motions, `/` search and commands like `:w`, `:q`, `:e` and `:help`.
- **Built-in auth and tunneling:** OAuth 2.0 (client credentials, password, auth code with PKCE), auth backed by your existing CLIs, SSH tunnels and Kubernetes port-forwards. No extra tools needed.
- **CLI runner:** `resterm run` for scripted runs and CI, with JSON and JUnit output.
- **Mock servers** declared next to the requests they mimic, with matching rules, sequences, call verification and hot reload.
- **Timeline tracing, profiling and compare runs** across environments.
- **Streaming transcripts** and an interactive console for WebSocket and SSE.
- **No AI integration**, ever.

## CLI

Use `resterm run` to execute `.http` / `.rest` files without opening the TUI.

```bash
mkdir my-api && cd my-api
resterm init
resterm run --request Echo requests.http
```

`resterm init` creates `requests.http` with sample requests and `resterm.env.json` with a `dev` environment pointing at httpbin.org. The last command runs the Echo request, which POSTs a JSON body and prints the response.

The [CLI documentation](docs/cli.md) covers selectors, output formats and more examples.

## Mock Servers

The same files that hold your requests can serve HTTP mocks.

- Match incoming requests by query, headers or JSON body, then pick a named or default response.
- Model polling and retry flows with response sequences, including independent cursors per resource or caller.
- Build responses from path, query, header and body values, with generators for dynamic data.
- Verify call counts with `@expect` or inspect received traffic from RestermScript.
- Hot reload source files and fixtures, with optional TLS.

Two scenarios on one route:

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

More in the [Mock Servers reference](docs/resterm.md#mock-servers), the [`resterm mock` CLI guide](docs/cli.md#resterm-mock) and the [working example](_examples/mocks.http).

## Headless

The [`headless`](./headless) package is the public Go API for the same engine that powers the TUI and CLI. Use it to run requests, workflows, assertions, compare runs and profiles from your own Go code or CI.

If you would rather not build a runner yourself, there is [resterm-runner](https://github.com/unkn0wn-root/resterm-runner).

## Quick Start

1. Install Resterm (see [Installation](#installation) for scripts, Windows and manual installs).

   ```bash
   brew install resterm
   ```

2. Bootstrap a workspace.

   ```bash
   mkdir my-api && cd my-api
   resterm init
   ```

3. Start it and send your first request.

   ```bash
   resterm
   ```

   Press `Ctrl+Enter` in the editor to send the highlighted request.

No files yet? Just run `resterm`, type a URL and press `Ctrl+Enter`. A pasted curl command works too.

## Keyboard cheat sheet

- Pane focus and layout
  - `Tab` / `Shift+Tab`: move between sidebar, editor and response.
  - `g+r`, `g+i`, `g+p`: jump to requests, editor or response.
  - `g+h` / `g+l`: resize horizontally. Changes sidebar width when the sidebar is focused, the editor/response split otherwise.
  - `g+j` / `g+k`: resize editor/response height when stacked, collapse or expand branches in the navigator.
  - `g+v` / `g+s`: toggle the response pane between inline and stacked layout.
  - `g+1`, `g+2`, `g+3`: minimize or restore sidebar, editor, response.
  - `g+z` / `g+Z`: zoom the focused pane, clear zoom.
- Environments and globals
  - `Ctrl+E`: switch environments.
  - `Ctrl+G`: inspect captured globals.
- Responses
  - `Ctrl+V` / `Ctrl+U`: split the response pane for side-by-side comparison.
  - `Ctrl+Shift+C` or `g y` (response focused): copy the whole Pretty, Raw or Headers tab.
  - `g x`: show the Explain preview for the active request without sending it.
  - `g e`: open the current file in your external editor.

> [!TIP]
> If you only remember three shortcuts:
> - `Ctrl+Enter` sends the request
> - `Tab` / `Shift+Tab` switches panes
> - `g+p` jumps to the response

## Installation

**Linux / macOS (Homebrew)**

```bash
brew install resterm
```

> [!NOTE]
> Homebrew installs should be updated with Homebrew (`brew upgrade resterm`). The built-in `resterm --update` command is for binaries installed from GitHub releases or install scripts.

**Linux / macOS (Shell script)**

> [!IMPORTANT]
> Pre-built Linux binaries depend on glibc 2.32 or newer. On an older distro, build from source with a newer glibc toolchain or upgrade glibc before using the release archives.

```bash
curl -fsSL https://raw.githubusercontent.com/unkn0wn-root/resterm/main/install.sh | bash
```

or with `wget`:

```bash
wget -qO- https://raw.githubusercontent.com/unkn0wn-root/resterm/main/install.sh | bash
```

**Windows (PowerShell)**

```powershell
iwr -useb https://raw.githubusercontent.com/unkn0wn-root/resterm/main/install.ps1 | iex
```

The scripts detect your architecture, download the latest release and install the binary.

### Manual installation

> [!NOTE]
> The manual install helper uses `curl` and `jq`. Install `jq` with your package manager (`brew install jq`, `sudo apt install jq`, etc.).

**Linux / macOS**

```bash
# Detect latest tag
LATEST_TAG=$(curl -fsSL https://api.github.com/repos/unkn0wn-root/resterm/releases/latest | jq -r .tag_name)

# Download the matching binary (Darwin/Linux + amd64/arm64)
curl -fL -o resterm "https://github.com/unkn0wn-root/resterm/releases/download/${LATEST_TAG}/resterm_$(uname -s)_$(uname -m)"

# Make it executable and move it onto your PATH
chmod +x resterm
sudo install -m 0755 resterm /usr/local/bin/resterm
```

**Windows (PowerShell)**

```powershell
$latest = Invoke-RestMethod https://api.github.com/repos/unkn0wn-root/resterm/releases/latest
$asset  = $latest.assets | Where-Object { $_.name -like 'resterm_Windows_*' } | Select-Object -First 1
Invoke-WebRequest -Uri $asset.browser_download_url -OutFile resterm.exe
# Optionally relocate to a directory on PATH, e.g.:
Move-Item resterm.exe "$env:USERPROFILE\bin\resterm.exe"
```

### From source

```bash
go install github.com/unkn0wn-root/resterm/cmd/resterm@latest
```

## Update

```bash
resterm --check-update
resterm --update
```

The first command reports whether a newer release is available. The second downloads, verifies and installs it in place. On Windows the old binary stays next to the new one as `resterm.exe.old` and is cleaned up on the next update.

## Configuration

- Environments are JSON files (`resterm.env.json`) discovered in the request directory, workspace root or CWD. Dotenv files (`.env`, `.env.*`) are opt-in via `--env-file` and are single-workspace. Prefer JSON when one file should hold several environments.
- Config is stored per OS and can be overridden with `RESTERM_CONFIG_DIR`:
  - macOS: `~/Library/Application Support/resterm`
  - Windows: `%APPDATA%\resterm`
  - Linux/Unix: `~/.config/resterm`

## Collections

Export a workspace as a Git-friendly bundle and import it into another one. Bundles carry a `manifest.json` with checksums, so imports verify file integrity first. Environment values are exported as `REPLACE_ME` placeholders, so secrets never leave your machine.

```bash
resterm collection export --workspace ./my-api --out ./shared/my-api-bundle
resterm collection import --in ./shared/my-api-bundle --workspace ./my-local-api
```

Add `--dry-run` to preview an import and `--force` to overwrite existing files. Docs: [collection sharing](./docs/resterm.md#collection-sharing).

## Curl import

Paste a curl command into the editor and press `Ctrl+Enter` to turn it into a structured request. Resterm understands the common flags, merges repeated data segments and keeps multipart uploads intact. Shell prefixes like `sudo` or `$` are ignored. The CLI does the same conversion with `--from-curl`.

Docs: [inline requests](./docs/resterm.md#inline-requests) and [import examples](./docs/cli.md#import-examples).

## RestermScript

RestermScript (RTS) is a small expression language built for Resterm. It targets the request format, workflows and directives directly, which keeps scripts short and predictable. JavaScript hooks remain available when you need more.

Quick example (RTS module + request):

```rts
// rts/helpers.rts
module helpers
export fn authHeader(token) {
  return token ? "Bearer " + token : ""
}
```

```http
# @use ./rts/helpers.rts
# @when env.has("feature")
# @assert response.statusCode == 200
GET https://api.example.com/users/{{= vars.get("user") }}
Authorization: {{= helpers.authHeader(vars.get("auth.token")) }}
```

Full reference: [`docs/restermscript.md`](docs/restermscript.md).

## Deep dive

#### OAuth 2.0

Client credentials, password grant and authorization code with PKCE. For auth code flows Resterm opens your browser, runs a local callback server on `127.0.0.1`, captures the redirect and exchanges the code. Tokens are cached per environment and refreshed when they expire. Docs: [`docs/resterm.md#oauth-20-directive`](./docs/resterm.md#oauth-20-directive) and `_examples/oauth2.http`.

#### Workflows and scripting

Chain requests with `@workflow` and `@step`, pass data between steps and add JS hooks where needed. Docs and sample: [`docs/resterm.md#workflows`](./docs/resterm.md#workflows) and `_examples/workflows.http`.

#### Compare runs

Run the same request across environments with `@compare` or `--compare`, then diff the responses side by side with `g+c`. Docs: [`docs/resterm.md#compare-runs`](./docs/resterm.md#compare-runs).

#### Tracing and timeline

Add `@trace` with budgets to capture DNS, connect, TLS, TTFB and transfer timings. Resterm highlights overruns and can export spans to OpenTelemetry. Docs: [`docs/resterm.md#timeline--tracing`](./docs/resterm.md#timeline--tracing).

#### Streaming (WebSocket and SSE)

Use `@websocket` with `@ws` steps or `@sse` to script and record streams. The Stream tab keeps transcripts and includes an interactive console. Docs: [`docs/resterm.md#streaming-sse--websocket`](./docs/resterm.md#streaming-sse--websocket).

#### gRPC

Unary and streaming calls with transcripts, metadata and body expansion. Docs: [`docs/resterm.md#grpc`](./docs/resterm.md#grpc).

#### OpenAPI import

Convert OpenAPI 3 specs into `.http` collections with `--from-openapi`, from a local file or an `http(s)` URL. Choose the generated blocks with `--openapi-mode requests`, `mocks` or `both`. Remote fetches respect the global `--insecure` and `--proxy` flags. Docs: [`docs/cli.md#import-examples`](./docs/cli.md#import-examples).

#### SSH tunnels

Route HTTP, gRPC, WebSocket and SSE traffic through bastions with `@ssh` profiles. Docs: [`docs/resterm.md#ssh-tunnels`](./docs/resterm.md#ssh-tunnels) and `_examples/ssh.http`.

#### Kubernetes port-forwards

Same idea with `@k8s` profiles, targeting pods, services, deployments or statefulsets. Docs: [`docs/resterm.md#kubernetes-port-forwards`](./docs/resterm.md#kubernetes-port-forwards) and `_examples/k8s.http`.

#### Theming and bindings

Customize colors and keybindings with `themes/*.toml` and `bindings.toml` or `bindings.json` in the config directory. Docs: [`docs/resterm.md#theming`](./docs/resterm.md#theming) and [`docs/resterm.md#custom-bindings`](./docs/resterm.md#custom-bindings).

## Documentation

- [`docs/resterm.md`](./docs/resterm.md) covers request syntax, directives, scripting and transports.
- [`docs/cli.md`](./docs/cli.md) covers `resterm run`, importers, collections and history.

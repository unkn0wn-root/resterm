<h1 align="center">
  <img src="_media/resterm_logo.png" alt="Resterm" width="120" />
  <br>
  Resterm
</h1>

<p align="center">
  <em>An API-as-code workbench for the terminal.</em>
  <br>
  <a href="https://resterm.app">resterm.app</a> · <a href="https://resterm.app/docs/">docs</a>
</p>

<p align="center">
  <img src="_media/resterm_base.png" alt="Screenshot of Resterm TUI base" width="720" />
</p>

Resterm is an API client for the terminal. Requests are saved in plain `.http` and `.rest` files, so they can live in your repo next to your code. Use them from the terminal UI, or run the same files in CI or CLI with `resterm run`.

## Why Resterm

- HTTP, GraphQL, gRPC, WebSocket and SSE.
- Assertions, captures, loops, conditions and multi-step workflows, all written in the request file.
- Scripting with RestermScript, a small expression language made for Resterm, or JavaScript if you prefer.
- Mock servers and traffic recording that use the same file format.
- OAuth 2.0, tokens from CLIs you already have (like `gh auth token`).
- SSH tunnels and Kubernetes port-forwards.
- Tracing, profiling and side-by-side comparison of responses across environments.
- Vim-style keys with built-in help.

## Screenshots

<details>
<summary>Click to see more of the UI</summary>

<p align="center">
  <strong>Trace and timeline</strong>
</p>

<p align="center">
  <img src="_media/resterm_trace_timeline.png" alt="Screenshot of Resterm with timeline" width="720" />
</p>

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

With Go 1.25 or newer:

```bash
go install github.com/unkn0wn-root/resterm/cmd/resterm@latest
```

Prebuilt binaries for macOS, Linux and Windows are on the [releases page](https://github.com/unkn0wn-root/resterm/releases). The Linux binaries need **glibc 2.32** or newer.

To update, run `brew upgrade resterm` if you installed with Homebrew. Otherwise run `resterm --update`. More in the [install guide](https://resterm.app/docs/install/).

## Quick start

```bash
mkdir my-api && cd my-api
resterm init
resterm
```

`resterm init` creates a `requests.http` file with a few sample requests and local mocks to answer them, so you don't need a real API to try it. Press `g Shift+M` to start the mock server, move the cursor to a request and press `Ctrl+Enter` to send it. Press `?` for help at any time.

You don't need `init` either. Run `resterm`, type a URL or paste a curl command, and press `Ctrl+Enter`.

The same file also runs without the UI. Start the mocks in one terminal and run a request in another:

```bash
resterm mock requests.http                       # terminal 1
resterm run --request CreateUser requests.http   # terminal 2
```

## Request files

Resterm reads the standard `.http` format and adds `# @` directives on top for settings and automation:

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

A `@setting` before the first request applies to the whole file. `###` starts a new request. Directives above a request can repeat it, skip it or check the response.

There are more examples in [`_examples/`](_examples/), and every directive is listed in the [request file reference](https://resterm.app/docs/request-files/).

## Documentation

The full docs are at [resterm.app/docs](https://resterm.app/docs/). Good places to start are [workflows](https://resterm.app/docs/workflows/), [mock servers](https://resterm.app/docs/mock-servers/), [recording](https://resterm.app/docs/recording/), [the CLI](https://resterm.app/docs/cli/) and [RestermScript](https://resterm.app/docs/rts/).

Inside the TUI, `:help <topic>` opens the built-in manual and `:docs <topic>` opens the same page on the website.

To run request files from your own Go code, see the [`headless`](./headless) package.

## Sponsor

Resterm is and always will be free and open source. If it saves you time, you can [sponsor its development](https://github.com/sponsors/unkn0wn-root).

## License

[Apache License 2.0](./LICENSE).

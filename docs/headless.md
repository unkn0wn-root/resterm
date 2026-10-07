# Headless Go API

The `headless` package runs request files from Go code. It uses the same engine as [`resterm run`](cli/run.md) and returns the same report, so you can call Resterm from your own tests, tools or CI jobs without starting a separate process.

```bash
go get github.com/unkn0wn-root/resterm
```

`headless` is Resterm's only public Go package. The [compatibility promise](compatibility.md) covers it, so code written against it keeps working through v1. Every type and field is listed on [pkg.go.dev](https://pkg.go.dev/github.com/unkn0wn-root/resterm/headless).

Request files can run commands and scripts, and they can write files. Only run files you trust. [Security](security.md) lists what a file can do.

## Run a file

```go
package main

import (
	"context"
	"log"
	"os"

	"github.com/unkn0wn-root/resterm/headless"
)

func main() {
	rep, err := headless.Run(context.Background(), headless.Options{
		Source:      headless.Source{Path: "api.http"},
		Environment: headless.EnvironmentOptions{Name: "dev"},
		Selection:   headless.Selection{Tag: "smoke"},
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := rep.Encode(os.Stdout, headless.Text); err != nil {
		log.Fatal(err)
	}
	os.Exit(rep.ExitCode(headless.ExitCodeDetailed))
}
```

This is the same as `resterm run --env dev --tag smoke api.http`.

`Run` returns an error when the run cannot start or finish, for example when the options are invalid or the state directory cannot be written. A request that fails, times out or breaks an assertion is not an error. It shows up as a failed result in the report.

The file does not have to exist on disk. Put the request in `Source.Content` and Resterm runs those bytes instead of reading the file. `Source.Path` is still required, because relative paths and the environment file are resolved from its directory.

```go
src := []byte("GET https://httpbin.org/status/200\n")
rep, err := headless.Run(ctx, headless.Options{
	Source: headless.Source{Path: "health.http", Content: src},
})
```

## Options

`Source.Path` is the only required field. The rest match the `resterm run` flags:

| Field | Flag | Description |
| --- | --- | --- |
| `Source.Path` | file argument | The request file. |
| `Source.Content` | `-` | Run these bytes instead of reading `Source.Path`. |
| `WorkspaceRoot` | `--workspace` | Where Resterm looks for other request files, such as files with `@auth global` definitions. Defaults to the directory of `Source.Path`. |
| `Recursive` | `--recursive` | Look in subdirectories of the workspace too. |
| `Selection` | `--request`, `--workflow`, `--tag`, `--all` | What to run. |
| `Environment` | `--env`, `--env-group`, `--env-file` | Which environment to use. See [Environments](#environments). |
| `Compare` | `--compare`, `--compare-base`, `--compare-group` | Run against several environments. See [Compare runs](compare-runs.md). |
| `Profile.Enabled` | `--profile` | Run the selected request in profile mode. See [Profiling](profiling.md). |
| `FailFast` | `--fail-fast` | Stop after the first failed result and mark the rest as skipped. |
| `HTTP` | see below | Default HTTP client settings. |
| `GRPC.Plaintext` | none | Use plaintext for gRPC requests that do not set `@grpc-plaintext` or any TLS setting. Defaults to `true`. |
| `State` | `--artifact-dir`, `--state-dir`, `--persist-globals`, `--persist-auth`, `--history` | Artifacts and state kept between runs. See [Artifacts and persisted state](cli/run.md#artifacts-and-persisted-state). |
| `Version` | none | Copied into the report as `Version`, so you can tell which build of your tool wrote it. |

`HTTP` holds the defaults for requests that do not set their own:

- `Timeout` - like `--timeout`. Zero means 30 seconds.
- `FollowRedirects` - like `--follow`. Leave it `nil` to follow redirects.
- `MaxRedirects` - like `--max-redirects`. Leave it `nil` for the default of 10.
- `MaxResponseBytes` - like `--max-response-size`. Leave it `nil` for the default of 32 MiB. Set it to `0` to remove the limit.
- `InsecureSkipVerify` - like `--insecure`.
- `ProxyURL` - like `--proxy`.

Selection follows the same rules as the CLI. Without a selector, a file with one request runs that request. A file with more than one request returns a usage error, since there is no picker to ask. `Workflow` cannot be combined with any other selector, or with `Compare` and `Profile`. `All` cannot be combined with `Request` or `Tag`, and `Request` cannot be combined with `Tag`.

## Environments

With no `Environment` options, Resterm looks for an environment file in the directory of `Source.Path` and then in the workspace root, the same as `resterm run`. It picks the same default environment too.

- `Name` selects a named environment, like `--env`.
- `Selection` picks profiles in a grouped environment file, like `--env-group`. Groups you leave out keep their defaults.
- `FilePath` loads a specific environment file, like `--env-file`.
- `Set` and `Grouped` pass environments in from your code, so no file is read. They take precedence over `FilePath`.

`Set` and `Grouped` cannot be combined, and neither can `Name` and `Selection`.

```go
opts.Environment = headless.EnvironmentOptions{
	Set: headless.EnvironmentSet{
		"ci": {"baseUrl": "http://localhost:8080", "token": os.Getenv("API_TOKEN")},
	},
	Name: "ci",
}
```

[Grouped environments](variables.md#grouped-environments) shows how to fill in `Grouped`.

## Reuse a plan

`Run` reads and checks everything on every call. When you run the same file many times, for example in a retry loop or from several goroutines, call `Build` once and then `RunPlan` as often as you need:

```go
plan, err := headless.Build(opts)
if err != nil {
	return err
}
for range 3 {
	rep, err := headless.RunPlan(ctx, plan)
	if err != nil {
		return err
	}
	if !rep.HasFailures() {
		break
	}
}
```

`Build` reads the request file once. Changes made to the file after that are not picked up, so build a new plan when the file changes.

A plan can be shared between goroutines. When it persists state through `PersistGlobals`, `PersistAuth` or `History`, its runs go one at a time so they do not overwrite each other's state files.

To stop a run, cancel its context. When several requests are selected, the one in progress stops, the ones after it are marked as skipped, and `rep.StopReason` is `headless.StopReasonCanceled`.

## Read the report

`Report` has the totals (`Total`, `Passed`, `Failed`, `Skipped`) and one `Result` for each request, workflow, compare run or profile run. Workflows and compare runs list their steps in `Steps`. A failed result or step carries a `Failure` with a `Code` such as `assertion`, `timeout` or `network`.

```go
for _, res := range rep.Results {
	if res.Failed() && res.Failure != nil {
		fmt.Printf("%s: %s: %s\n", res.Name, res.Failure.Code, res.Failure.Message)
	}
}
```

`rep.FailureCodes()` returns each failure code in the report once. `rep.Warnings` lists parse warnings from the request file. Warnings never fail a run.

## Output and exit codes

`Encode` writes the report in one of three formats. They match `resterm run --format`:

- `headless.Text` for people
- `headless.JSON` for scripts and other tools
- `headless.JUnit` for CI systems

`ParseFormat` turns a name like `"junit"` into a `Format`, which helps when the format comes from a flag or a config file. `json.Marshal(rep)` gives the same JSON as `Encode`, without indentation.

`rep.ExitCode(headless.ExitCodeDetailed)` returns the code `resterm run` would exit with. The [exit code table](cli/run.md#exit-codes) lists them, and each one has a constant such as `headless.ExitTimeout`. Pass `headless.ExitCodeSummary` to get only `0` or `1`.

Errors returned by `Run` never reach the report, so decide on their exit code yourself:

```go
rep, err := headless.Run(ctx, opts)
switch {
case headless.IsUsageError(err):
	fmt.Fprintln(os.Stderr, err)
	os.Exit(headless.ExitUsage)
case err != nil:
	fmt.Fprintln(os.Stderr, err)
	os.Exit(headless.ExitInternal)
}
os.Exit(rep.ExitCode(headless.ExitCodeDetailed))
```

Invalid options and selections return a `UsageError`. So does a request file that cannot be read or has parse errors, so the example exits `2` for it, as `resterm run` does. Use `errors.Is` to check for a specific one, such as `headless.ErrNoSourcePath` when `Source.Path` is empty or `headless.ErrTooFewTargets` when `Compare.Targets` names fewer than two environments.

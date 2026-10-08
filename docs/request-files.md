# Request file anatomy

## Separators and comments

- Begin each request with a line that starts with `###`. Everything up to the next separator belongs to the same request.
- Lines prefixed with `#`, `//`, or `--` are treated as comments. Metadata directives live inside these comment blocks.
- A block comment starts with `/*` at the beginning of a line and ends at `*/`. Directives inside it are read like any other comment, and a leading `*` on each line is skipped, so a `/** ... */` block with `* @name login` lines works.
- A standalone comment whose content starts with `@name` is treated as a directive.
- At file or request scope, an unknown directive is ignored with a warning. A known directive used in the wrong place is handled the same way.
- Between `@workflow` and the next request, an unknown directive is a parse error. This catches mistakes such as `@stpe` that would otherwise remove a workflow step. Directives attached to requests are still request-scoped, even when a workflow runs those requests.
- After `@mock` and before the response, only `@match` and `@expect` are allowed. Any other directive-shaped comment is a parse error.
- To write a comment that starts like a directive, add another comment marker, for example `## @if ...`.
- A directive problem that does not invalidate the file becomes a warning, not an error. Parsing continues and keeps the valid parts where it can. An unknown option on `@ssh`, `@k8s`, `@sse`, or `@websocket` is dropped, and the rest of the directive still applies. A directive the parser cannot read at all, such as an `@capture` with no usable scope, is dropped completely and reported. Warnings never change the exit code.
- An option may appear only once in a directive. Resterm reports duplicates instead of silently keeping the last value. Repeated `@match json` and `@match json-rules` declarations are merged as described in [Splitting a long matcher](mock-servers.md#splitting-a-long-matcher).
- Write options as `key=value` without spaces around `=`. In most directives, a bare key means `true`. Resterm rejects `key = value`, `key =value`, `key= value`, and `=value` without setting the option. This keeps `persist = false` from enabling persistence by mistake.
- Use `=` for regular options: `timeout<=1s` is an error. `@trace` latency budgets also accept `<=`.
- Quote only the value, as in `cmd="gh auth token"`. A quoted field such as `"timeout=1s"` is a plain word and does not set `timeout`. Directives that accept only options, such as `@settings`, warn about quoted options.
- An empty value such as `strict_hostkey=` is allowed at the end of the line or before another `key=value` option.
- A file or global `@ssh` or `@k8s` profile with spaces around an operator, a missing key, or the wrong operator is unavailable to requests. Requests using an invalid `@ssh` profile report that it was not found; requests using an invalid `@k8s` profile report the profile's error.
- Alternate names count as the same option. For example, you cannot use both `known_hosts` and `known-hosts` on one `@ssh` directive. Empty values are ignored for regular options, but not for switches. `strict_hostkey=` enables the switch, so it conflicts with `strict-hostkey=false`.
- `@compare` requires non-empty values for its baseline and group options. This is stricter than general alias conflict handling: `# @ssh host=h known-hosts=a known_hosts=` is valid, but `# @compare dev stage base=dev baseline=` reports an empty baseline.
- Directives that require a value report `value missing` when left empty. This applies to `@name`, `@operation`, `@grpc-descriptor`, `@grpc-authority`, and `@grpc-metadata`. Some directives deliberately accept an empty value. `@graphql` enables GraphQL, `@query` and `@variables` read the lines below them, and `@grpc-reflection` defaults to on.
- A request directive that replaces one value may appear only once. This includes `@auth`, `@name`, `@timeout`, `@when`, `@for-each`, `@poll`, `@retry`, `@trace`, `@profile`, `@compare`, and the single-value gRPC and GraphQL directives. A second declaration is a parse error, and the file does not run until it is removed. A declaration the parser rejects does not count, so a later one is not reported as a duplicate. A GraphQL directive ignored while GraphQL is off does not count either, but it does produce a warning.
- Directives such as `@tag`, `@capture`, `@assert`, `@apply`, `@var`, `@setting`, and `@body` add to earlier declarations. `@graphql`, `@sse`, and `@websocket` may repeat because `off` resets their state. For GraphQL, the reset also clears `@operation`, `@variables`, and `@query`, so they may be declared again after `@graphql off`. Duplicate directive checks apply only within a request. File directives may repeat because some of them define named profiles.
- Files can be saved with parse errors. The status line shows the number of errors, for example `Saved requests.http (1 parse error)`. Requests cannot run until those errors are fixed.
- In the TUI, live editor diagnostics underline offending text and show `ERR <n>` / `WARN <n>` counts beside the status message. Press `K` in editor normal mode for details on the current line, or `g .` / `:diagnostics` for the complete list. When editor diagnostics are disabled, the existing `WARN line <n>` segment reports warnings from the last matching parse. Parse warnings also appear in the Explain pane for each run.
- When editor diagnostics are disabled, editing hides the `WARN line <n>` segment until the document is parsed again on save, explicit reload, or request execution.
- `resterm run` lists warnings under `WARN` in text output, in the `Warnings:` section of a single-request result, and under `warnings` in JSON.

## Multiline directives

Some directives can span multiple comment lines. Resterm keeps reading while the directive's expression, matcher, or template is incomplete:

```http
# @assert licensing.assertResponse(
#   response,
#   expected,
#   options
# )
```

- RestermScript expressions continue while an opening `(`, `[`, or `{` remains unmatched outside a string or comment. This applies to `@assert`, `@when`, `@skip-if`, `@capture`, `@apply`, `@patch`, `@for-each`, `@if`, `@elif`, `@switch`, and `@case`.
- `@match` continues while a quoted or bracketed option value remains open. See [Splitting a long matcher](mock-servers.md#splitting-a-long-matcher).
- Text captures continue while a `{{` marker remains open. See [Captures](captures.md).
- Continuation lines may use any supported comment marker and do not repeat the directive name. Directives such as `@name`, `@tag`, and `@step` remain on one line.
- Only the expression can keep a directive open. Names, messages, options, and loop variables do not. Separators such as `=>`, `run=`, `as`, and `in` only count outside strings, comments, and nested groups.
- Options and loop variables after the closing delimiter remain part of the directive.
- A new directive, a non-comment line, a request separator, or the end of the file stops collection. Resterm reports an unmatched delimiter at the opening line and drops the incomplete directive.
- Errors inside continued expressions keep their original line and column. Reports and stack frames show the expression on one line.

## Metadata directives

| Directive | Syntax | Description |
| --- | --- | --- |
| `@name` | `# @name identifier` | Friendly name used in the navigator, history, and captures. |
| `@const` | `# @const name value` | Compile-time constant resolved when the file is loaded. Immutable and visible to all requests in the document. |
| `@description` / `@desc` | `# @description ...` | Multi-line description. Lines are joined with newlines. |
| `@tag` / `@tags` | `# @tag smoke billing` | Tags for grouping and filters (comma- or space-separated). |
| `@trace` | `# @trace dns<=40ms total<=200ms tolerance=25ms` | Enable per-phase tracing and optional latency budgets. `@trace off` turns it off. See [Timeline & tracing](ui-tour.md#timeline--tracing). |
| `@no-log` / `@nolog` | `# @no-log` | Prevents the response body snippet from being stored in history. |
| `@log-sensitive-headers` / `@log-secret-headers` | `# @log-sensitive-headers [true\|false]` | Allow allowlisted sensitive headers (Authorization, Proxy-Authorization, and API-token headers such as `X-API-Key`, `X-Access-Token`, and `X-Auth-Key`) to appear in history. Omit it or set it to `false` to keep them masked, which is the default. |
| `@setting` | `# @setting key value` | Set an HTTP, transport, or TLS option such as `timeout`, `proxy`, `max-redirects`, or `max-response-size`. |
| `@settings` | `# @settings key1=val1 key2=val2 ...` | Several settings on one line. Supports the same keys as `@setting` and future prefixes. |
| `@timeout` | `# @timeout 5s` | Equivalent to `@setting timeout 5s`. |

## Body content

- **Inline**: everything after the blank line that separates headers and body.
- **External file**: `< ./payloads/create-user.json` loads the file relative to the request file and sends it as it is. Add `# @body expand` (or `# @body expand-templates`) to expand `{{...}}` templates and `@ path` include lines in the file first. To also search the workspace root and the current working directory, set `RESTERM_ENABLE_FALLBACK=1`.
- **Inline includes**: lines in the body starting with `@ path/to/file` are replaced with the file contents (useful for multi-part templates). Only lines written in the body count. A value placed by a template never becomes an include, even when it contains a line that starts with `@`.
- **XML/SOAP**: inline XML is sent exactly as written after template expansion. XML tags such as `<soap:Envelope>` are body text, not file references.
- **Forced inline body**: add `# @body inline` (or `# @body raw`) when a literal body line intentionally looks like a file reference, such as `< this is just a string`. This only affects parsing. Template expansion and inline includes still work as usual.
- **Multipart**: set `Content-Type: multipart/form-data; boundary=...` and write the parts inline. Everything from the first `--boundary` line to the closing `--boundary--` line is body text, so part lines that start with `#`, `//`, or `--` are sent as written instead of being read as comments. Without a `boundary` in the header, the boundary lines are still kept, but part lines that start with `#` or `//` are read as comments. An `@ path` line inside a part is replaced with the file's bytes unchanged, so binary files work. Resterm sends multipart bodies with CRLF line endings.
- **GraphQL**: handled separately (see [GraphQL](graphql.md)).

A multipart upload with one text field and one file:

```http
### Upload avatar
POST https://example.com/upload
Content-Type: multipart/form-data; boundary=resterm

--resterm
Content-Disposition: form-data; name="title"

Profile photo
--resterm
Content-Disposition: form-data; name="file"; filename="avatar.png"
Content-Type: image/png

@ ./avatar.png
--resterm--
```

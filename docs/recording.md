# Recording traffic

```sh
resterm record --listen 127.0.0.1:9000 --upstream https://service.example.com --out captured.http --mode both
```

Point your application's API base URL at `http://127.0.0.1:9000`. Resterm forwards traffic to `--upstream` and writes recordings as requests finish. `--mode` chooses `requests` (default), `mocks`, or `both`. [`resterm record`](cli/record.md) lists every flag, its default, and the exit codes.

In the TUI, run `:record start --upstream https://service.example.com`, view captures with `:record list`, and stop with `:record stop`. Use `:record as-request` or `:record as-mock` to insert all completed recordings into the current `.http` or `.rest` file. Pass a capture ID to insert one. The file must have a path, and each insertion can be undone. **Save the file after inserting recordings.** They count as exported only after saving.

Recordings stay in memory when you switch files. `:record clear` discards a stopped session. Resterm warns you before quitting while a recording is active or captures are unsaved. `:q!` discards them.

**Redaction uses header and field names.** Saved copies replace known credential headers and query, form, and JSON fields with `REDACTED`, and remove cookies. Repeat `--redact-header` or `--redact-field` to add names.

Free text and values under unrecognized names stay unchanged, even if they contain tokens or passwords. Review recordings before sharing. Before replaying a request that needs credentials, replace its `REDACTED` values with variables. Forwarded traffic keeps its original credentials.

Use `--skip` and `--only` to choose what is recorded. Rules are `[METHOD ]path` with the path in the mock route syntax described under [Route and response syntax](mock-servers.md#route-and-response-syntax), for example `--skip "GET /health"` or `--only /api/{rest...}`. Both flags can be repeated. `--skip` wins. Filtered requests are still forwarded and do not count as skipped captures, so they do not affect the exit status.

Supported bodies are empty, JSON, URL-encoded forms, and UTF-8 text. Gzip is decoded only in recorded copies. Bodies that would change when parsed as request-file syntax are saved in separate files in a `resterm-record-*` directory. Keep that directory beside the request file. Its bodies use the same redaction rules and are read as literal data, without running templates or includes.

Binary, multipart, unsupported encodings, malformed JSON, duplicate JSON keys, oversized bodies, and incomplete bodies cannot be exported. Resterm reports why an export was skipped. Forwarding continues. CONNECT tunnels and protocol upgrades are rejected. Recording WebSocket, gRPC, and SSE traffic is not supported.

The listener uses HTTP. The upstream must be one HTTP(S) origin, such as `https://service.example.com`, without credentials, a path prefix, query, or fragment. TLS uses the system's trusted certificates. Environment proxy settings are ignored. Redirects, CORS, cookies, and response URLs are forwarded unchanged, so redirected requests may bypass the recorder.

Memory, body size, and concurrency are limited. The limits can cause captures or exports to be skipped. Existing recordings are kept and forwarding continues.

Generated requests use `recordedBaseUrl`. Set it to your mock server's address to replay them. Mocks match the method, path, and any query values or JSON fields that were not redacted. Form and text bodies are not used for matching. If several mocks match, the first wins. Use `X-Resterm-Mock: <scenario-name>` to choose a particular response. Resterm does not create response sequences.

Request names come from the method and path, such as `get-users-42`. Their descriptions include the capture ID, status, and timing. Each request asserts the recorded status code, except for redirects, since `resterm run` follows them. Add more `@assert` and `@expect` rules as needed, then check the requests with `resterm run` and the mock calls with `resterm mock verify`.

For a local walkthrough, serve `_examples/recording.http` with `resterm mock _examples/recording.http --addr 127.0.0.1:9001`, record that origin on port 9000, and run `resterm run _examples/recording.http`. The example includes assertions and a mock call expectation.

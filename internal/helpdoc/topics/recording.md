# Recording Traffic

Point your application's API base URL at `http://127.0.0.1:9000`. The recorder forwards HTTP traffic to `--upstream` and saves it as requests or mocks.

```sh
resterm record --listen 127.0.0.1:9000 --upstream https://service.example.com --out captured.http
```

The default mode is `requests`. Use `--mode mocks` or `--mode both` to save responses as mocks. Recordings are written as requests finish. The output file must be new and its parent directory must exist. Ctrl+C allows active requests up to 3 seconds to finish.

In the TUI:

```text
:record start --upstream https://service.example.com
:record list
:record stop
:record as-request
:record as-mock 1
```

Exports insert recordings into the current `.http` or `.rest` file and can be undone. The file must have a path. Without an ID, export commands select all completed recordings. **Save the file after exporting.** `:record status` shows counts. Recordings stay in memory across file switches. `:record clear` discards a stopped session. Stop, export, and save before quitting, or use `:q!` to discard unsaved captures.

Saved copies redact known credential headers and query, form, and JSON fields, and remove cookies. Add names with `--redact-header X-Custom-Key` or `--redact-field privateCredential`. Both flags can be repeated. **Free text and values under unrecognized names are left unchanged**, including tokens and passwords. Review recordings before sharing. Replace `REDACTED` request values with variables before replaying requests that need credentials. Forwarded traffic keeps its original credentials.

Empty bodies, JSON, URL-encoded forms, and UTF-8 text are supported. Gzip is decoded only in recorded copies. Binary, multipart, unsupported encodings, malformed JSON, duplicate JSON keys, oversized bodies, and incomplete bodies cannot be exported. Resterm reports why an export was skipped. Forwarding continues. CONNECT tunnels and protocol upgrades are rejected. WebSocket, gRPC, and SSE recording are unsupported.

Bodies that would change when parsed as request-file syntax are saved in separate files. Keep their `resterm-record-*` directory beside the request file. These bodies use the same redaction rules and are read as literal data, without running templates or includes.

Default limits are 1,000 recordings in memory, 64 MiB each for stored recordings and shared capture buffers, 4 MiB per body before and after decoding, and 32 simultaneous captures. Adjust `--max-entries`, `--max-bytes`, `--body-limit`, and `--capture-concurrency`. Limits can cause captures or exports to be skipped. Existing recordings are kept and forwarding continues. CLI exit status is 0 on success, 1 if any requested capture or export was skipped or recording failed, and 2 for invalid arguments.

Set `recordedBaseUrl` to the mock server address for replay. Mocks match the method, path, and query values and JSON fields that were not redacted. Form and text bodies are not used for matching. The first matching mock wins. Select a particular response with `X-Resterm-Mock: <scenario-name>`. Add `@assert` and `@expect` for tests, then use `resterm run` and `resterm mock verify`. Response sequences are not created automatically.

The listener uses HTTP. The upstream must be an HTTP(S) origin without credentials, a path prefix, query, or fragment. TLS uses the system's trusted certificates. Environment proxy settings are ignored. Redirects, CORS, cookies, and response URLs are forwarded unchanged, so redirected requests may bypass the recorder.

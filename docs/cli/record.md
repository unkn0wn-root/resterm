# resterm record

```sh
resterm record --listen 127.0.0.1:9000 --upstream https://service.example.com --out captured.http --mode both
```

Point your application's API base URL at `http://127.0.0.1:9000`. `--upstream` and `--out` are required. `--mode` accepts `requests` (default), `mocks`, or `both`.

Recordings are written as requests finish. The output file must be new and its parent directory must exist. Some bodies are saved in separate files beside it. After Ctrl+C, active requests get up to 3 seconds to finish. See [Recording traffic](../recording.md) for redaction, filters, replay, and supported traffic.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--listen` | `127.0.0.1:9000` | Local HTTP listener. |
| `--max-entries` | `1000` | Maximum recordings kept in memory. |
| `--max-bytes` | `64MiB` | Separate limits for stored recordings and total capture buffers. |
| `--body-limit` | `4MiB` | Maximum request or response body size, including after decoding. |
| `--capture-concurrency` | `32` | Maximum simultaneous captures. Extra requests are still forwarded. |
| `--redact-header`, `--redact-field` | built-in rules | Additional header or field names to redact. Both flags can be repeated. |
| `--skip`, `--only` | none | Forward without recording, or record only, requests matching `[METHOD ]path`. Both flags can be repeated. `--skip` wins. |

Exit status is 0 on success, 1 if any requested capture or export was skipped or recording failed, and 2 for invalid arguments. HTTP error responses can be recorded and do not cause a nonzero exit status on their own.

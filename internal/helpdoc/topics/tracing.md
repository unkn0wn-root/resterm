# Timeline and Tracing

Enable `@trace` to record DNS, connection, TLS, first-byte, and transfer timing. Budgets use `phase<=duration` notation.

```http
# @trace dns<=100ms ttfb<=500ms total<=1s
GET https://example.com/data
```

Open the Timeline response tab with `g t` to inspect phases and budget overruns. Breaches also raise status bar warnings.

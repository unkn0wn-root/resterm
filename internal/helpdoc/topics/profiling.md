# Profiling Requests

Use `@profile` to repeat one request and summarize its latency distribution.

```http
# @profile count=50 warmup=5 delay=100ms
GET https://example.com/health
```

- `count` sets the number of measured runs. The default is 10. `@profile 50` is the short form.
- `warmup` sets how many runs happen before measuring. Warmup runs are not included in the statistics.
- `delay` sets how long to wait between runs.

An unknown option or invalid value stops the request with a parse error. Profiling does not support gRPC requests.

Latency statistics include only successful measured runs. A measured failure fails the profile. A warmup failure appears as a warning.

The Profile tab opens when the run starts. Percentiles, the histogram, status codes, and failures update after each request.

The tab also shows the change from the previous finished run of the same request with the same environment and delay. Changes under 5% are not colored.

Press `Enter` on a profile entry in History to open it again. Only profile requests that are safe to repeat.

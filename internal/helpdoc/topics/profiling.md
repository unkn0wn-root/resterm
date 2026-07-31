# Profiling Requests

Use `@profile` to repeat one request and summarize its latency distribution.

```http
# @profile count=50 warmup=5 delay=100ms
GET https://example.com/health
```

Warmup runs are excluded from the statistics. The Profile response tab shows percentiles, histograms, and failure counts. Keep profiled requests safe to repeat.

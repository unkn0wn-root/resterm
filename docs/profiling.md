# Profiling

Add `# @profile` to a request to run it repeatedly and measure its latency. Requests run one at a time.

```
### Benchmark health check
# @profile count=50 warmup=5 delay=100ms
GET https://httpbin.org/status/200
```

Options:

- `count` - the number of measured runs. It must be positive and defaults to 10. Use `# @profile 50` as a short form when no other option is set.
- `warmup` - the number of runs to make before measuring. It must be zero or more. Warmup runs are not included in the statistics.
- `delay` - the time to wait between runs. It must be zero or more, for example `250ms`.

If an option is unknown, missing a value, or invalid, a parse error is shown and no request in the file runs until it is fixed. Profiling does not support gRPC requests, so `@profile` on a gRPC request is also a parse error.

How results are counted:

- Latency statistics include only successful measured runs.
- A measured run fails on a transport error, an HTTP status of 400 or higher, a script error, or a failed test. Any measured failure also fails the profile.
- A failed warmup run appears as a warning. It does not fail the profile or change the CLI exit code.
- The wall rate includes delays between requests. The active rate uses only the time spent inside requests.

The response pane's **Profile** tab opens when profiling starts. Percentiles, the histogram, status codes, and failures update after each request. If you switch tabs during the run, you stay on the tab you chose. The Pretty, Raw, and Headers tabs show the last response.

The tab compares the run with the previous finished run of the same request in the same environment with the same delay. The row under the headline numbers shows the change, and the line below it shows when that run happened. Slower latency and a lower success rate or wall rate are shown as warnings. Changes under 5% are not colored. A percentile change is not colored until both runs have enough successful requests for it to differ from the slowest one, 20 for P95 and 100 for P99.

Every profile run is saved to history, even with `@no-log`. Profile history does not store response bodies. Press `Enter` on an entry to reopen the Profile tab, or `p` to inspect the stored JSON. Entries from older versions show counts and latency, but not failure details or status codes.

# Polling and retries

Use `@poll` to repeat a request until its response matches a condition:

```http
### Wait for job
# @poll every=500ms timeout=30s until=response.json().status == "completed"
GET {{base.url}}/jobs/{{job.id}}
```

The first request is sent right away. `every` defaults to `1s`, and `timeout`
defaults to `30s`. `until` must come last because its RestermScript expression
uses the rest of the directive.

Use `@retry` to try a request again after a network error, timeout, or a response
that matches `@retry-when`:

```http
# @retry count=4
# @retry-when contains([429, 502, 503], response.statusCode)
# @retry-backoff exponential(100ms, 2s) jitter=20%
POST {{base.url}}/jobs
```

`count` is the number of retries after the first attempt, so this example allows
five attempts. Without `@retry-when`, Resterm retries only network errors and
attempt timeouts. The backoff values shown above are the defaults. A valid
`Retry-After` header can increase the delay before the next attempt. Resterm
ignores an invalid value and shows one warning.

When `@poll` and `@retry` are used together, each poll gets the full retry count.
Resterm runs captures, assertions, and response test scripts only after the last
response. It also saves only one history entry. Be careful when retrying requests
that can change data, such as `POST`. The server may have accepted an attempt
even if Resterm did not receive its response.

Resterm prepares the request once and uses the same values for every attempt.
Templates such as `{{$uuid}}` and `{{$timestamp}}` are not evaluated again, and
authentication is not refreshed during polling or retries. The body is kept in
memory so it can be sent again. The history duration includes all attempts and
waits.

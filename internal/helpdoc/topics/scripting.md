# Scripting API

JavaScript blocks can modify a request before sending it or test a response afterward. Script lines start with `>`.

```http
# @script test
> client.test("status", function () {
>   tests.assert(response.statusCode === 200, "expected 200")
> })
```

Pre-request scripts use `request.setHeader`, `request.setBody`, and `vars.set`. Use RestermScript directives for short expressions and JavaScript for stateful logic.

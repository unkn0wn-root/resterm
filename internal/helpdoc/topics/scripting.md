# Scripting API

JavaScript blocks can modify a request before sending it or test a response afterward. Script lines start with `>`.

```http
# @script test
> client.test("status", function () {
>   tests.assert(response.statusCode === 200, "expected 200")
> })
```

Pre-request scripts can change headers and bodies with `request.setHeader` and `request.setBody`, or set variables with `vars.set`. Values are sent as written. Use `vars.interpolate("{{base}}/users/{{id}}")` to fill placeholders in a string.

Use RestermScript directives for short expressions. Use JavaScript when you need to keep state between script blocks.

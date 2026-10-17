# Scripting API

Use RestermScript directives for short expressions, `@rts pre-request` blocks to change a request, and `@rts test` blocks for longer checks. Use JavaScript blocks to change a request or test its response when you already have the script in JavaScript or need something RestermScript cannot do, such as saving a response body to a file. Script lines start with `>`.

```http
# @script test
> client.test("status", function () {
>   tests.assert(response.statusCode === 200, "expected 200")
> })
```

Pre-request scripts can change headers and bodies with `request.setHeader` and `request.setBody`, or set variables with `vars.set`. Values are sent as written. Use `vars.interpolate("{{base}}/users/{{id}}")` to fill placeholders in a string.

Variables declared in a block are not visible in the next block. Pre-request blocks share values set with `vars.set`, and `@capture` keeps a value for later requests, as does `vars.global.set` outside `@rts test` blocks.

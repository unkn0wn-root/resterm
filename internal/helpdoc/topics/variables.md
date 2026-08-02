# Variables and Environments

Interpolate values with `{{name}}`. Values come from request files, environment files, captures, and built-ins such as `{{$uuid}}` and `{{$timestamp}}`.

```http
# @global apiUrl https://api.example.com
# @request userId 42
GET {{apiUrl}}/users/{{userId}}
```

Declare values at `@const`, `@request`, `@file`, or `@global` scope. Append `-secret` to mask them in summaries. `@capture` stores response data for later requests. Press `Ctrl+E` to select an environment.

An undefined variable in any part of an outbound request blocks the send. Previews keep the `{{...}}` placeholder and list the missing names.

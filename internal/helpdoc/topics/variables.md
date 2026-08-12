# Variables and Environments

Interpolate values with `{{name}}`. Values come from request files, environment files, captures, and built-ins such as `{{$uuid}}` and `{{$timestamp}}`.

```http
# @global apiUrl https://api.example.com
# @request userId 42
GET {{apiUrl}}/users/{{userId}}
```

Declare values at `@const`, `@request`, `@file`, or `@global` scope. Append `-secret` to mask them in summaries. `@capture` stores response data for later requests. Press `Ctrl+E` to select an environment.

Built-in helpers need no declaration: `{{$uuid}}`, `{{$timestamp}}`, `{{$timestampMs}}`, `{{$timestampISO8601}}`, `{{$randomInt}}`, `{{$randomString}}`, `{{$randomName}}`, `{{$randomEmail}}`, `{{$randomChoice("a", "b")}}`, and the `{{$fake.*}}` family (`person`, `firstName`, `lastName`, `email`, `username`, `company`, `domain`, `city`, `country`, `phone`, `word`, `sentence`). Timestamps accept offsets such as `{{$timestampISO8601 - 90m}}`; `{{$randomInt(1, 6)}}` and `{{$randomString(24)}}` take arguments.

An undefined variable in any part of an outbound request blocks the send. Previews keep the `{{...}}` placeholder and list the missing names.

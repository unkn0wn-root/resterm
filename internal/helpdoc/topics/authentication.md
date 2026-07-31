# Authentication

Use `@auth` to attach credentials. Types include `basic`, `bearer`, `apikey`, custom headers, `oauth2`, and `command` for tokens printed by a CLI.

```http
# @auth bearer {{auth.token}}
GET https://api.example.com/me
```

Scope with `@auth file` or `@auth global` to inherit credentials, and opt out for one request with `@auth none`. OAuth 2.0 tokens are fetched, cached, and refreshed automatically.

Related: `:help variables`, `:help scripting`.

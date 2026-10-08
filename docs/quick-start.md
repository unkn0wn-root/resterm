# Quick start

1. Save the example below as a `.http` or `.rest` file in your project directory. You can also use a sample from `_examples/`.
2. Run `resterm --workspace path/to/project`.
3. Expand the file in the navigator sidebar with `→` or `Space`. Highlight a request and press `Ctrl+Enter` to send it. With a request highlighted, `Enter` also sends it and `Space` previews it.
4. Read the response in the tabs on the right: Pretty, Raw, Headers, Diff, Compare, or History.

A minimal `.http` file looks like this:

```http
### Fetch Status
# @name health
GET https://httpbin.org/status/204
User-Agent: resterm
Accept: application/json

### Create Resource
# @name create
POST https://httpbin.org/anything
Content-Type: application/json

{
  "id": "{{$uuid}}",
  "note": "created from Resterm"
}
```

To compare environments, press `g+c`. This runs the current request against the `--compare` targets, or the targets in its `@compare` directive, and shows the results beside the editor.

## Initializing a Project

`resterm init` writes a starter workspace so you can send requests right away:

```bash
mkdir my-api && cd my-api
resterm init
```

Run `resterm` in the same directory. Press `g Shift+M` to start the local mock server, then use `Ctrl+Enter` to try the sample requests. Press `Ctrl+E` to switch between the local `dev` and `test` environments. [`resterm init`](cli/init.md) covers the templates and flags.

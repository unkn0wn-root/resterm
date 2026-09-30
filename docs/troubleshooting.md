# Troubleshooting

- Use `Ctrl+P` to force a reparse if the navigator seems out of sync with editor changes.
- If a template fails to expand (undefined variable), Resterm blocks the send and reports the missing variable. This covers URLs, query parameters, headers, auth values, expanded bodies, gRPC targets and messages, and WebSocket steps. Errors in the URL, headers, and inline or file bodies name the variable and point at the placeholder with its file, line, and column, and the TUI quotes the line. A failing `{{= ... }}` expression keeps its script diagnostics and points at the expression. Explain previews keep the placeholder intact and list the unresolved names.
- A placeholder that never closes, such as `{{token}`, is sent as literal text. The editor diagnostics warn about it before you send.
- Combine `@capture request ...` with test scripts to assert on response headers without cluttering file/global scopes.
- Inline curl import works best with single commands. Complex shell pipelines may need manual cleanup.
- `Ctrl+Shift+V` pins the focused response pane, which is useful for comparing the last good response with the current attempt.
- Keep secrets in environment files or runtime globals marked as `-secret`.

For additional questions or feature requests, open an issue on GitHub.

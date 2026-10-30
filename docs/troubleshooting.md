# Troubleshooting

- Use `Ctrl+P` to force a reparse if the navigator seems out of sync with editor changes.
- If a variable is undefined, Resterm reports its name and does not send the request. This applies to URLs, query parameters, headers, auth values, expanded bodies, gRPC targets and messages, and WebSocket steps.

  For URLs, headers, and inline or file bodies, the error points to the placeholder's file, line, and column and quotes the line. A failing `{{= ... }}` expression keeps its script diagnostics and points to the expression. Explain previews leave the placeholder visible and list unresolved names.
- A placeholder that never closes, such as `{{token}`, is sent as literal text. The editor diagnostics warn about it before you send.
- Combine `@capture request ...` with test scripts to assert on response headers without filling up the file and global scopes.
- Inline curl import works best with single commands. Complex shell pipelines may need manual cleanup.
- `Ctrl+Shift+V` pins the focused response pane, which is useful for comparing the last good response with the current attempt.
- When no clipboard tool is available, for example on a remote host over SSH, Resterm sends copied text to the terminal with OSC 52 and the status line says "Sent via OSC 52". The terminal has to allow it.

  Kitty, Ghostty, WezTerm, Alacritty, foot, and Windows Terminal allow it by default. iTerm2 needs "Applications in terminal may access clipboard" in Settings > General > Selection. macOS Terminal and GNOME Terminal do not support it. In tmux, add `set -g set-clipboard on`. Text over about 768 KB is not sent unless you raise the [OSC 52 limit](configuration.md#clipboard). `p` still pastes from the editor register. To paste the terminal clipboard, use the terminal's paste shortcut.
- Keep secrets in environment files or runtime globals marked as `-secret`.

For other questions or feature requests, open an issue on GitHub.

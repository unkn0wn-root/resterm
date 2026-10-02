# Configuration

- Config directory: `$HOME/Library/Application Support/resterm` (macOS), `%APPDATA%\resterm` (Windows), or `$HOME/.config/resterm` (Linux/Unix). Override with `RESTERM_CONFIG_DIR`.
- History file: `<config-dir>/history.db` (no fixed entry limit). On first launch after an upgrade, Resterm imports `<config-dir>/history.json` into it when present. If the file is corrupted, Resterm moves it to `history.db.corrupt-<timestamp>` and starts a fresh one.
- Settings file: `<config-dir>/settings.toml` (created when you first change preferences such as the default theme).
- Theme directory: `<config-dir>/themes/` (override with `RESTERM_THEMES_DIR`). Drop `.toml` or `.json` files here to make them available in the selector.
- Runtime globals and file captures are scoped to the full environment selection and the document. Resterm drops them when you clear globals or switch environments.

## Editor diagnostics

Diagnostics are enabled by default for `.http`, `.rest`, and unnamed request buffers. They show the parser's existing warnings and errors, such as unknown directives, mistyped option keys, missing values, conflicting options, and placeholders such as `{{token}` that never close. Diagnostics refresh when you leave insert mode, without saving the buffer or changing the active request. They do not refresh while you type, even if you pause. Existing marks stay until an edit changes the text they point at. They then clear until you return to normal mode. Changes made in normal mode, such as undo or deletion, use a short debounce.

`K` (Shift+K) in normal mode opens a popup beside the cursor with diagnostics on that line. Findings under the cursor appear first. Errors come before warnings. `Enter` opens related documentation when available, `PgUp` / `PgDown` scroll long messages, and `Esc` or cursor movement closes the popup. On a line without diagnostics, `K` opens contextual help as usual. In insert mode, `K` types normally.

Use `] d` / `[ d` or `:diagnostics next` / `:diagnostics prev` to visit diagnostic locations. Navigation wraps at the ends of the buffer. `:diagnostics` opens the full list. Empty-line and EOF findings use a line-number marker when there is no text to underline.

`:diagnostics off` and `:diagnostics on` change the setting for this session. To disable diagnostics at startup, put this in `<config-dir>/settings.toml`:

```toml
[editor]
diagnostics = false
```

For `settings.json`, use `{"editor":{"diagnostics":false}}`. Omitting the setting enables diagnostics. Session overrides do not change the stored preference.

This feature reports request-file parsing findings. It does not evaluate scripts, resolve runtime variables, or perform network checks, and it does not add script-language diagnostics to `.rts` files. It does not change how requests are checked when they run or how headless runs report warnings.

Themes can customize `[styles.editor_diagnostic_warning]` and `[styles.editor_diagnostic_error]` using the usual style fields, including `foreground` and `underline`. The defaults use plain terminal underlines and different colors for warnings and errors. Your terminal does not need undercurl support.

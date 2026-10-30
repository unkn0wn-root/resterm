# Configuration

- Config directory: `$HOME/Library/Application Support/resterm` (macOS), `%APPDATA%\resterm` (Windows), or `$HOME/.config/resterm` (Linux/Unix). Override with `RESTERM_CONFIG_DIR`.
- History file: `<config-dir>/history.db` (no fixed entry limit). On first launch after an upgrade, Resterm imports `<config-dir>/history.json` into it when present. If the file is corrupted, Resterm moves it to `history.db.corrupt-<timestamp>` and starts a fresh one.
- Settings file: `<config-dir>/settings.toml` (created when you first change preferences such as the default theme). If the settings file has an error, Resterm starts with default settings and does not save changes until you fix the file and restart Resterm.
- Theme directory: `<config-dir>/themes/` (override with `RESTERM_THEMES_DIR`). Drop `.toml` or `.json` files here to make them available in the selector.
- Runtime globals and file captures are scoped to the full environment selection and the document. Resterm drops them when you clear globals or switch environments.

## Editor diagnostics

Editor diagnostics mark errors and warnings in `.http`, `.rest`, and unnamed request buffers. They are enabled by default and show parsing problems such as unknown directives, mistyped option keys, missing values, conflicting options, and unclosed placeholders such as `{{token}`.

The marks refresh when you leave insert mode. You do not need to save, and the active request stays the same. They do not refresh while you type, even if you pause. Editing marked text clears its marks until you return to normal mode. Changes made in normal mode, such as undo or deletion, refresh after a short delay.

`K` (Shift+K) in normal mode opens a popup beside the cursor with diagnostics on that line. Findings under the cursor appear first. Errors come before warnings. `Enter` opens related documentation when available, `PgUp` / `PgDown` scroll long messages, and `Esc` or cursor movement closes the popup. On a line without diagnostics, `K` opens contextual help as usual. In insert mode, `K` types normally.

Use `] d` / `[ d` or `:diagnostics next` / `:diagnostics prev` to move between marked locations. Navigation wraps at both ends of the buffer. `:diagnostics` opens the full list. Findings on an empty line or at the end of the file (EOF) use a line-number marker when there is no text to underline.

`:diagnostics off` and `:diagnostics on` change the setting for this session. To disable diagnostics at startup, put this in `<config-dir>/settings.toml`:

```toml
[editor]
diagnostics = false
```

For `settings.json`, use `{"editor":{"diagnostics":false}}`. Omitting the setting enables diagnostics. Session overrides do not change the stored preference.

Diagnostics check request-file syntax. They do not run scripts, resolve runtime variables, check the network, or check the script language in `.rts` files. Request validation and warning reports during headless runs work the same whether editor diagnostics are enabled or disabled.

Themes can customize `[styles.editor_diagnostic_warning]` and `[styles.editor_diagnostic_error]` using the usual style fields, including `foreground` and `underline`. The defaults use plain terminal underlines and different colors for warnings and errors. Your terminal does not need undercurl support.

## Clipboard

When no clipboard tool is available, for example over SSH, Resterm sends copied text to the terminal with OSC 52. A copy over the OSC 52 limit is not sent. It stays in the editor register, and the status line shows the limit. The default limit is 1 MiB, which holds about 768 KB of text. To change it, put this in `<config-dir>/settings.toml`:

```toml
[clipboard]
osc52_limit = "8MiB"
```

For `settings.json`, use `{"clipboard":{"osc52_limit":"8MiB"}}`. The size can be plain bytes or use KB, KiB, MB, MiB, GB, or GiB. Every unit is a power of two, so `1KB` is 1024 bytes. The value must be greater than zero. Leave the setting out to use the default.

Raise the limit for terminals that accept larger copies, such as Kitty. Lower it to `16KiB` for mosh, which cuts longer copies.

# Configuration

Resterm supports custom settings, key bindings, themes, and persisted pane layouts.

```toml
[bindings]
show_context_help = ["shift+k"]
next_diagnostic = ["] d"]
previous_diagnostic = ["[ d"]
toggle_help = ["?"]
```

Binding and theme files may be TOML or JSON and live in the config directory, which `RESTERM_CONFIG_DIR` overrides. Open the theme selector with `Ctrl+Alt+T` and save the current layout with `g Shift+L`.

Editor diagnostics are enabled by default. To disable them at startup, add this to `settings.toml`:

```toml
[editor]
diagnostics = false
```

The JSON equivalent in `settings.json` is `{"editor":{"diagnostics":false}}`. `:diagnostics on` and `:diagnostics off` override the setting for the current session. Saving a theme or layout preserves the stored setting.

Themes may customize `[styles.editor_diagnostic_warning]` and `[styles.editor_diagnostic_error]`. Both use underlined text by default.

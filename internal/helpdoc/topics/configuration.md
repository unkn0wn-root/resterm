# Configuration

Resterm supports custom settings, key bindings, themes, and persisted pane layouts.

```toml
[bindings]
show_context_help = ["shift+k"]
toggle_help = ["?"]
```

Binding and theme files may be TOML or JSON and live in the config directory, which `RESTERM_CONFIG_DIR` overrides. Open the theme selector with `Ctrl+Alt+T` and save the current layout with `g Shift+L`.

# Key bindings

The first table lists the default shortcuts by task. [Custom bindings](#custom-bindings) shows how to change them, and the [binding reference](#binding-reference) lists the action IDs you can rebind.

## All shortcuts

| Action | Shortcut |
| --- | --- |
| Send active request | `Ctrl+Enter` / `Cmd+Enter` / `Alt+Enter` / `Ctrl+J` / `Ctrl+M` |
| Toggle help overlay | `?` |
| Show diagnostics on the current line, or contextual help | `K` (editor normal mode) |
| Next / previous editor diagnostic | `] d` / `[ d` (editor normal mode) |
| Toggle editor insert mode | `i` / `Esc` |
| Cycle focus (navigator -> editor -> response) | `Tab` / `Shift+Tab` |
| Focus navigator / editor / response panes | `g+r` / `g+i` / `g+p` |
| Open timeline tab | `Ctrl+Alt+L` (or `g+t`) |
| WebSocket commands (Stream tab) | `g+w`, then `i`/`p`/`c`/`l` |
| Adjust sidebar or side-by-side editor/response width | `g+h` / `g+l` (contextual) |
| Adjust stacked editor/response height, or collapse / expand current navigator branch | `g+j` / `g+k` (contextual) |
| Collapse all / expand all in navigator | `g+Shift+J` / `g+Shift+K` |
| Toggle sidebar / editor / response minimize | `g+1` / `g+2` / `g+3` |
| Zoom focused pane / clear zoom | `g+z` / `g+Z` |
| Stack/inline response pane | `g+s` (stack) / `g+v` (inline) |
| Jump to top/bottom of focused response tab | `g+g` / `G` |
| Cycle Raw tab mode (text / hex / base64, summary for large binary) | `g+b` |
| Load full Raw dump (hex) | `g+Shift+D` |
| Save response body / open externally | `g+Shift+S` / `g+Shift+E` |
| Run compare sweep (`@compare` or `--compare` targets) | `g+c` |
| Start/stop workspace mock server | `g+Shift+M` |
| Capture focused HTTP response as a mock | `g+a` |
| Navigator filter | `/` to focus, then type to search files, requests, and tags. `Esc` clears the filter and chips |
| Navigator: toggle method filter for selected request | `m` (repeat to switch/clear) |
| Navigator: toggle tag filters from selected item | `t` (repeat to toggle) |
| Navigator: jump to selected request/workflow in editor | `l` / `r` (when a request or workflow is highlighted) |
| Open environment selector | `Ctrl+E` |
| Save file | `Ctrl+S` |
| Save layout (prompt) | `g+Shift+L` |
| Open file/workspace popup | `Ctrl+O` |
| Open current/selected file in external editor | `g+e` |
| New scratch buffer | `Ctrl+T` |
| Reparse current document | `Ctrl+P` (also `Ctrl+Alt+P`) |
| Refresh workspace files | `Ctrl+Shift+O` |
| Split response vertically / horizontally | `Alt+V` / `Alt+H` |
| Pin or unpin response pane | `Ctrl+Shift+V` |
| Choose target pane for next response | `Ctrl+F` or `Ctrl+B`, then arrow keys or `h` / `l` |
| Show globals summary / clear globals and cookies | `Ctrl+G` / `Ctrl+Shift+G` (or `g Shift+G`) |
| Quit | `Ctrl+Q` (or `Ctrl+D`) |

## Custom bindings

Resterm looks for `${RESTERM_CONFIG_DIR}/bindings.toml` first, then `${RESTERM_CONFIG_DIR}/bindings.json`. The config directory defaults to `~/.config/resterm`. If neither file exists, Resterm uses the built-in bindings. Example:

```toml
[bindings]
save_file = ["ctrl+s"]
set_main_split_horizontal = ["g s", "ctrl+alt+s"]
send_request = ["ctrl+enter", "cmd+enter"]
show_context_help = ["shift+k"]
```

- Modifiers use `+` (`ctrl+shift+o`), while chord steps are separated by spaces (`"g s"`).
- A binding can have at most two steps. `send_request` must stay a single step so it works inside the editor.
- If the file has an unknown action ID or a duplicate binding, Resterm rejects the file, logs the error, and keeps the defaults.

## Binding reference

| Action ID | Description | Default bindings |
| --- | --- | --- |
| `cycle_focus_next` | Cycle focus forward (skips editor insert mode). | `tab` |
| `cycle_focus_prev` | Cycle focus backward. | `shift+tab` |
| `open_env_selector` | Open environment picker. | `ctrl+e` |
| `show_globals` | Show global variable summary. | `ctrl+g` |
| `clear_globals` | Clear global variables and cookies. | `ctrl+shift+g`, `g shift+g` |
| `save_file` | Save the current `.http` / `.rest` file. | `ctrl+s` |
| `save_layout` | Prompt to save the current layout (splits, widths) to settings. | `g shift+l` |
| `toggle_response_split_vertical` | Toggle response inline vs vertical split. | `alt+v` |
| `toggle_response_split_horizontal` | Toggle response inline vs horizontal split. | `alt+h` |
| `toggle_pane_follow_latest` | Toggle follow-latest for the focused response pane. | `ctrl+shift+v` |
| `toggle_help` | Open/close the help overlay. | `?` (aka `shift+/`) |
| `show_context_help` | Show diagnostics on the editor's current line, or open contextual documentation when there are none. | `shift+k` (soft default) |
| `next_diagnostic` / `previous_diagnostic` | Move between editor diagnostics, wrapping at document boundaries. | `] d`, `[ d` (soft defaults) |
| `show_status_message` | Show current editor diagnostics, or the current status message when there are none. | `g .` (soft default) |
| `open_path_modal` | Open the filesystem picker for a supported file or workspace. | `ctrl+o` |
| `reload_workspace` | Rescan the workspace root(s). | `ctrl+shift+o`, `g shift+o` |
| `open_new_file_modal` | Open the "New Request" modal. | `ctrl+n` |
| `open_file_in_editor` | Open the current or selected supported file in `$RESTERM_EDITOR`, `$VISUAL`, or `$EDITOR`. | `g e` |
| `open_theme_selector` | Open theme selector. | `ctrl+alt+t`, `g m`, `g shift+t` |
| `open_temp_document` | Open a scratch document. | `ctrl+t` |
| `reparse_document` | Reparse the active buffer. | `ctrl+p`, `ctrl+alt+p`, `ctrl+shift+t` |
| `reload_file_from_disk` | Reload the active file from disk (discarding unsaved buffer changes). | `g shift+r` |
| `select_timeline_tab` | Focus the Timeline tab. | `ctrl+alt+l`, `g t` |
| `quit_app` | Quit Resterm. | `ctrl+q`, `ctrl+d` |
| `send_request` | Send the active request (single-step only). | `ctrl+enter`, `cmd+enter`, `alt+enter`, `ctrl+j`, `ctrl+m` |
| `explain_request` | Prepare an Explain preview for the active request without sending it. | `g x` |
| `cancel_run` | Cancel the in-flight request, compare, profile, or workflow run. | `ctrl+c` |
| `copy_response_tab` | Copy the focused Pretty/Raw/Headers response tab to the clipboard. | `ctrl+shift+c`, `g y` |
| `toggle_mock_server` | Start or stop the workspace mock server. | `g shift+m` |
| `capture_mock_response` | Append the focused live/pinned HTTP response as a mock block. | `g a` |

| Action ID | Description | Default bindings | Repeatable |
| --- | --- | --- | --- |
| `sidebar_width_decrease` / `sidebar_width_increase` | Shrink or grow the sidebar when the navigator is focused. In side-by-side layout, resize the editor and response width. | `g h`, `g l` | ✓ |
| `sidebar_height_decrease` / `sidebar_height_increase` | Collapse or expand the selected navigator branch. In stacked layout, resize the editor and response height. | `g j`, `g k` | ✓ |
| `workflow_height_increase` / `workflow_height_decrease` | Collapse all / expand all navigator branches. | `g shift+j`, `g shift+k` | ✓ |
| `focus_requests` / `focus_response` / `focus_editor_normal` | Jump directly to a pane. | `g r`, `g p`, `g i` | ✗ |
| `set_main_split_horizontal` / `set_main_split_vertical` | Stack vs side-by-side editor/response. | `g s`, `g v` | ✗ |
| `start_compare_run` | Trigger compare sweep for the current request. | `g c` | ✗ |
| `toggle_ws_console` | Enter WebSocket command mode (`i` console, `p` ping, `c` close, `l` clear). | `g w` | ✗ |
| `toggle_sidebar_collapse` / `toggle_editor_collapse` / `toggle_response_collapse` | Collapse/expand panes. | `g 1`, `g 2`, `g 3` | ✗ |
| `toggle_zoom` / `clear_zoom` | Zoom current region / clear zoom. | `g z`, `g shift+z` | ✗ |

`send_request` is part of the editor's "send on Ctrl+Enter" handling, so keep it a single step. The `show_context_help`, `show_status_message`, `next_diagnostic`, and `previous_diagnostic` shortcuts are soft defaults. If you bind one of their keys to another action, your binding wins and that default is dropped. Explicit bindings still follow the conflict rules above.

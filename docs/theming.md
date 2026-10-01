# Theming

Resterm lets you override its Lip Gloss styles. Fonts and ultimate colour rendering still come from your terminal emulator; the theme controls which colours Resterm asks the terminal to use.

## Where themes live

- Default directory: `<config-dir>/themes/`
- Override with `RESTERM_THEMES_DIR`
- Sample: `_examples/themes/aurora.toml`
- Switch at runtime with `Ctrl+Alt+T` (or press `g` then `m`). The selection persists in `settings.toml`.

## Theme anatomy

Theme files can be TOML or JSON. Unspecified fields inherit defaults.

```toml
[metadata]
name = "Oceanic"
author = "You"
description = "Cool dusk palette"

[styles.header_title]
foreground = "#5fd1ff"
bold = true

[colors]
pane_border_focus_file = "#1f6feb"
pane_border_focus_editor = "#56a9dd"
pane_border_focus_response = "#33c481"
pane_active_foreground = "#f8faff"

[editor_metadata]
comment_marker = "#4c566a"

[[command_segments]]
background = "#3b4252"
key = "#88c0d0"
text = "#eceff4"

[status_bar]
base = "#000000"

[status_bar.info]
foreground = "#eff6ff"
background = "#2563eb"

[status_bar.tests_pass]
foreground = "#ecfeff"
background = "#0e7490"

[status_bar.tests_fail]
foreground = "#fff1f2"
background = "#be123c"

[status_bar.tests_error]
foreground = "#faf5ff"
background = "#7e22ce"

[status_bar.file]
foreground = "#f9fafb"
background = "#404040"
```

### Sections and fields

| Section | Keys | Notes |
| --- | --- | --- |
| `[metadata]` | `name`, `description`, `author`, `version`, `tags[]` | Informational only; shown in the selector. |
| `[styles.*]` | `browser_border`, `editor_border`, `response_border`, `navigator_title`, `navigator_title_selected`, `navigator_subtitle`, `navigator_subtitle_selected`, `navigator_badge`, `navigator_tag`, `navigator_detail_title`, `navigator_detail_value`, `navigator_detail_dim`, `app_frame`, `header`, `header_title`, `header_value`, `header_icon`, `header_label`, `header_help`, `header_warn`, `header_separator`, `status_bar`, `status_bar_info`, `status_bar_key`, `status_bar_value`, `command_bar`, `command_bar_hint`, `cli_run_picker`, `cli_run_picker_selected`, `cli_run_picker_cursor`, `cli_run_picker_cursor_selected`, `response_search_highlight`, `response_search_highlight_active`, `tabs`, `tab_active`, `tab_inactive`, `notification`, `error`, `success`, `header_brand`, `command_divider`, `pane_title`, `pane_title_file`, `pane_title_requests`, `pane_title_editor`, `pane_title_response`, `pane_divider`, `editor_hint_box`, `editor_hint_item`, `editor_hint_selected`, `editor_hint_annotation`, `list_item_title`, `list_item_description`, `list_item_selected_title`, `list_item_selected_description`, `list_item_dimmed_title`, `list_item_dimmed_description`, `list_item_filter_match`, `response_content`, `response_content_raw`, `response_content_headers`, `response_selection`, `response_cursor`, `stream_content`, `stream_timestamp`, `stream_direction_send`, `stream_direction_receive`, `stream_direction_info`, `stream_event_name`, `stream_data`, `stream_binary`, `stream_summary`, `stream_error`, `stream_console_title`, `stream_console_mode`, `stream_console_status`, `stream_console_prompt`, `stream_console_input`, `stream_console_input_focused` | Accept `foreground`, `background`, `border_color`, `border_background`, `border_style` (`normal`, `rounded`, `thick`, `double`, `ascii`, `block`), plus booleans `bold`, `italic`, `underline`, `faint`, `strikethrough`, and `align` (`left`, `center`, `right`). |
| `[colors]` | `pane_border_focus_file`, `pane_border_focus_requests`, `pane_border_focus_editor`, `pane_border_focus_response`, `pane_active_foreground`, `git_branch`, `git_modified`, `git_added`, `git_untracked`, `git_deleted`, `git_renamed`, `git_conflict`, `method_get`, `method_post`, `method_put`, `method_patch`, `method_delete`, `method_head`, `method_options`, `method_grpc`, `method_ws`, `method_default` | Frequently reused colours for pane borders, active text, Git markers, and method badges. |
| `[status_bar]` | `base`, plus `[status_bar.info]`, `[status_bar.warn]`, `[status_bar.error]`, `[status_bar.success]`, `[status_bar.tests_pass]`, `[status_bar.tests_fail]`, `[status_bar.tests_error]`, `[status_bar.file]`, `[status_bar.focus]`, `[status_bar.mode]`, `[status_bar.editor]`, `[status_bar.mock]`, `[status_bar.record]`, `[status_bar.zoom]`, `[status_bar.minimized]`, `[status_bar.version]`, `[status_bar.user]`, `[status_bar.host]` with `foreground` and `background` | Controls the segmented bottom status bar. Segment entries inherit the built in palette. The `tests_*` entries colour test result blocks separately from matching request status blocks. `[status_bar.editor]` colours the editor cursor position (`Ln/Col`), shown faint by default. Set it to override. `[status_bar.mock]` colours the mock server segment, which switches to the warn colours while a reload error is active. `[status_bar.record]` colours the traffic recorder segment, which switches to the warn colours once a capture limit is reached. `base` fills otherwise empty status bar cells when set. Omit it to leave them uncoloured. |
| `[editor_metadata]` | `comment_marker`, `directive_default`, `value`, `setting_key`, `setting_value`, `request_line`, `request_separator`, `rts_keyword_default`, `rts_keyword_decl`, `rts_keyword_control`, `rts_keyword_literal`, `rts_keyword_logical`, `rts_function`, `[editor_metadata.directive_colors]` | Controls metadata highlighting inside the editor. The `rts_*` keys colour `.rts` files: keyword classes and, for `rts_function`, function and call names. |
| `[[header_segments]]` | `background`, `foreground`, `border`, `accent` | Accepted for older themes but ignored. The header uses `header` for its background, `pane_divider` for the bottom rule, `header_icon` for segment icons, `header_label` for the `env`, `workspace`, `mock`, `rec`, `req`, and Help labels, `header_value` for values, `header_separator` for gaps, `header_help` for the Help key, `header_warn` for warnings, and `header_brand` for the left block. Test results use `success` and `error`. The active request uses its method colour. |
| `[[command_segments]]` | `background`, `border`, `key`, `text` | Colour sets for contextual command bar hints. Hints render flat by default; set `background` to a colour to enable keycap chips, or to `none` to disable them explicitly. `border` is accepted for compatibility. |

The segmented bottom bar uses `[status_bar]` for section backgrounds and text colours. Legacy `[styles.status_bar*]` entries remain accepted for older themes, but they do not control the segmented section palette.

Setting `editor_metadata.directive_default` recolours every built-in directive (`@name`, `@tag`, etc.) that still uses the inherited default. Specify entries inside `[editor_metadata.directive_colors]` only when you need a directive to diverge from that default.

Set `editor_metadata.request_line` to recolour the full request line (`POST https://…`). If you omit it, Resterm falls back to the directive default.
Use `editor_metadata.request_separator` for the `###` section dividers. `editor_metadata.comment_marker` colours ordinary `#`, `//`, `--`, and `/* … */` comments. Directive names and values keep their metadata colours. Comment markers inside multipart bodies, mock responses, and script blocks are treated as data and are not coloured.

`styles.stream_*` keys control the transcript viewer (events, timestamps, direction badges). `styles.stream_console_*` tweak the interactive WebSocket console (prompt, status line, input field).

`styles.cli_run_picker*` keys only affect the interactive `resterm run` request picker. If they are omitted, the picker inherits the general list item styles where possible.

`navigator_*` styles control the unified sidebar tree, and `styles.list_item_*` keys continue to power history/picker list rows. `styles.response_content`, `styles.response_content_raw`, `styles.response_content_headers`, `styles.response_selection`, and `styles.response_cursor` colour the response panes (with the general key applied first, then the tab-specific override for Raw and Headers).
Pane border titles inherit the active or inactive border colour by default; set a `pane_title_*` foreground when you want a title colour to diverge.

## Testing a theme

```bash
export RESTERM_THEMES_DIR="$(pwd)/_examples/themes"
resterm
```

Inside Resterm, press `g` then `m` (or `Ctrl+Alt+T`) and pick “Aurora” for a dark setup or “Daybreak” for light terminals. Quit and restart to confirm the theme persists. If a theme fails to parse, Resterm logs the error and falls back to the default palette.

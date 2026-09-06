# Commands and Shortcuts

Press `:` outside editor insert mode to open the Vim-style command line. Type to filter commands and press `Tab` to complete the selected suggestion. Path arguments use the same popup: `Tab` descends into a directory without running the command, and paths containing whitespace are quoted automatically.

`Ctrl+O` opens the path popup directly. Type to filter, select with `Up` / `Down` or `Ctrl+P` / `Ctrl+N`, use `Tab` to descend, and press `Enter` to open the selected file or workspace.

```text
:w                 save
:q                 quit when clean
:q!                quit without saving
:help authentication
:man requests      alias for :help requests
:docs authentication
:edit examples/basic.http
:mock start --source examples/mock.http
:diagnostics       show all editor warnings and errors
:diagnostics next  jump to the next diagnostic
:diagnostics prev  jump to the previous diagnostic
:diagnostics off   disable diagnostics for this session
:diagnostics on    enable diagnostics for this session
```

`Up` / `Down` or `Ctrl+P` / `Ctrl+N` select a suggestion. `Enter` runs an explicit selection. Without one it runs the text in the prompt.

In editor normal mode, `K` (Shift+K) shows warnings and errors on the current line, or opens contextual documentation when the line has no diagnostics. Inside the diagnostic popup, `Enter` opens related documentation, `PgUp` / `PgDown` scroll long messages, and `Esc` closes it. Moving the cursor also closes the popup. `] d` and `[ d` move to the next or previous diagnostic, wrapping at the ends of the buffer. These shortcuts are soft defaults and can be rebound.

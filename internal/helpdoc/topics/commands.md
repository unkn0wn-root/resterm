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
```

`Up` / `Down` or `Ctrl+P` / `Ctrl+N` select a suggestion. `Enter` runs an explicit selection. Without one it runs the text in the prompt.

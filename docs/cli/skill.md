# resterm skill

`resterm skill` prints or installs a skill for coding agents such as Claude Code and Codex. The skill teaches an agent to write request files, run them with `resterm run`, and read the JSON report and exit codes. It also tells the agent never to start the TUI, to keep secrets out of request files, and to leave masked values alone.

```bash
# Print the skill
resterm skill

# Install it for Claude Code, for all projects
resterm skill --agent claude

# Install it for Codex, for all projects
resterm skill --agent codex

# Install it for both
resterm skill --agent claude --agent codex
```

`--agent claude` writes the skill to `~/.claude/skills/resterm`, or to `$CLAUDE_CONFIG_DIR/skills/resterm` when `CLAUDE_CONFIG_DIR` is set. `--agent codex` writes it to `~/.agents/skills/resterm`. Gemini CLI and Copilot CLI read that folder too. Cursor and OpenCode read both `~/.agents/skills` and `~/.claude/skills`, so one install is enough for them. On Windows, `~` is your user folder. Missing folders are created, and the command prints the path of each folder it writes.

`--dir` writes the skill to a `resterm` folder inside the directory you give and prints that folder's path. The directory is created if it does not exist. To use the skill in one project only, run the command in the project with `--dir .claude/skills` or `--dir .agents/skills`. For other agents, pass the directory where they look for skills. `--agent` and `--dir` cannot be used together.

## Updating

The skill comes with Resterm, so it always matches the version you have. After you update Resterm, run the same `resterm skill` command again.

Each install replaces the `resterm` folder, so files that a newer version no longer has are removed. A folder counts as a skill when it holds a `SKILL.md` file whose frontmatter has `name: resterm`. If `resterm` is anything else, for example a clone of the Resterm repository or the `resterm` binary, nothing is changed and the command fails with an error that names it. A link named `resterm` is refused too, even when it points to a skill, such as a link made by `npx skills add`. Update the skill where the link points, or remove the link. With more than one `--agent`, a refused folder does not stop the others, and the command exits `1`.

## Without Resterm installed

`npx skills add unkn0wn-root/resterm` installs the skill straight from the repository. That copy follows the main branch, which can be newer than your Resterm. Add a release tag to match your version, such as `npx skills add unkn0wn-root/resterm@v1.18.0`.

## Flags

| Flag | Description |
| --- | --- |
| `--agent <name>` | Install the skill for `claude` or `codex` instead of printing it. Repeat it to install for both. |
| `--dir <dir>` | Install the skill in `<dir>/resterm` instead of printing it. |

`resterm skill` exits `0` on success, `2` for an unknown flag or agent, extra arguments, a missing or empty flag value, or `--agent` with `--dir`, and `1` for any other error, such as a `resterm` folder that is not a skill or a file named `skill` in the current directory.

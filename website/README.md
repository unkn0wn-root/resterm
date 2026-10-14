# resterm web

Resterm's landing page and documentation, built with [Astro](https://astro.build) and TypeScript.

## Commands

Requires Node 22.12 or newer. `.node-version` pins Node 24.

```bash
npm install
npm run dev      # http://localhost:4321
npm run build    # writes dist/
npm run preview  # serves dist/
npm test         # unit tests, including the docs renderer
npm run check    # checks Astro and TypeScript
```

From the repo root, `make website-dev` starts the dev server. `make website-check` runs type checks, tests, and a build.
`make website-deploy` starts the Website workflow on `main` to check, build, and publish to resterm.app. Sign in with `gh auth login` first.

## The tour

The landing page tour renders captured Resterm terminal output. `src/content/tour/tour.json` defines each step's keys, its initial `focus`, and the `panes` visitors can interact with.

Run `make website-tour` from the repo root after TUI changes. It runs Resterm against `tour.http` in both themes, with the editor shown and hidden (`g 2`). It starts a mock server on `127.0.0.1:8080`, so port 8080 must be free.

Commit the generated files in `src/content/tour/`:

- `frames-*.json`: the full screen for each step.
- `panes-*.json`: each pane's focused screen and scrollable content, captured with mouse-wheel events.

The page loads pane data as the tour nears the viewport. It loads the `-editor-hidden` variants when the editor is hidden. With a mouse or trackpad, visitors can click a pane to focus it and scroll longer editor or response content.

## Docs come from docs/

The site reads `../docs/**/*.md` at build time and creates a page for each file. Edit the source files in `docs/` to update the documentation.

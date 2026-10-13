# resterm web

Resterm landing page and the documentation.
Static site built with [Astro](https://astro.build) and TypeScript.

## Commands

Node 22.12 or newer. `.node-version` pins 24.

```bash
npm install
npm run dev      # http://localhost:4321
npm run build    # writes dist/
npm run preview  # serves dist/
npm test         # unit tests, including a full pass over the real docs
npm run check    # type check
```

From the repo root, `make website-dev`, `make website-check` and `make website-deploy` do the same.
`make website-deploy` runs the Website workflow on `main`, which checks, builds and publishes to resterm.app. It needs `gh` to be logged in.

## The tour

The landing page tour shows real Resterm screens, not screenshots. `src/content/tour/tour.json` lists the steps and the keys that reach each one.
`make website-tour` runs the Resterm TUI against `tour.http` and writes every step's screen to `frames-dark.json` and `frames-light.json`, and again with the editor hidden (`g 2`) to the `-editor-hidden` files. The page loads those only when a visitor hides the editor.
Run it after UI changes and commit the frames. It serves mocks on `127.0.0.1:8080`, so that port must be free.

## Docs come from docs/

The site does not keep its own copy of the docs. At build time it reads `../docs/**/*.md` and turns each file into one page.
Screenshots are read from `../_media` and converted to WebP at build time.

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

## Docs come from docs/

The site does not keep its own copy of the docs. At build time it reads `../docs/**/*.md` and turns each file into one page.
Screenshots are read from `../_media` and converted to WebP at build time.

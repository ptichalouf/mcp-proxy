# Management UI

The dashboard served by `mcp-proxy -web`. It is a small React + Vite single-page
app, built to static files and compiled into the binary with `//go:embed` (see
`../web.go`).

## `dist/` is committed on purpose

`web/dist` is checked into the repository. `//go:embed all:web/dist` is resolved
at compile time, so without it `go build ./...` fails — and anyone building
`mcp-proxy` from source would need a Node toolchain to build a Go binary. Keeping
the artifact in the tree means `go install github.com/tbxark/mcp-proxy@latest`
still works with nothing but Go installed.

The trade-off is that `dist/` has to be regenerated whenever `src/` changes, and
that both go in the same commit.

## Rebuilding

```sh
make web          # from the repository root
# or:
cd web && npm ci && npm run build
```

`npm run build` runs `tsc` before `vite build`, so a type error fails the build
rather than shipping.

## Layout

```
web/
├── index.html         Vite entry point
├── src/
│   ├── api.ts         typed client for /api/mcp/*
│   ├── i18n.ts        English + French strings, locale detection
│   ├── components/    dashboard, marketplace, config drawer, log viewer
│   └── ...
└── dist/              build output — committed, embedded
```

## Internationalisation

English is the default and the fallback for any missing key. French is selected
automatically when the browser reports a `fr` locale, and either language can be
chosen from the header; the choice is stored in `localStorage`.

Catalog entries carry their descriptions in both languages (`description` and
`descriptionFr` in `../catalog.go`), so switching language never needs a round
trip to the server.

## Development

```sh
cd web && npm run dev
```

Vite proxies `/api` to `http://localhost:9090` (see `vite.config.ts`), so run
`mcp-proxy -web` alongside it to develop against a real backend.

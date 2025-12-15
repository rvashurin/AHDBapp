# Repository Guidelines

## Project Structure & Module Organization

- `ahdb.go`: CLI importer that reads AuctionDB saved variables from stdin and writes to MySQL (`ahdb` DB).
- `schema.sql`: MySQL schema for `items`, `scanmeta`, and `auctions`.
- `lua2json/`: Go package used to convert Lua saved variables to JSON.
- `cmd/ahdbweb/`: PoC local web app (API + embedded UI).
  - `cmd/ahdbweb/web/`: static assets embedded into the binary (HTML/JS/CSS).
- `*.sh`: helper scripts (Lua→JSON conversion, etc.).

## Build, Test, and Development Commands

- `go test ./...`: compile/typecheck all packages (there are currently no unit tests).
- `MYSQL_PASSWORD=... go run ahdb.go < /path/to/AuctionDB.lua`: import scans/items into MySQL.
- `MYSQL_PASSWORD=... go run ./cmd/ahdbweb`: run the local UI at `http://127.0.0.1:8080`.
- `go build ./cmd/ahdbweb`: build the web app binary.

DB configuration is via env vars:
- `MYSQL_USER` (default `root`)
- `MYSQL_PASSWORD`
- `MYSQL_CONNECTION_INFO` (default `tcp(:3306)`)

## Coding Style & Naming Conventions

- Go: run `gofmt` on all changed `.go` files; follow standard Go naming and package layout (new binaries go under `cmd/<name>/`).
- DB/API: keep queries parameterized; use `context` timeouts for DB calls; return JSON with stable field names.
- Frontend: keep `cmd/ahdbweb/web/app.js` as vanilla JS (no build tooling); prefer small, pure helper functions.

## Testing Guidelines

- Minimum expectation: `go test ./...` passes and the UI loads locally.
- Manual smoke checks for UI/API changes: item search, series load, box-plot hover histogram.
- If adding tests, use the standard Go `testing` package (`*_test.go`, table-driven where appropriate).

## Commit & Pull Request Guidelines

- Commit messages are typically short and imperative (e.g., `Add web app`, `linter fixes`).
- PRs should explain intent and impact, link issues when relevant, and include screenshots for UI changes.
- Note any schema changes and how to migrate (`schema.sql`), and avoid committing credentials or local data dumps.


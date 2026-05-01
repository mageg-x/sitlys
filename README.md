# Sitlys

Go + SQLite self-hosted analytics system with:

- single-binary backend
- embedded static admin console
- multi-website analytics
- user / role / website-scope permissions
- pixel collection
- attribution / funnel / retention / revenue analysis
- public read-only share links
- multilingual frontend source based on Vue 3 + `<script setup>`

## Directory Layout

- `server/`: Go backend source
- `server/embed/`: frontend build output embedded into the Go binary
- `web/`: Vue 3 frontend source
- `docs/`: product documents

## Backend

Run locally:

```bash
go run ./server -addr 127.0.0.1:8080 -db ./data/sitlys.db
```

Build:

```bash
go build -o sitlys ./server
```

## Frontend

The repository already contains a runnable embedded static console in `server/embed/`.

Vue source lives in `web/`. Its configured build target is `../server/embed`.

```bash
cd web
npm install
npm run build
```

This turn intentionally did not run the frontend build because WSL 9p filesystem builds are slow and the backend path was prioritized.

## Tracker

```html
<script async data-website-id="YOUR_WEBSITE_ID" src="http://127.0.0.1:8080/tracker.js"></script>
```

Custom event:

```js
window.sitlys.track("signup", { plan: "pro" });
```

Revenue event:

```js
window.sitlys.revenue("purchase", 99.9, "USD", { orderNo: "A1001" });
```

## Notes

- default database: SQLite
- default session duration: 30 days
- backend uses standard library HTTP server plus embedded static files
- public share pages are read-only
- event ingest path is asynchronous: HTTP -> in-memory queue -> single writer worker -> batched SQLite transactions
- SQLite stores both raw facts (`events`, `sessions`) and aggregate tables (`agg_*`) for faster analytics queries

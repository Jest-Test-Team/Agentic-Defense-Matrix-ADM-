# ADM Battle Console (dashboard)

A realtime web dashboard for the Agentic Defense Matrix red/blue/green exercise.
It shows, live, how the defenses are holding: service health, a battle scoreboard
(attacks, block rate, detection rate, remediations, MTTR), a per-technique
blocked-vs-landed breakdown, a streaming event feed, and recent attack→remediation
sessions.

**Live:** https://jest-test-team.github.io/Agentic-Defense-Matrix-ADM/

## What it talks to

The dashboard is a **static site** (Next.js `output: "export"`) — no server. In the
browser it:

- polls the analysis API `GET /api/stats` and `/api/timeline` every few seconds,
- opens a Server-Sent-Events stream `GET /api/stream` for live events,
- probes `/health`, `/ready` (Neon), and the gateway `/v1/health`.

Because GitHub Pages is HTTPS, the API must also be HTTPS (see the Caddy setup in
`docs/architecture/live-deployment.md`). The endpoint is **runtime-configurable**
so the same build can point at any deployment:

- `?api=https://host&gw=https://host` in the URL, or
- the **Endpoint** box at the bottom of the page (persisted to `localStorage`).

The default is `https://api.dennisleehappy.org` (Caddy fronts both APIs; it routes
`/v1/*` to the gateway and everything else to the analysis engine, so one host
serves both).

## Internationalization

English and Traditional Chinese (繁體中文), toggled in the header and persisted;
first visit auto-detects `zh-*` browsers. All strings live in `lib/i18n.ts` — add a
language by extending the `translations` map. The collapsible **About** panel
explains the project's purpose and the three teams in both languages.

## Layout

| File | Purpose |
|---|---|
| `app/page.tsx` | The dashboard (single client component + small presentational pieces) |
| `app/globals.css` | Dark-theme styling (colors from the repo's dataviz palette) |
| `lib/api.ts` | Endpoint config + typed fetch helpers |
| `lib/i18n.ts` | EN / zh-Hant dictionary |
| `next.config.mjs` | Static export + `basePath` for GitHub Pages |

## Develop

```bash
cd dashboard
npm install
npm run dev            # http://localhost:3000
# point it at a running API, e.g. http://localhost:3000/?api=http://localhost:8090&gw=http://localhost:8080
```

## Build & deploy

`npm run build` produces the static `out/`. Deployment is automatic:
`.github/workflows/pages.yml` builds and publishes to GitHub Pages on every push
to `dashboard/**` (baking in the correct Pages base path).

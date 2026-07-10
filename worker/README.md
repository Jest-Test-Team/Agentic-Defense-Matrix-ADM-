# ADM API HTTPS proxy (Cloudflare Worker)

The GitHub Pages dashboard is HTTPS; the ADM API on the OCI box is HTTP, so the
browser blocks the calls (mixed content). This Worker fronts the API over HTTPS
and adds CORS, so the dashboard can reach it. Free tier is plenty.

## Deploy (one-time)

```bash
cd worker
npm install
npx wrangler login          # opens a browser; authorize your Cloudflare account
npm run deploy
```

`wrangler deploy` prints the URL, e.g. `https://adm-api-proxy.<your-subdomain>.workers.dev`.

## Point the dashboard at it

Open the dashboard with the Worker URL (analysis and gateway are the same host —
the Worker routes `/v1/*` to the gateway and everything else to the analysis API):

```
https://jest-test-team.github.io/Agentic-Defense-Matrix-ADM/?api=https://adm-api-proxy.<sub>.workers.dev&gw=https://adm-api-proxy.<sub>.workers.dev
```

…or paste that URL into the dashboard's **Endpoint** box and click *Save & reload*.
The choice persists in `localStorage`, so afterwards the bare dashboard URL works.

## When the OCI IP changes

Each `replace_instance` deploy gives the box a new public IP. Update the Worker's
targets and redeploy:

```bash
npx wrangler deploy \
  --var ADM_ANALYSIS_URL:http://<new-ip>:8090 \
  --var ADM_GATEWAY_URL:http://<new-ip>:8080
```

(or edit `vars` in `wrangler.jsonc`). A stable IP (OCI reserved public IP) or a
domain in front of the box avoids this.

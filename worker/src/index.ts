// ADM API HTTPS proxy (Cloudflare Worker).
//
// The dashboard is served from GitHub Pages over HTTPS, but the ADM analysis API
// (:8090) and gateway (:8080) on the OCI box are plain HTTP — browsers block
// HTTPS->HTTP requests (mixed content). This Worker sits in front over HTTPS and
// proxies to the box server-side (no mixed content), adding permissive CORS and
// passing streaming bodies through unchanged (so /api/stream SSE keeps working).
//
// Routing by path so a single Worker URL serves both:
//   /v1/*   -> gateway  (ADM_GATEWAY_URL,  default :8080)
//   else    -> analysis (ADM_ANALYSIS_URL, default :8090)
//
// Point the dashboard at it:  ?api=https://<worker>.workers.dev&gw=https://<worker>.workers.dev

export interface Env {
  ADM_ANALYSIS_URL?: string;
  ADM_GATEWAY_URL?: string;
}

const CORS = {
  "Access-Control-Allow-Origin": "*",
  "Access-Control-Allow-Methods": "GET,POST,OPTIONS",
  "Access-Control-Allow-Headers": "Content-Type, X-Session-ID, Authorization",
  "Access-Control-Max-Age": "86400",
};

export default {
  async fetch(req: Request, env: Env): Promise<Response> {
    if (req.method === "OPTIONS") {
      return new Response(null, { status: 204, headers: CORS });
    }

    const url = new URL(req.url);
    const analysis = (env.ADM_ANALYSIS_URL || "http://161.33.209.244:8090").replace(/\/$/, "");
    const gateway = (env.ADM_GATEWAY_URL || "http://161.33.209.244:8080").replace(/\/$/, "");
    const base = url.pathname.startsWith("/v1/") ? gateway : analysis;
    const upstream = base + url.pathname + url.search;

    // Forward the request; let the platform stream the response body (SSE-safe).
    const init: RequestInit = {
      method: req.method,
      headers: filterReqHeaders(req.headers),
      body: req.method === "GET" || req.method === "HEAD" ? undefined : req.body,
      redirect: "manual",
    };

    let res: Response;
    try {
      res = await fetch(upstream, init);
    } catch (e) {
      return new Response(
        JSON.stringify({ error: "upstream unreachable", upstream, detail: String(e) }),
        { status: 502, headers: { "Content-Type": "application/json", ...CORS } },
      );
    }

    const headers = new Headers(res.headers);
    for (const [k, v] of Object.entries(CORS)) headers.set(k, v);
    // Don't let a proxied cache directive break SSE / live polling.
    if (url.pathname.endsWith("/stream")) headers.set("Cache-Control", "no-cache");

    return new Response(res.body, { status: res.status, statusText: res.statusText, headers });
  },
};

// Strip hop-by-hop / host headers so the upstream sees a clean request.
function filterReqHeaders(h: Headers): Headers {
  const out = new Headers();
  const drop = new Set(["host", "cf-connecting-ip", "cf-ipcountry", "cf-ray", "cf-visitor", "x-forwarded-proto", "x-forwarded-for"]);
  h.forEach((v, k) => {
    if (!drop.has(k.toLowerCase())) out.set(k, v);
  });
  return out;
}

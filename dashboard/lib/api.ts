// API client + config for the ADM live dashboard.
//
// The analysis engine (Rust/axum) and gateway are HTTP on the OCI instance. The
// endpoints are resolvable at runtime so this static build can be pointed at any
// deployment (e.g. once a domain + TLS is in front) without a rebuild:
//   ?api=https://host   -> analysis base URL   (also persisted to localStorage)
//   ?gw=https://host     -> gateway base URL

// Default endpoints: the box's APIs fronted by Caddy auto-HTTPS at a real
// domain, so the HTTPS Pages site can reach them (no mixed content). The same
// host serves both — Caddy routes /v1/* to the gateway, everything else to the
// analysis API. Override at runtime with ?api=…&gw=… or the Endpoint box.
const DEFAULT_ANALYSIS = "https://api.dennisleehappy.org";
const DEFAULT_GATEWAY = "https://api.dennisleehappy.org";

export interface ApiConfig {
  analysis: string;
  gateway: string;
}

export function getConfig(): ApiConfig {
  if (typeof window === "undefined") {
    return { analysis: DEFAULT_ANALYSIS, gateway: DEFAULT_GATEWAY };
  }
  const q = new URLSearchParams(window.location.search);
  const analysis =
    q.get("api") || window.localStorage.getItem("adm_api") || DEFAULT_ANALYSIS;
  const gateway =
    q.get("gw") || window.localStorage.getItem("adm_gw") || DEFAULT_GATEWAY;
  if (q.get("api")) window.localStorage.setItem("adm_api", q.get("api")!);
  if (q.get("gw")) window.localStorage.setItem("adm_gw", q.get("gw")!);
  return { analysis: analysis.replace(/\/$/, ""), gateway: gateway.replace(/\/$/, "") };
}

export function setConfig(analysis: string, gateway: string) {
  if (typeof window === "undefined") return;
  window.localStorage.setItem("adm_api", analysis);
  window.localStorage.setItem("adm_gw", gateway);
}

export interface TechniqueStat {
  technique: string;
  blocked: number;
  landed: number;
}

export interface Stats {
  attacks: number;
  blocked: number;
  landed: number;
  detected: number;
  remediations: number;
  residual_risk: number;
  block_rate: number;
  landing_rate: number;
  detection_rate: number;
  mttr_seconds: number | null;
  elastic_enabled: boolean;
  by_technique: TechniqueStat[];
}

export interface SessionRow {
  session_id: string;
  technique: string;
  variant: string;
  target: string;
  severity: number;
  attack_ts: string;
  attack_outcome: string;
  remediation_ts: string | null;
  remediation_outcome: string | null;
  mttr_seconds: number | null;
}

export interface SystemService {
  name: string;
  tech: string;
  category: string;
  detail: string;
  status: "up" | "down" | "disabled";
  hint?: string | null;
}

export interface LlmProvider {
  role: "primary" | "fallback";
  name: string;
  status: "up" | "down" | "unconfigured";
  active: boolean;
}

export interface LlmStatus {
  active: "primary" | "fallback" | "none";
  providers: LlmProvider[];
}

export interface BattleEvent {
  id?: string;
  ts?: string;
  team: string;
  kind: string;
  technique?: string;
  variant?: string;
  session_id?: string;
  target?: string;
  outcome?: string;
  severity?: number;
  latency_ms?: number;
  detail?: string;
  labels?: Record<string, string>;
}

export interface SearchHit {
  score: number;
  event: BattleEvent;
}

export interface SearchResult {
  total: number;
  hits: SearchHit[];
  error?: string;
}

const TIMEOUT = 6000;

async function getJSON<T>(url: string): Promise<T> {
  const ctrl = new AbortController();
  const t = setTimeout(() => ctrl.abort(), TIMEOUT);
  try {
    const res = await fetch(url, { signal: ctrl.signal, cache: "no-store" });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    return (await res.json()) as T;
  } finally {
    clearTimeout(t);
  }
}

export async function probe(url: string): Promise<boolean> {
  const ctrl = new AbortController();
  const t = setTimeout(() => ctrl.abort(), TIMEOUT);
  try {
    const res = await fetch(url, { signal: ctrl.signal, cache: "no-store" });
    return res.ok;
  } catch {
    return false;
  } finally {
    clearTimeout(t);
  }
}

export const api = {
  stats: (c: ApiConfig) => getJSON<Stats>(`${c.analysis}/api/stats`),
  system: (c: ApiConfig) => getJSON<{ services: SystemService[] }>(`${c.analysis}/api/system`),
  llm: (c: ApiConfig) => getJSON<LlmStatus>(`${c.analysis}/api/llm`),
  // Full-text search over the Elasticsearch (Bonsai) `adm-battle-events` index.
  // Accepts Lucene query_string syntax; returns the raw ES response, which we
  // normalize into { total, hits[] }.
  search: async (c: ApiConfig, q: string): Promise<SearchResult> => {
    const raw = await getJSON<any>(`${c.analysis}/api/search?q=${encodeURIComponent(q || "*")}`);
    if (raw?.error) return { total: 0, hits: [], error: raw.error };
    const h = raw?.hits;
    const total = typeof h?.total === "object" ? h.total.value : h?.total ?? 0;
    const hits: SearchHit[] = (h?.hits ?? []).map((x: any) => ({ score: x._score ?? 0, event: x._source ?? {} }));
    return { total, hits };
  },
  timeline: (c: ApiConfig, limit = 40) =>
    getJSON<{ sessions: SessionRow[] }>(`${c.analysis}/api/timeline?limit=${limit}`),
  analysisReady: (c: ApiConfig) => probe(`${c.analysis}/ready`),
  analysisHealth: (c: ApiConfig) => probe(`${c.analysis}/health`),
  gatewayHealth: (c: ApiConfig) => probe(`${c.gateway}/v1/health`),
};

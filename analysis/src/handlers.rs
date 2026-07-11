use std::convert::Infallible;
use std::sync::Arc;

use axum::extract::{Query, State};
use axum::http::StatusCode;
use axum::response::sse::{Event as SseEvent, Sse};
use axum::response::IntoResponse;
use axum::Json;
use futures::stream::Stream;
use serde::Deserialize;
use serde_json::json;
use tokio_stream::wrappers::BroadcastStream;
use tokio_stream::StreamExt;

use crate::model::{AppState, BattleEvent, EventRow, SessionRow};

pub async fn health() -> impl IntoResponse {
    Json(json!({ "status": "ok" }))
}

// ---- /api/system : consolidated status of every stack component -------------

enum Probe {
    /// GET the URL, healthy if status < 400.
    Http(&'static str),
    /// TCP connect to host:port.
    Tcp(&'static str),
    /// TCP connect to host:port taken from an env var (falling back to a
    /// default) — used for the host-networked watchdog, whose address differs
    /// between the bridge and host-network deployments.
    TcpEnv(&'static str, &'static str),
    /// SELECT 1 against Postgres.
    Db,
    /// Elasticsearch enabled + reachable.
    Elastic,
    /// GET {ADM_LLM_BASE_URL}/models (bearer) — the hosted LLM.
    Llm,
    /// Always up (the analysis engine answering means it's up).
    SelfUp,
}

struct Svc {
    name: &'static str,
    tech: &'static str,
    category: &'static str,
    detail: &'static str,
    probe: Probe,
    /// Optional components are "disabled" (not "down") when unreachable —
    /// otel/watchdog/control-plane are dropped on the 1 GB micro.
    optional: bool,
    /// Extra context shown in the dashboard detail view (e.g. why a component
    /// is host-only or how to enable it). None for the always-on core services.
    hint: Option<&'static str>,
}

fn catalog() -> Vec<Svc> {
    use Probe::*;
    vec![
        Svc { name: "API Gateway", tech: "Go (Echo)", category: "Edge", detail: "Request interception, semantic analysis, routing", probe: Http("http://gateway:8080/v1/health"), optional: false, hint: None },
        Svc { name: "Analysis Engine", tech: "Rust (axum)", category: "Edge", detail: "Battle event ingest, scoring, dashboard API", probe: SelfUp, optional: false, hint: None },
        Svc { name: "SIEM Engine", tech: "Go", category: "Detection", detail: "Correlation engine + Redis streams", probe: Http("http://siem:9091/health"), optional: false, hint: None },
        Svc { name: "Policy Engine", tech: "OPA", category: "Detection", detail: "Rego authorization decisions", probe: Http("http://policy:8181/health"), optional: false, hint: None },
        Svc { name: "Planner Agent", tech: "Go + gRPC", category: "Agents", detail: "Task decomposition", probe: Http("http://planner:9081/health"), optional: false, hint: None },
        Svc { name: "Executor Agent", tech: "Go + gRPC", category: "Agents", detail: "Tool execution (Docker API)", probe: Http("http://executor:9082/health"), optional: false, hint: None },
        Svc { name: "Summarizer Agent", tech: "Go + gRPC", category: "Agents", detail: "Response summarization", probe: Http("http://summarizer:9083/health"), optional: false, hint: None },
        Svc { name: "LLM Backend", tech: "Groq → X.AI", category: "Agents", detail: "Chat-completion inference (Groq primary, X.AI fallback)", probe: Llm, optional: false, hint: None },
        Svc { name: "Sandboxing", tech: "Docker API", category: "Runtime", detail: "Ephemeral per-agent containers (via executor)", probe: Http("http://executor:9082/health"), optional: false, hint: None },
        Svc { name: "Storage", tech: "Redis 7", category: "Runtime", detail: "SIEM hot path + session store", probe: Tcp("redis:6379"), optional: false, hint: None },
        Svc { name: "Durable Log", tech: "Neon Postgres", category: "Data", detail: "Retained battle-event log", probe: Db, optional: false, hint: None },
        Svc { name: "Search / Aggregation", tech: "Elasticsearch", category: "Data", detail: "Full-text + aggregation index", probe: Elastic, optional: true, hint: Some("Optional. Set ELASTIC_URL (e.g. a free Bonsai cluster) to enable full-text search + aggregation; the durable log in Postgres works without it.") },
        Svc { name: "Observability", tech: "OpenTelemetry", category: "Ops", detail: "Traces, metrics, logs", probe: Http("http://otel-collector:13133/"), optional: true, hint: Some("Off by default — the collector doesn't fit the 1 GB micro. Enable with ADM_ENABLE_OTEL=true and re-run battle-up.sh (recommended on an A1 / 12 GB box). Every service already exports OTLP to it.") },
        Svc { name: "Endpoint Watchdog", tech: "Rust", category: "Ops", detail: "Host agent: macOS EndpointSecurity / Windows WFP syscall interception", probe: TcpEnv("ADM_WATCHDOG_ADDR", "watchdog:9084"), optional: true, hint: Some("Host-only. It runs ON the protected macOS/Windows endpoint (host network + syscall hooks), not as a cloud service — so it reads 'disabled' on the Linux box. Run adm-watchdog on your endpoint, or enable a demo container on A1 with ADM_ENABLE_WATCHDOG=true.") },
    ]
}

pub async fn system(State(st): State<Arc<AppState>>) -> impl IntoResponse {
    let http = reqwest::Client::builder()
        .timeout(std::time::Duration::from_secs(3))
        .build()
        .unwrap_or_default();

    let results = futures::future::join_all(catalog().into_iter().map(|s| {
        let http = http.clone();
        let st = st.clone();
        async move {
            let up = match &s.probe {
                Probe::SelfUp => true,
                Probe::Http(url) => http.get(*url).send().await.map(|r| r.status().as_u16() < 400).unwrap_or(false),
                Probe::Tcp(hp) => tokio::time::timeout(std::time::Duration::from_secs(2), tokio::net::TcpStream::connect(*hp)).await.map(|r| r.is_ok()).unwrap_or(false),
                Probe::TcpEnv(var, default) => {
                    let hp = std::env::var(var).unwrap_or_else(|_| (*default).to_string());
                    tokio::time::timeout(std::time::Duration::from_secs(2), tokio::net::TcpStream::connect(hp)).await.map(|r| r.is_ok()).unwrap_or(false)
                }
                Probe::Db => sqlx::query("SELECT 1").execute(&st.pool).await.is_ok(),
                Probe::Elastic => st.elastic.enabled(),
                Probe::Llm => probe_llm(&http).await,
            };
            let status = if up { "up" } else if s.optional { "disabled" } else { "down" };
            json!({ "name": s.name, "tech": s.tech, "category": s.category, "detail": s.detail, "status": status, "hint": s.hint })
        }
    }))
    .await;

    Json(json!({ "services": results }))
}

/// LLM Backend is "up" if *either* the primary (Groq) or the fallback (X.AI)
/// provider answers — the Go client fails over transparently, so from the
/// stack's perspective inference is available as long as one is reachable.
async fn probe_llm(http: &reqwest::Client) -> bool {
    let (base, key) = primary_llm();
    if probe_llm_url(http, &base, &key).await {
        return true;
    }
    let (fb_base, fb_key) = fallback_llm();
    !fb_base.is_empty() && probe_llm_url(http, &fb_base, &fb_key).await
}

fn primary_llm() -> (String, String) {
    (
        std::env::var("ADM_LLM_BASE_URL").unwrap_or_default(),
        std::env::var("ADM_LLM_API_KEY").unwrap_or_default(),
    )
}

fn fallback_llm() -> (String, String) {
    (
        std::env::var("ADM_LLM_FALLBACK_BASE_URL").unwrap_or_default(),
        std::env::var("ADM_LLM_FALLBACK_API_KEY").unwrap_or_default(),
    )
}

/// Friendly provider name derived from the API host, so the dashboard can label
/// "Groq" vs "X.AI (Grok)" vs "Ollama" without hardcoding it server-side.
fn provider_label(base: &str) -> &'static str {
    let b = base.to_ascii_lowercase();
    if b.contains("groq") {
        "Groq"
    } else if b.contains("x.ai") || b.contains("xai") {
        "X.AI (Grok)"
    } else if b.contains("openai") {
        "OpenAI"
    } else if b.is_empty() {
        "None"
    } else {
        "Ollama / custom"
    }
}

async fn probe_llm_url(http: &reqwest::Client, base: &str, key: &str) -> bool {
    if base.is_empty() {
        return false;
    }
    let mut req = http.get(format!("{}/models", base.trim_end_matches('/')));
    if !key.is_empty() {
        req = req.bearer_auth(key);
    }
    req.send().await.map(|r| r.status().as_u16() < 400).unwrap_or(false)
}

/// GET /api/llm : per-provider status of the primary (Groq) and fallback (X.AI)
/// backends, plus which one is currently serving traffic.
pub async fn llm() -> impl IntoResponse {
    let http = reqwest::Client::builder()
        .timeout(std::time::Duration::from_secs(3))
        .build()
        .unwrap_or_default();

    let (p_base, p_key) = primary_llm();
    let (f_base, f_key) = fallback_llm();

    let (p_up, f_up) = futures::future::join(
        probe_llm_url(&http, &p_base, &p_key),
        probe_llm_url(&http, &f_base, &f_key),
    )
    .await;

    // Active provider: primary if reachable, else the fallback if reachable.
    let active = if p_up {
        "primary"
    } else if f_up && !f_base.is_empty() {
        "fallback"
    } else {
        "none"
    };

    let status = |configured: bool, up: bool| {
        if !configured {
            "unconfigured"
        } else if up {
            "up"
        } else {
            "down"
        }
    };

    let providers = json!([
        {
            "role": "primary",
            "name": provider_label(&p_base),
            "status": status(!p_base.is_empty(), p_up),
            "active": active == "primary",
        },
        {
            "role": "fallback",
            "name": provider_label(&f_base),
            "status": status(!f_base.is_empty(), f_up),
            "active": active == "fallback",
        }
    ]);

    Json(json!({ "active": active, "providers": providers }))
}

pub async fn ready(State(st): State<Arc<AppState>>) -> impl IntoResponse {
    match sqlx::query("SELECT 1").execute(&st.pool).await {
        Ok(_) => (StatusCode::OK, Json(json!({ "ready": true }))),
        Err(e) => (
            StatusCode::SERVICE_UNAVAILABLE,
            Json(json!({ "ready": false, "error": e.to_string() })),
        ),
    }
}

/// Ingest a battle event: durable Postgres write, optional Elastic index, live
/// SSE fan-out.
pub async fn ingest(
    State(st): State<Arc<AppState>>,
    Json(ev): Json<BattleEvent>,
) -> impl IntoResponse {
    let id = match uuid::Uuid::parse_str(&ev.id) {
        Ok(u) => u,
        Err(_) => uuid::Uuid::new_v4(),
    };
    let labels = serde_json::to_value(&ev.labels).unwrap_or_else(|_| json!({}));

    let res = sqlx::query(
        "INSERT INTO battle_events \
         (id, ts, team, kind, technique, variant, session_id, target, outcome, severity, latency_ms, detail, labels) \
         VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) \
         ON CONFLICT (id) DO NOTHING",
    )
    .bind(id)
    .bind(ev.ts)
    .bind(&ev.team)
    .bind(&ev.kind)
    .bind(&ev.technique)
    .bind(&ev.variant)
    .bind(&ev.session_id)
    .bind(&ev.target)
    .bind(&ev.outcome)
    .bind(ev.severity)
    .bind(ev.latency_ms)
    .bind(&ev.detail)
    .bind(&labels)
    .execute(&st.pool)
    .await;

    if let Err(e) = res {
        return (StatusCode::INTERNAL_SERVER_ERROR, Json(json!({ "error": e.to_string() })));
    }

    st.elastic.index(&ev).await;

    // Best-effort live fan-out (ignore if no subscribers).
    if let Ok(payload) = serde_json::to_string(&ev) {
        let _ = st.tx.send(payload);
    }

    (StatusCode::CREATED, Json(json!({ "accepted": true, "id": id })))
}

#[derive(Deserialize)]
pub struct LimitQuery {
    #[serde(default = "default_limit")]
    pub limit: i64,
}
fn default_limit() -> i64 {
    100
}

pub async fn events(
    State(st): State<Arc<AppState>>,
    Query(q): Query<LimitQuery>,
) -> impl IntoResponse {
    let rows = sqlx::query_as::<_, EventRow>(
        "SELECT id, ts, team, kind, technique, variant, session_id, target, outcome, severity, latency_ms, detail \
         FROM battle_events ORDER BY ts DESC LIMIT $1",
    )
    .bind(q.limit.clamp(1, 1000))
    .fetch_all(&st.pool)
    .await;

    match rows {
        Ok(r) => (StatusCode::OK, Json(json!({ "events": r }))),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, Json(json!({ "error": e.to_string() }))),
    }
}

pub async fn timeline(
    State(st): State<Arc<AppState>>,
    Query(q): Query<LimitQuery>,
) -> impl IntoResponse {
    let rows = sqlx::query_as::<_, SessionRow>(
        "SELECT session_id, technique, variant, target, severity, attack_ts, attack_outcome, \
         remediation_ts, remediation_outcome, mttr_seconds \
         FROM battle_sessions ORDER BY attack_ts DESC LIMIT $1",
    )
    .bind(q.limit.clamp(1, 1000))
    .fetch_all(&st.pool)
    .await;

    match rows {
        Ok(r) => (StatusCode::OK, Json(json!({ "sessions": r }))),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, Json(json!({ "error": e.to_string() }))),
    }
}

/// GET /api/latency : δ (detection delay) and κ (containment latency)
/// distributions from the stored battle events. Reports p50/p95/p99 — not just a
/// mean/MTTR — because the tail is what bounds the blast radius B(∞) ∝ (δ+κ).
///
///   δ ≈ latency_ms of caught attack events (detection compute at the boundary)
///   κ ≈ latency_ms of remediation events (session revoke + container kill)
pub async fn latency(State(st): State<Arc<AppState>>) -> impl IntoResponse {
    let delta = latency_dist(
        &st,
        "kind='attack' AND outcome IN ('blocked','detected')",
    )
    .await;
    let kappa = latency_dist(
        &st,
        "kind='remediation' AND outcome IN ('revoked','killed','restarted')",
    )
    .await;

    match (delta, kappa) {
        (Ok(d), Ok(k)) => (
            StatusCode::OK,
            Json(json!({
                "unit": "ms",
                "delta_detection": d,
                "kappa_containment": k,
                "note": "B(∞) ∝ (δ+κ); percentiles over stored latency_ms."
            })),
        ),
        (Err(e), _) | (_, Err(e)) => (
            StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({ "error": e.to_string() })),
        ),
    }
}

/// The quantile grid returned as a CDF so the dashboard/figures can draw a curve,
/// not just three points.
const CDF_QS: [f64; 9] = [0.05, 0.10, 0.25, 0.50, 0.75, 0.90, 0.95, 0.99, 1.0];

/// Percentile summary + CDF of latency_ms over the battle_events rows matching
/// `pred`. Returns p50/p95/p99 for the headline plus a `cdf` array of {q, ms}.
async fn latency_dist(st: &Arc<AppState>, pred: &str) -> Result<serde_json::Value, sqlx::Error> {
    let sql = format!(
        "SELECT \
           COUNT(*) AS n, \
           COALESCE(MIN(latency_ms),0) AS min, \
           COALESCE(MAX(latency_ms),0) AS max, \
           COALESCE(AVG(latency_ms),0) AS mean, \
           COALESCE(percentile_cont($1) WITHIN GROUP (ORDER BY latency_ms), ARRAY[]::double precision[]) AS qs \
         FROM battle_events WHERE {pred}"
    );
    let row = sqlx::query_as::<_, (i64, i64, i64, f64, Vec<f64>)>(&sql)
        .bind(&CDF_QS[..])
        .fetch_one(&st.pool)
        .await?;
    let (n, min, max, mean, qs) = row;
    let q = |i: usize| qs.get(i).copied().unwrap_or(0.0);
    let cdf: Vec<_> = CDF_QS
        .iter()
        .enumerate()
        .map(|(i, &p)| json!({ "q": p, "ms": q(i) }))
        .collect();
    Ok(json!({
        "count": n, "min_ms": min, "max_ms": max, "mean_ms": mean,
        "p50_ms": q(3), "p95_ms": q(6), "p99_ms": q(7),
        "cdf": cdf
    }))
}

/// Scoreboard computed entirely from Postgres (Elastic not required).
pub async fn stats(State(st): State<Arc<AppState>>) -> impl IntoResponse {
    let agg = sqlx::query_as::<_, (i64, i64, i64, i64, i64)>(
        "SELECT \
           COUNT(*) FILTER (WHERE kind='attack') AS attacks, \
           COUNT(*) FILTER (WHERE kind='attack' AND outcome='blocked') AS blocked, \
           COUNT(*) FILTER (WHERE kind='attack' AND outcome='allowed') AS landed, \
           COUNT(*) FILTER (WHERE kind='defense' AND outcome='detected') AS detected, \
           COUNT(*) FILTER (WHERE kind='remediation' AND outcome IN ('revoked','killed','restarted')) AS remediations \
         FROM battle_events",
    )
    .fetch_one(&st.pool)
    .await;

    let (attacks, blocked, landed, detected, remediations) = match agg {
        Ok(t) => t,
        Err(e) => return (StatusCode::INTERNAL_SERVER_ERROR, Json(json!({ "error": e.to_string() }))),
    };

    let mttr: Option<f64> = sqlx::query_scalar(
        "SELECT AVG(mttr_seconds)::double precision FROM battle_sessions \
         WHERE attack_outcome='allowed' AND remediation_ts IS NOT NULL",
    )
    .fetch_one(&st.pool)
    .await
    .ok()
    .flatten();

    let residual: i64 = sqlx::query_scalar(
        "SELECT COUNT(*) FROM battle_sessions \
         WHERE attack_outcome='allowed' AND remediation_ts IS NULL",
    )
    .fetch_one(&st.pool)
    .await
    .unwrap_or(0);

    // Per-technique breakdown.
    let by_technique = sqlx::query_as::<_, (String, i64, i64)>(
        "SELECT technique, \
           COUNT(*) FILTER (WHERE outcome='blocked') AS blocked, \
           COUNT(*) FILTER (WHERE outcome='allowed') AS landed \
         FROM battle_events WHERE kind='attack' GROUP BY technique ORDER BY technique",
    )
    .fetch_all(&st.pool)
    .await
    .unwrap_or_default();

    let technique_json: Vec<_> = by_technique
        .into_iter()
        .map(|(t, b, l)| json!({ "technique": t, "blocked": b, "landed": l }))
        .collect();

    let rate = |n: i64| if attacks > 0 { n as f64 / attacks as f64 } else { 0.0 };

    (
        StatusCode::OK,
        Json(json!({
            "attacks": attacks,
            "blocked": blocked,
            "landed": landed,
            "detected": detected,
            "remediations": remediations,
            "residual_risk": residual,
            "block_rate": rate(blocked),
            "landing_rate": rate(landed),
            "detection_rate": rate(detected),
            "mttr_seconds": mttr,
            "elastic_enabled": st.elastic.enabled(),
            "by_technique": technique_json,
        })),
    )
}

#[derive(Deserialize)]
pub struct SearchQuery {
    #[serde(default)]
    pub q: String,
}

pub async fn search(
    State(st): State<Arc<AppState>>,
    Query(sq): Query<SearchQuery>,
) -> impl IntoResponse {
    let q = if sq.q.is_empty() { "*".to_string() } else { sq.q };
    match st.elastic.search(&q).await {
        Ok(v) => (StatusCode::OK, Json(v)),
        Err(e) => (StatusCode::OK, Json(json!({ "error": e, "hint": "set ELASTIC_URL to enable search" }))),
    }
}

/// Live SSE feed of ingested events.
pub async fn stream(
    State(st): State<Arc<AppState>>,
) -> Sse<impl Stream<Item = Result<SseEvent, Infallible>>> {
    let rx = st.tx.subscribe();
    let stream = BroadcastStream::new(rx).filter_map(|msg| match msg {
        Ok(data) => Some(Ok(SseEvent::default().data(data))),
        Err(_) => None,
    });
    Sse::new(stream).keep_alive(axum::response::sse::KeepAlive::default())
}

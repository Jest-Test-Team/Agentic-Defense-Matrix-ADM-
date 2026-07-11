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
}

fn catalog() -> Vec<Svc> {
    use Probe::*;
    vec![
        Svc { name: "API Gateway", tech: "Go (Echo)", category: "Edge", detail: "Request interception, semantic analysis, routing", probe: Http("http://gateway:8080/v1/health"), optional: false },
        Svc { name: "Analysis Engine", tech: "Rust (axum)", category: "Edge", detail: "Battle event ingest, scoring, dashboard API", probe: SelfUp, optional: false },
        Svc { name: "SIEM Engine", tech: "Go", category: "Detection", detail: "Correlation engine + Redis streams", probe: Http("http://siem:9091/health"), optional: false },
        Svc { name: "Policy Engine", tech: "OPA", category: "Detection", detail: "Rego authorization decisions", probe: Http("http://policy:8181/health"), optional: false },
        Svc { name: "Planner Agent", tech: "Go + gRPC", category: "Agents", detail: "Task decomposition", probe: Http("http://planner:9081/health"), optional: false },
        Svc { name: "Executor Agent", tech: "Go + gRPC", category: "Agents", detail: "Tool execution (Docker API)", probe: Http("http://executor:9082/health"), optional: false },
        Svc { name: "Summarizer Agent", tech: "Go + gRPC", category: "Agents", detail: "Response summarization", probe: Http("http://summarizer:9083/health"), optional: false },
        Svc { name: "LLM Backend", tech: "Groq → X.AI", category: "Agents", detail: "Chat-completion inference (Groq primary, X.AI fallback)", probe: Llm, optional: false },
        Svc { name: "Sandboxing", tech: "Docker API", category: "Runtime", detail: "Ephemeral per-agent containers (via executor)", probe: Http("http://executor:9082/health"), optional: false },
        Svc { name: "Storage", tech: "Redis 7", category: "Runtime", detail: "SIEM hot path + session store", probe: Tcp("redis:6379"), optional: false },
        Svc { name: "Durable Log", tech: "Neon Postgres", category: "Data", detail: "Retained battle-event log", probe: Db, optional: false },
        Svc { name: "Search / Aggregation", tech: "Elasticsearch", category: "Data", detail: "Full-text + aggregation index", probe: Elastic, optional: true },
        Svc { name: "Observability", tech: "OpenTelemetry", category: "Ops", detail: "Traces, metrics, logs", probe: Http("http://otel-collector:13133/"), optional: true },
        Svc { name: "Endpoint Watchdog", tech: "Rust", category: "Ops", detail: "macOS ES / Windows WFP syscall interception", probe: Tcp("watchdog:0"), optional: true },
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
                Probe::Db => sqlx::query("SELECT 1").execute(&st.pool).await.is_ok(),
                Probe::Elastic => st.elastic.enabled(),
                Probe::Llm => probe_llm(&http).await,
            };
            let status = if up { "up" } else if s.optional { "disabled" } else { "down" };
            json!({ "name": s.name, "tech": s.tech, "category": s.category, "detail": s.detail, "status": status })
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

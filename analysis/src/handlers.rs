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

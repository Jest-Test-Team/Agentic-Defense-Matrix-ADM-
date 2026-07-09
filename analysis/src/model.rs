use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::PgPool;
use tokio::sync::broadcast;

use crate::elastic::ElasticClient;

/// Shared application state.
pub struct AppState {
    pub pool: PgPool,
    pub elastic: ElasticClient,
    /// Live fan-out channel for the SSE dashboard feed.
    pub tx: broadcast::Sender<String>,
}

/// Canonical battle event, mirror of the Go `battle.Event` type.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BattleEvent {
    #[serde(default = "uuid_v4")]
    pub id: String,
    #[serde(default = "now")]
    pub ts: DateTime<Utc>,
    pub team: String,
    pub kind: String,
    #[serde(default)]
    pub technique: String,
    #[serde(default)]
    pub variant: String,
    #[serde(default)]
    pub session_id: String,
    #[serde(default)]
    pub target: String,
    #[serde(default)]
    pub outcome: String,
    #[serde(default = "one")]
    pub severity: i32,
    #[serde(default)]
    pub latency_ms: i64,
    #[serde(default)]
    pub detail: String,
    #[serde(default)]
    pub labels: std::collections::HashMap<String, String>,
}

fn uuid_v4() -> String {
    uuid::Uuid::new_v4().to_string()
}
fn now() -> DateTime<Utc> {
    Utc::now()
}
fn one() -> i32 {
    1
}

/// A row from the `battle_events` table (for /api/events).
#[derive(Debug, Serialize, sqlx::FromRow)]
pub struct EventRow {
    pub id: uuid::Uuid,
    pub ts: DateTime<Utc>,
    pub team: String,
    pub kind: String,
    pub technique: String,
    pub variant: String,
    pub session_id: String,
    pub target: String,
    pub outcome: String,
    pub severity: i32,
    pub latency_ms: i64,
    pub detail: String,
}

/// A correlated session row from the `battle_sessions` view (for /api/timeline).
#[derive(Debug, Serialize, sqlx::FromRow)]
pub struct SessionRow {
    pub session_id: String,
    pub technique: String,
    pub variant: String,
    pub target: String,
    pub severity: i32,
    pub attack_ts: DateTime<Utc>,
    pub attack_outcome: String,
    pub remediation_ts: Option<DateTime<Utc>>,
    pub remediation_outcome: Option<String>,
    pub mttr_seconds: Option<f64>,
}

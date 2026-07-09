//! ADM battle analysis engine.
//!
//! - `POST /ingest`            accept a BattleEvent -> durable Postgres row +
//!                             (optional) Elasticsearch index + live SSE fan-out
//! - `GET  /api/stats`         scoreboard (block/detection/landing rate, MTTR)
//! - `GET  /api/timeline`      recent correlated sessions
//! - `GET  /api/events`        recent raw events
//! - `GET  /api/stream`        Server-Sent-Events live feed
//! - `GET  /`                  static dashboard (db/be/fe in one binary)
//!
//! Postgres (via DATABASE_URL) is the authoritative durable log and computes all
//! stats on its own. Elasticsearch (via optional ELASTIC_URL) is an extra search
//! index; when it is unset the engine runs fully on Postgres, so it stays free.

mod elastic;
mod handlers;
mod model;

use std::sync::Arc;
use std::time::Duration;

use axum::routing::{get, post};
use axum::Router;
use sqlx::postgres::PgPoolOptions;
use tokio::sync::broadcast;
use tower_http::cors::CorsLayer;

use crate::model::AppState;

const INDEX_HTML: &str = include_str!("../web/index.html");

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(std::env::var("RUST_LOG").unwrap_or_else(|_| "info".into()))
        .init();

    let database_url = std::env::var("DATABASE_URL")
        .expect("DATABASE_URL is required (e.g. a Neon postgres connection string)");
    let elastic_url = std::env::var("ELASTIC_URL").ok().filter(|s| !s.is_empty());
    let bind = std::env::var("ADM_ANALYSIS_BIND").unwrap_or_else(|_| "0.0.0.0:8090".into());

    let pool = PgPoolOptions::new()
        .max_connections(5)
        .acquire_timeout(Duration::from_secs(10))
        .connect(&database_url)
        .await?;

    // Apply migrations (idempotent) so a fresh managed DB is ready on first boot.
    sqlx::migrate!("./migrations").run(&pool).await?;

    let elastic = elastic::ElasticClient::new(elastic_url.clone());
    elastic.ensure_index().await;

    let (tx, _rx) = broadcast::channel(1024);
    let state = Arc::new(AppState {
        pool,
        elastic,
        tx,
    });

    let app = Router::new()
        .route("/", get(|| async { axum::response::Html(INDEX_HTML) }))
        .route("/health", get(handlers::health))
        .route("/ready", get(handlers::ready))
        .route("/ingest", post(handlers::ingest))
        .route("/api/events", get(handlers::events))
        .route("/api/timeline", get(handlers::timeline))
        .route("/api/stats", get(handlers::stats))
        .route("/api/search", get(handlers::search))
        .route("/api/stream", get(handlers::stream))
        .layer(CorsLayer::permissive())
        .with_state(state);

    tracing::info!("adm-analysis listening on {bind} (elastic={})", elastic_url.is_some());
    let listener = tokio::net::TcpListener::bind(&bind).await?;
    axum::serve(listener, app).await?;
    Ok(())
}

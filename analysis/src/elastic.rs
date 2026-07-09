//! Optional Elasticsearch sink. When `ELASTIC_URL` is unset the client is inert
//! and every method is a no-op, so the engine runs fully on Postgres and stays
//! free. When set, events are additionally indexed for search/aggregation.

use crate::model::BattleEvent;

const INDEX: &str = "adm-battle-events";

#[derive(Clone)]
pub struct ElasticClient {
    base: Option<String>,
    http: reqwest::Client,
}

impl ElasticClient {
    pub fn new(base: Option<String>) -> Self {
        Self {
            base: base.map(|b| b.trim_end_matches('/').to_string()),
            http: reqwest::Client::new(),
        }
    }

    pub fn enabled(&self) -> bool {
        self.base.is_some()
    }

    /// Create the index if it does not exist. Best-effort; failures are logged.
    pub async fn ensure_index(&self) {
        let Some(base) = &self.base else { return };
        let url = format!("{base}/{INDEX}");
        // PUT is idempotent-ish: a 400 "already exists" is fine.
        match self.http.put(&url).json(&serde_json::json!({
            "mappings": {
                "properties": {
                    "ts":        { "type": "date" },
                    "team":      { "type": "keyword" },
                    "kind":      { "type": "keyword" },
                    "technique": { "type": "keyword" },
                    "outcome":   { "type": "keyword" },
                    "target":    { "type": "keyword" },
                    "session_id":{ "type": "keyword" },
                    "severity":  { "type": "integer" },
                    "detail":    { "type": "text" }
                }
            }
        })).send().await {
            Ok(resp) => tracing::info!("elastic ensure_index: {}", resp.status()),
            Err(e) => tracing::warn!("elastic ensure_index failed: {e}"),
        }
    }

    /// Index a single event. No-op when disabled.
    pub async fn index(&self, ev: &BattleEvent) {
        let Some(base) = &self.base else { return };
        let url = format!("{base}/{INDEX}/_doc/{}", ev.id);
        if let Err(e) = self.http.put(&url).json(ev).send().await {
            tracing::warn!("elastic index failed: {e}");
        }
    }

    /// Passthrough search. Returns the raw Elasticsearch response body, or an
    /// error message when disabled.
    pub async fn search(&self, q: &str) -> Result<serde_json::Value, String> {
        let Some(base) = &self.base else {
            return Err("elastic disabled (ELASTIC_URL unset)".into());
        };
        let url = format!("{base}/{INDEX}/_search");
        let body = serde_json::json!({
            "size": 50,
            "query": { "query_string": { "query": q } },
            "sort": [{ "ts": { "order": "desc" } }]
        });
        self.http
            .post(&url)
            .json(&body)
            .send()
            .await
            .map_err(|e| e.to_string())?
            .json::<serde_json::Value>()
            .await
            .map_err(|e| e.to_string())
    }
}

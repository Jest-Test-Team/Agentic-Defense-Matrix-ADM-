-- Durable battle-event log. This table is the authoritative retained log of
-- every attack, defense decision and remediation. Nothing is deleted.
CREATE TABLE IF NOT EXISTS battle_events (
    id          UUID PRIMARY KEY,
    ts          TIMESTAMPTZ NOT NULL,
    team        TEXT        NOT NULL,
    kind        TEXT        NOT NULL,
    technique   TEXT        NOT NULL DEFAULT '',
    variant     TEXT        NOT NULL DEFAULT '',
    session_id  TEXT        NOT NULL DEFAULT '',
    target      TEXT        NOT NULL DEFAULT '',
    outcome     TEXT        NOT NULL DEFAULT '',
    severity    INT         NOT NULL DEFAULT 1,
    latency_ms  BIGINT      NOT NULL DEFAULT 0,
    detail      TEXT        NOT NULL DEFAULT '',
    labels      JSONB       NOT NULL DEFAULT '{}'::jsonb,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_battle_events_ts        ON battle_events (ts DESC);
CREATE INDEX IF NOT EXISTS idx_battle_events_session   ON battle_events (session_id);
CREATE INDEX IF NOT EXISTS idx_battle_events_team_kind ON battle_events (team, kind);
CREATE INDEX IF NOT EXISTS idx_battle_events_technique ON battle_events (technique);

-- Correlated view: one row per attack session, joined to the defense/remediation
-- it triggered. Powers the timeline and MTTR calculation.
CREATE OR REPLACE VIEW battle_sessions AS
SELECT
    a.session_id,
    a.technique,
    a.variant,
    a.target,
    a.severity,
    a.ts                              AS attack_ts,
    a.outcome                         AS attack_outcome,
    r.ts                              AS remediation_ts,
    r.outcome                         AS remediation_outcome,
    EXTRACT(EPOCH FROM (r.ts - a.ts)) AS mttr_seconds
FROM battle_events a
LEFT JOIN LATERAL (
    SELECT ts, outcome
    FROM battle_events r
    WHERE r.session_id = a.session_id
      AND r.kind = 'remediation'
    ORDER BY r.ts DESC
    LIMIT 1
) r ON TRUE
WHERE a.kind = 'attack';

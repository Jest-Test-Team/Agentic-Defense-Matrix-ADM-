# ADM Battle Orchestration — Red vs Blue vs Green

**Status:** implemented (vertical slice), free/local-first, cloud-portable
**Audience:** operators running an end-to-end adversarial exercise against the ADM stack.

---

## 1. Context & goal

The base ADM stack (README) is the **blue team + target system**: the gateway,
agent services (planner/executor/summarizer), SIEM engine, OPA policy engine and
Rust watchdog. It is deployed on OCI (see `docs/instruction.md`).

This document adds the missing pieces so ADM runs as a **continuous, observable
adversarial exercise**:

| Team | Role | Status before | This work |
|------|------|---------------|-----------|
| 🔴 **Red** | Attack the target continuously | `tests/redteam/` Go **tests** only | New long-running **attacker service** + corpus generator expanding 30 base techniques → up to **10 000** variants |
| 🔵 **Blue** | Detect & block (gateway semantics, SIEM, policy, watchdog) | Deployed | Reused as-is; every decision emitted as a battle event |
| 🟢 **Green** | Remediate damage from attacks that land | Concept in README only | New **remediation service**: revoke session → kill/restart agent container → log |
| 📊 **Analysis** | Durable log + scoring + dashboard (db/be/fe) | Did not exist | New **Rust + Postgres + Elasticsearch** engine and web dashboard |

**Hard requirement:** *all events must retain logs.* Every attack, defense
decision and remediation is written to a durable Postgres table **and** indexed
in Elasticsearch for search/aggregation. Nothing is fire-and-forget.

---

## 2. Where it runs (decision: free cloud — OCI micro + free managed data)

Everything is **free** and **cloud-hosted**. The heavy stateful services are
offloaded to free-forever managed clouds so nothing large runs on the tiny VM:

| Piece | Where | Free tier |
|-------|-------|-----------|
| Blue stack + red + green + analysis binaries | OCI Always Free VM (`VM.Standard.E2.1.Micro`, 1 GB) | free forever |
| **Postgres** (durable battle log) | **Neon** managed Postgres | free forever (0.5 GB) |
| **Search/aggregation** (optional) | **Bonsai** Elasticsearch sandbox *(or any Elastic-compatible URL)* | free sandbox; **env-gated — degrades to Postgres if unset** |
| LLM | Ollama on the VM, tiny model (`qwen2.5:0.5b`) | free |

Rationale:

- Elasticsearch has **no** free-*forever* managed tier (Elastic Cloud is a 14-day
  trial), and a self-hosted Elastic/OpenSearch node wants 1–2 GB — it will not fit
  on the 1 GB micro. So search is offloaded to a managed sandbox **and made
  optional**: the analysis engine computes every scoreboard number from Postgres
  alone, and *additionally* indexes to Elasticsearch only when `ELASTIC_URL` is
  set. "All free" holds with or without it.
- Neon gives real managed Postgres, free forever, reachable over TLS from the VM —
  so the *durable retained log* lives off-box and survives instance recreation.
- The red/green/analysis binaries are tiny (Go ~20 MB RSS, Rust ~30 MB RSS); they
  add little to the micro because the DB/search RAM is elsewhere.

### Required secrets / env (set on the VM or as repo variables)

| Var | Meaning | Example |
|-----|---------|---------|
| `DATABASE_URL` | Neon Postgres connection string | `postgres://user:pw@ep-x.neon.tech/adm?sslmode=require` |
| `ELASTIC_URL` | *(optional)* Elastic-compatible endpoint | `https://user:pw@xxx.bonsaisearch.net` |
| `ADM_GATEWAY_URL` | target gateway | `http://gateway:8080` (in-compose) |

**A1 upgrade path:** the same stack self-hosts Postgres + OpenSearch on-box once
you move to an A1 shape (2–4 OCPU / 12–24 GB). Terraform now retries across
availability domains on "Out of host capacity" (§8) so an A1 launch lands as soon
as capacity frees — at which point `DATABASE_URL`/`ELASTIC_URL` can point at
on-box containers instead of managed clouds.

---

## 3. Architecture

```
                    ┌──────────────────────────────────────────────┐
                    │              📊 ANALYSIS (Rust/axum)          │
                    │  POST /ingest  ── durable ──►  Postgres        │
                    │                └─ index ────►  Elasticsearch   │
                    │  GET /api/stats /api/timeline /api/stream(SSE) │
                    │  GET /  ── static dashboard (db/be/fe) ───────►│
                    └───────▲───────────────▲───────────────▲────────┘
   battle events (JSON)     │               │               │
        ┌───────────────────┘               │               └───────────────┐
        │                                   │                               │
 ┌──────┴───────┐   attacks (HTTP)   ┌──────┴───────┐   alerts (Redis)  ┌───┴──────────┐
 │  🔴 RED TEAM │ ─────────────────► │  🔵 BLUE /    │ ────────────────► │ 🟢 GREEN TEAM│
 │  attacker    │  /v1/chat/...      │  TARGET       │  siem alertChan   │ remediation  │
 │  service     │  /v1/tools/execute │  gateway+SIEM │                   │ service      │
 │  (Go)        │ ◄───────────────── │  +policy+     │ ◄──────────────── │ (Go)         │
 └──────────────┘  blocked/allowed   │  watchdog     │  revoke / kill    └──────────────┘
                                     └───────────────┘
```

### Event bus
All three teams emit a canonical **battle event** (see §4) two ways:
1. **`POST /ingest`** to the analysis engine → durable Postgres + Elasticsearch.
2. **Redis stream `adm:battle`** → realtime fan-out (green team subscribes; the
   analysis SSE endpoint tails it for the live dashboard).

The existing blue-team SIEM keeps its own `siem:events` list and `alertChan`; the
green team consumes SIEM alerts, and the gateway/SIEM also emit battle events so
the dashboard shows blue-team decisions inline with red/green actions.

---

## 4. Canonical battle event schema

Shared Go type `pkg/battle.Event` (mirrored by the Rust `BattleEvent`):

```jsonc
{
  "id":         "uuid",
  "ts":         "2026-07-09T00:00:00Z",   // RFC3339
  "team":       "red|blue|green",
  "kind":       "attack|defense|remediation",
  "technique":  "RT-004",                  // base technique id
  "variant":    "RT-004#a1b2 (base64+multilang)",
  "session_id": "sess-...",                // ties attack→defense→remediation
  "target":     "gateway|executor|...",
  "outcome":    "blocked|allowed|detected|revoked|killed|error",
  "severity":   1,                          // 1..5
  "latency_ms": 12,
  "detail":     "free text",
  "labels":     { "mutation": "base64", "lang": "zh" }
}
```

`session_id` is the **join key**: the red team stamps it on each attack via the
`X-Session-ID` header; the blue team records it on detection; the green team keys
remediation off it. The analysis engine correlates all three into one timeline
row per session.

---

## 5. 🔴 Red team — continuous attacker + corpus generator

### Corpus generator (`pkg/redteam`)
- **Base techniques (30):** the RT-001…RT-030 catalog from the README, each with
  a name, MITRE-ATLAS/OWASP tag, target endpoint, and one or more seed payloads
  (lifted from the existing `tests/redteam` harness).
- **Mutation strategies:** `identity`, `base64`, `hex`, `url-encode`,
  `unicode-homoglyph`, `zero-width-inject`, `case-flip`, `multilang` (en/zh/es/ar/ru),
  `nesting`, `concat-split`, `whitespace-pad`.
- **Expansion:** deterministic (seeded) cartesian expansion of
  `technique × seed × mutation × paraphrase` produces up to **`ADM_CORPUS_SIZE`
  (default 10 000)** concrete `AttackVariant`s. Deterministic seed = reproducible
  campaigns.

### Attacker service (`cmd/redteam_agent`)
- Long-running loop; every `ADM_ATTACK_INTERVAL` (default 500 ms) it pulls the
  next variant and fires it at the gateway (`/v1/chat/completions` or
  `/v1/tools/execute`), stamping a fresh `session_id`.
- Classifies the response: HTTP 4xx / empty / policy-blocked ⇒ `blocked`;
  2xx with content ⇒ `allowed` (potential landing).
- Emits a `team=red, kind=attack` battle event with outcome, latency, technique,
  variant and labels.
- Config: `ADM_GATEWAY_URL`, `ADM_ANALYSIS_URL`, `ADM_REDIS_URL`,
  `ADM_CORPUS_SIZE`, `ADM_ATTACK_INTERVAL`, `ADM_ATTACK_CONCURRENCY`, `ADM_MODEL`.

---

## 6. 🟢 Green team — remediation service (`cmd/greenteam_agent`)

Subscribes to blue-team **SIEM alerts** (`GET /api/v1/alerts` poll + Redis
`adm:battle` stream for `kind=defense outcome=detected`). On an alert it runs the
README response chain — **for real**:

1. **Revoke session** → `POST {gateway}/v1/admin/revoke/{session_id}`.
2. **Contain the agent** → Docker API kill+restart of the affected agent
   container (executor/planner/summarizer), guarded to only touch containers
   labelled `adm.role=agent` so it can never harm infra.
3. **Log** → emit `team=green, kind=remediation` events for each action
   (`revoked`, `killed`, `restarted`), keyed by `session_id` for correlation.

A `ADM_GREEN_DRY_RUN=true` switch logs intended actions without calling
Docker/gateway (useful on hosts without the Docker socket mounted).

---

## 7. 📊 Analysis engine + service (Rust + Postgres + Elasticsearch)

Crate `analysis/` (axum). Single binary = backend + static frontend.

### Backend (be)
- `POST /ingest` — accept a `BattleEvent`; write row to Postgres `battle_events`
  and index the same doc into Elasticsearch `adm-battle-events`. Also re-publishes
  to the SSE hub.
- `GET /api/timeline?limit=` — recent correlated sessions (attack + defense +
  remediation joined on `session_id`) from Postgres.
- `GET /api/stats` — scoreboard: attacks sent, blocked, landed, detected,
  remediated; **block rate**, **detection rate**, **mean time to remediate
  (MTTR)**; per-technique and per-mutation breakdown (Elasticsearch aggregations,
  Postgres fallback).
- `GET /api/search?q=` — full-text/agg passthrough to Elasticsearch.
- `GET /api/stream` — Server-Sent-Events live feed.
- `GET /health`, `GET /ready`.

### Database (db)
- **Postgres (Neon, managed cloud)** = durable, authoritative log
  (`battle_events`, plus a `sessions` view correlating the three teams). This is
  the *retained log* of record and computes **all** scoreboard stats on its own.
  Migrations auto-applied from `analysis/migrations/` on startup. Connection via
  `DATABASE_URL`.
- **Elasticsearch (managed sandbox, optional)** = extra search + aggregation
  index, enabled only when `ELASTIC_URL` is set. Rebuildable from Postgres; the
  dashboard never *depends* on it, so the whole thing stays free even without it.

### Frontend (fe)
`analysis/web/index.html` — a single static dashboard served by the backend:
live scoreboard tiles (block rate, detection rate, MTTR), a technique heatmap, and
a streaming battle timeline (colored by team) fed by `/api/stream`. No JS build
step (vanilla + SSE), so it runs anywhere.

---

## 8. Orchestration

- **`deploy/Dockerfile.battle`** — Go builder producing `adm-redteam` and
  `adm-greenteam`.
- **`deploy/Dockerfile.analysis`** — Rust builder producing the analysis binary +
  bundling `web/`.
- **`deploy/docker-compose.battle.yml`** — overlay that adds: `postgres`,
  `elasticsearch`, `analysis`, `redteam`, `greenteam`. Layer it on the base stack:

```bash
make battle-up      # base docker-compose.yml + overlay, detached
make battle-logs    # follow red/green/analysis logs
make battle-down    # stop the exercise
open http://localhost:8090   # dashboard
```

- **Makefile targets:** `battle-up`, `battle-down`, `battle-logs`,
  `battle-build`, and `build-battle` (compiles the two Go services locally).

### Startup ordering & health
`analysis` waits for Postgres + Elasticsearch healthchecks; `redteam`/`greenteam`
wait for the gateway healthcheck. The red team ramps only after the gateway is
`healthy` so early requests aren't counted as false "landings".

---

## 8.5 Verifying connections & running status

You don't test Neon / Bonsai / OCI separately — the running app **self-reports
all three** through its health/stats endpoints. After deploy, run these against
the instance's public IP:

| Connection | How it's verified | "Good" looks like |
|---|---|---|
| **OCI app running** | `curl http://<ip>:8080/v1/health` and `curl http://<ip>:8090/health` | `{"status":"ok"}` on both |
| **OCI → Neon (Postgres)** | `curl http://<ip>:8090/ready` | `{"ready":true}` — analysis only reports ready once it has connected to Neon and applied migrations |
| **OCI → Bonsai (Elastic)** | `curl http://<ip>:8090/api/stats` → `elastic_enabled` | `"elastic_enabled": true` = Bonsai wired; `false` = Postgres-only (still fully functional) |
| **Red→Blue→Green loop** | `curl http://<ip>:8090/api/stats` counters | `attacks` rising; `blocked` / `landed` / `remediations` moving |

One-shot check:

```bash
IP=<public-ip>
echo "gateway:";   curl -s http://$IP:8080/v1/health
echo "analysis:";  curl -s http://$IP:8090/health
echo "neon:";      curl -s http://$IP:8090/ready
echo "bonsai+score:"; curl -s http://$IP:8090/api/stats | jq '{elastic_enabled, attacks, blocked, landed, remediations, mttr_seconds}'
```

On the box itself, container status is `docker compose -f docker-compose.yml -f deploy/docker-compose.battle.yml ps` (or `sudo -u adm /opt/adm/repo/deploy/scripts/status.sh`).

### DNS / static IP

The instance's public IP changes on every recreation, so before any DNS: attach
a **reserved public IP** (OCI Always Free includes one) so the address is stable
across redeploys. DNS then maps a name → that IP only; it does **not** remove the
`:8090` port — for a clean `https://name/` you'd add a reverse proxy (e.g. Caddy)
on 443 in front of the dashboard. Recommended order: get it running → reserve a
static IP → (optional) DNS + reverse proxy.

## 9. Scoring model (how "who's winning" is computed)

Per campaign, from the durable log:

- **Block rate** = `blocked / attacks` (blue at the boundary).
- **Detection rate** = `detected / attacks` (blue via SIEM, even if it landed).
- **Landing rate** = `allowed / attacks` (red success at L7).
- **Containment / MTTR** = mean `remediation.ts − attack.ts` over landed-then-
  remediated sessions (green effectiveness).
- **Residual risk** = landed **and** never remediated.

The dashboard renders these live; the same numbers are queryable from
`/api/stats` for CI gating.

---

## 10. Security & scope guardrails

- The red team only targets `ADM_GATEWAY_URL` — a deployment you own. It sends
  real attack payloads; **never point it at third-party systems.**
- Green team Docker actions are restricted to `adm.role=agent`-labelled
  containers; infra (db, redis, gateway, ollama) is never touched.
- On the internet-exposed OCI deployment, keep this exercise **local** or firewall
  the analysis/Postgres/Elastic ports — they are not authN-protected in this slice.

---

## 11. Roadmap beyond this slice

- SPIRE-issued mTLS between teams and the analysis ingest.
- Replace poll-based alert consumption with a real Redis consumer group.
- Kibana dashboards on the same Elasticsearch index for deep forensics.
- Adaptive red team (LLM-guided mutation toward payloads that landed).
- Move Postgres to OCI Autonomous DB (Always Free) when deploying to cloud.
```

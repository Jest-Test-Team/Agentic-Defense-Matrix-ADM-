# Live Deployment — Infrastructure & Services

This document describes the **production deployment** of the Agentic Defense
Matrix (ADM) red/blue/green exercise as it actually runs — every piece of
infrastructure, every service, and how a request flows end to end. It complements
`docs/battle-orchestration.md` (the exercise design) and `docs/instruction.md`
(operating the box).

Everything runs on **free tiers**.

---

## 1. Topology (end to end)

```
                        ┌──────────────────────────────────────────────┐
   viewer's browser ───▶│  GitHub Pages  (static Next.js dashboard)     │  HTTPS
                        │  jest-test-team.github.io/…-ADM/              │
                        └───────────────┬──────────────────────────────┘
                                        │  fetch /api/stats,/timeline,/stream (SSE)
                                        ▼  HTTPS
                        ┌──────────────────────────────────────────────┐
                        │  Cloudflare DNS (DNS-only)                    │
                        │  api.dennisleehappy.org  →  <box IP>          │
                        └───────────────┬──────────────────────────────┘
                                        ▼  HTTPS (Let's Encrypt)
 ┌───────────────────────────────────────────────────────────────────────────────┐
 │  OCI Always-Free VM  (VM.Standard.E2.1.Micro, 1 GB, Oracle Linux 8)            │
 │                                                                                │
 │   Caddy :80/:443  ── /v1/* ─▶ gateway:8080                                     │
 │   (auto-HTTPS)     └─ else ──▶ analysis:8090                                   │
 │                                                                                │
 │   ┌───────────── Docker (adm-internal / adm-public bridges) ───────────────┐  │
 │   │  🔴 redteam ─attacks▶ 🔵 gateway:8080 ─┬─ semantic + policy(OPA:8181)    │  │
 │   │                                        ├─ siem:9091  ─┐                  │  │
 │   │  🟢 greenteam ◀─remediates────────────┘              │                  │  │
 │   │       │ revoke session / restart agent               │ events           │  │
 │   │       ▼                                               ▼                  │  │
 │   │  planner · executor · summarizer (agents)        redis:6379 (sessions)   │  │
 │   │                                                                          │  │
 │   │  📊 analysis:8090  ──durable──▶ Neon Postgres   ──index──▶ Bonsai Elastic │  │
 │   │        └─ serves /api/* + SSE to the dashboard                           │  │
 │   └──────────────────────────────────────────────────────────────────────────┘│
 │        gateway/agents ──LLM──▶ Groq (hosted, OpenAI-compatible)                │
 └───────────────────────────────────────────────────────────────────────────────┘
        │ prebuilt images                    ▲ terraform apply
        ▼ docker pull                        │
   GHCR (ghcr.io/jest-test-team/adm-*)   GitHub Actions (build · deploy · pages)
```

---

## 2. Infrastructure (where things run)

| Layer | Provider / Tech | Role | Free tier |
|---|---|---|---|
| **Compute** | OCI Always-Free `VM.Standard.E2.1.Micro` (1 OCPU / 1 GB, Oracle Linux 8), region ap-tokyo-1 | Hosts the whole battle stack in Docker | free forever |
| **Container runtime** | Docker CE + Compose | Runs all services | free |
| **HTTPS front** | Caddy 2 | Auto Let's Encrypt cert; reverse-proxy `/v1/*`→gateway, else→analysis; CORS pass-through | free |
| **DNS** | Cloudflare (`dennisleehappy.org`, **DNS-only / grey-cloud**) | `api.dennisleehappy.org` → box IP | free |
| **Durable database** | **Neon** managed Postgres | Authoritative, retained battle-event log + correlated `sessions` view | free (0.5 GB) |
| **Search / aggregation** | **Bonsai** Elasticsearch | Full-text + aggregation index (optional; Postgres is the source of truth) | free sandbox |
| **LLM** | **Groq** hosted (OpenAI-compatible) | Chat-completion backend for the gateway + agents (replaces on-box Ollama so it fits 1 GB) | free tier |
| **Image registry** | **GHCR** `ghcr.io/jest-test-team/adm-*` (public) | Prebuilt images the 1 GB box pulls instead of compiling | free (public) |
| **Dashboard hosting** | **GitHub Pages** | Serves the static Next.js dashboard over HTTPS | free |
| **CI/CD + IaC** | **GitHub Actions** + **Terraform** | Build images, deploy to OCI, publish the dashboard | free |

> Why the split: the 1 GB micro can't compile images or run Ollama + Elasticsearch,
> so heavy/stateful pieces (Postgres, Elasticsearch, the LLM, image builds) are
> offloaded to free managed clouds, leaving the micro to run ~13 small containers.

---

## 3. Services (what runs in Docker)

| Service | Image | Port | Team | Role |
|---|---|---|---|---|
| **caddy** | `caddy:2-alpine` | 80/443 | infra | Auto-HTTPS reverse proxy in front of the APIs |
| **gateway** | `adm-gateway` | 8080 (9090 gRPC) | 🔵 blue / target | L7 API front door: semantic analysis, policy check, session mgmt, admin (revoke/metrics). The thing the red team attacks. |
| **siem** | `adm-siem` | 9091 | 🔵 blue | Correlation engine; ring-buffer ingest + Redis persistence |
| **policy** | `adm-policy` (OPA) | 8181 | 🔵 blue | Open Policy Agent — Rego authorization decisions |
| **planner** | `adm-planner` | 9081 gRPC | agent | Task-decomposition agent (uses the LLM) |
| **executor** | `adm-executor` | 9082 gRPC | agent | Tool-execution agent (Docker API); green team's restart target |
| **summarizer** | `adm-summarizer` | 9083 gRPC | agent | Response-summarization agent (uses the LLM) |
| **redis** | `redis:7-alpine` | 6379 | infra | Session + event store; realtime `adm:battle` stream |
| **analysis** | `adm-analysis` (Rust/axum) | 8090 | 📊 | Ingests battle events → Neon (durable) + Elastic; serves `/api/stats /timeline /events /stream` + the bundled HTML dashboard |
| **redteam** | `adm-redteam` | — | 🔴 red | Continuous attacker: expands ~30 techniques → 10k variants, fires them at the gateway, logs outcomes |
| **greenteam** | `adm-greenteam` | — | 🟢 green | Watches for landed attacks → revokes session (gateway admin) + restarts the affected agent (Docker), logs remediation |
| otel-collector | `adm-*`/upstream | 4317/4318 | obs | Telemetry (disabled on the micro; `ADM_BATTLE_FULL=true` to enable) |
| control-plane | `adm-control-plane` | 9092 | infra | Auto-update checks (disabled on the micro) |
| watchdog | `adm-watchdog` (Rust) | host | 🔵 blue | OS-level endpoint monitor (disabled on the micro) |

On the 1 GB micro, `battle-up.sh` runs the essential set (caddy + gateway + siem +
policy + 3 agents + redis + analysis + redteam + greenteam) and drops
otel/control-plane/watchdog to fit memory.

---

## 4. Request / data flow

1. **Viewer** opens the GitHub Pages dashboard (HTTPS). It polls `/api/stats` &
   `/api/timeline` every few seconds and opens an SSE stream `/api/stream`, all
   against `https://api.dennisleehappy.org`.
2. **Cloudflare DNS** resolves that to the box; **Caddy** terminates TLS and routes
   the request (`/v1/*` → gateway, else → analysis).
3. **Red team** continuously POSTs attacks to the **gateway** (`/v1/chat/completions`,
   `/v1/tools/execute`). The gateway runs semantic analysis + **OPA policy** and
   either blocks or forwards; chat completions go to **Groq**.
4. Each attack/defense/remediation is emitted as a **battle event** to the
   **analysis** engine, which writes it to **Neon Postgres** (durable) and indexes
   it in **Bonsai Elasticsearch**, and re-publishes it on the SSE stream.
5. **Green team** watches the `adm:battle` Redis stream; when an attack lands it
   calls the gateway's admin revoke and restarts the affected agent container, then
   logs the remediation.
6. The dashboard renders the scoreboard, per-technique breakdown, live feed, and
   correlated sessions from the analysis engine's responses.

---

## 5. Deployment pipeline (CI/CD)

| Workflow | Trigger | What it does |
|---|---|---|
| `images.yml` | push to `cmd/**`, `pkg/**`, `analysis/**`, Dockerfiles | Builds all 11 service images and pushes to **GHCR** (public) |
| `terraform-oci.yml` | manual dispatch (`apply`) | Provisions the OCI VM; cloud-init installs Docker, pulls images, runs `battle-up.sh`. `replace_instance=true` recreates the box to pick up new provisioning |
| `pages.yml` | push to `dashboard/**` | Builds the static Next.js dashboard and deploys it to **GitHub Pages** |
| `ci.yml` | push / PR | Go + Rust build/test/lint; builds the analysis engine |
| `red_team_fuzz.yml` | push / PR / weekly | Runs the RT attack suite + proves the corpus scales to 10 000 |
| `release.yml` | tag `v*` | Cross-platform binaries + versioned Docker images |

The box provisions itself from `deploy/terraform/cloud-init.yaml`, which writes a
`provision.sh` script that: fixes the VNIC MTU, installs Docker CE, disables
firewalld (+ restarts Docker), makes the reserved `adm` user usable, writes
`/opt/adm/battle.env` from Terraform variables, and runs `battle-up.sh`. See
`docs/battle-orchestration.md §8.6` for the OL8/OCI gotchas each of those steps
works around.

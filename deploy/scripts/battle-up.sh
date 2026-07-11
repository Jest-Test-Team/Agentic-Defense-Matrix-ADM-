#!/usr/bin/env bash
# One-command launcher for the ADM battle exercise (red vs blue vs green +
# analysis) on the OCI box.
#
# Requires DATABASE_URL (a free Neon Postgres connection string). Optional:
#   ELASTIC_URL        - Elastic-compatible endpoint (e.g. Bonsai); degrades to
#                        Postgres-only when unset.
#   ADM_OLLAMA_MODEL   - tiny model to pull for the 1 GB micro (default below).
#
# Values may be exported in the environment or placed in /opt/adm/battle.env
# (cloud-init writes that file when a DATABASE_URL is provided via Terraform).
#
# Usage:
#   DATABASE_URL='postgres://user:pw@ep-x.neon.tech/adm?sslmode=require' \
#     /opt/adm/repo/deploy/scripts/battle-up.sh
set -euo pipefail

REPO="${ADM_REPO:-/opt/adm/repo}"
ENV_FILE="${ADM_BATTLE_ENV:-/opt/adm/battle.env}"
MODEL="${ADM_OLLAMA_MODEL:-qwen2.5:0.5b}"

# Load persisted env (if cloud-init or the operator wrote it).
if [[ -f "$ENV_FILE" ]]; then
  # shellcheck disable=SC1090
  set -a; source "$ENV_FILE"; set +a
fi

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "ERROR: DATABASE_URL is not set." >&2
  echo "Get a free Neon connection string (see docs/battle-setup.local.md) and either" >&2
  echo "  export DATABASE_URL=... before running this, or write it to $ENV_FILE" >&2
  exit 1
fi
export DATABASE_URL
export ELASTIC_URL="${ELASTIC_URL:-}"

cd "$REPO"
BATTLE=(docker compose -f docker-compose.yml -f deploy/docker-compose.battle.yml)

# Prebuilt images live in GHCR so the 1 GB micro never compiles Go/Rust on-box.
# If the packages are private, provide GHCR_USER/GHCR_TOKEN (in battle.env) for
# an authenticated pull; public packages need no login.
if [[ -n "${GHCR_TOKEN:-}" && -n "${GHCR_USER:-}" ]]; then
  echo "[battle] logging in to GHCR as $GHCR_USER..."
  echo "$GHCR_TOKEN" | docker login ghcr.io -u "$GHCR_USER" --password-stdin || \
    echo "[battle] WARNING: GHCR login failed; assuming public images"
fi

# Essential services only. On the 1 GB micro the observability/update/endpoint
# extras (otel-collector, control-plane, watchdog) are dropped to fit memory;
# set ADM_BATTLE_FULL=true to run everything. Ollama is only started when the
# LLM mode is not "openai" (i.e. we're NOT using a hosted API like Groq) — on the
# micro, hosted-LLM mode is what makes the stack fit at all.
LLM_MODE="${ADM_LLM_MODE:-ollama}"
BASE_SVCS=(redis gateway siem policy planner executor summarizer)
OVERLAY_SVCS=(analysis redteam greenteam)
if [[ "$LLM_MODE" != "openai" ]]; then
  BASE_SVCS=(redis ollama gateway siem policy planner executor summarizer)
fi
if [[ "${ADM_BATTLE_FULL:-false}" == "true" ]]; then
  BASE_SVCS+=(otel-collector control-plane watchdog)
# Observability alone: opt in with ADM_ENABLE_OTEL=true (recommended only on an
# A1/12 GB box — the OTel collector doesn't fit alongside the full stack on the
# 1 GB micro). Guarded so we don't add it twice under ADM_BATTLE_FULL.
elif [[ "${ADM_ENABLE_OTEL:-false}" == "true" ]]; then
  BASE_SVCS+=(otel-collector)
  echo "[battle] ADM_ENABLE_OTEL=true; starting the OpenTelemetry collector (Observability)."
fi
# Front the APIs with Caddy (auto-HTTPS) only when a domain is configured — its
# A record must already point at this box or Let's Encrypt issuance fails.
if [[ -n "${ADM_API_DOMAIN:-}" ]]; then
  OVERLAY_SVCS+=(caddy)
  echo "[battle] ADM_API_DOMAIN=$ADM_API_DOMAIN set; Caddy will serve HTTPS for the APIs."
fi

# Pull only the images we will actually start — critically this skips the ~3 GB
# Ollama image in Groq mode, which would otherwise waste time and disk.
echo "[battle] pulling prebuilt images from GHCR (${BASE_SVCS[*]} ${OVERLAY_SVCS[*]})..."
"${BATTLE[@]}" pull "${BASE_SVCS[@]}" "${OVERLAY_SVCS[@]}" || \
  echo "[battle] WARNING: image pull incomplete; are the GHCR packages public?"

echo "[battle] bringing up base blue-team stack (${BASE_SVCS[*]})..."
"${BATTLE[@]}" up -d --no-build "${BASE_SVCS[@]}"

if [[ "$LLM_MODE" != "openai" ]]; then
  echo "[battle] waiting for ollama to accept commands..."
  for _ in $(seq 1 60); do
    if docker exec adm-ollama ollama list >/dev/null 2>&1; then break; fi
    sleep 3
  done
  echo "[battle] pulling tiny model '$MODEL'..."
  docker exec adm-ollama ollama pull "$MODEL" || \
    echo "[battle] WARNING: model pull failed; the gateway may not answer chat attacks"
else
  echo "[battle] hosted LLM mode ($LLM_MODE); skipping on-box Ollama."
fi

echo "[battle] launching battle overlay (${OVERLAY_SVCS[*]})..."
"${BATTLE[@]}" up -d --no-build "${OVERLAY_SVCS[@]}"

echo
echo "[battle] up. Services:"
"${BATTLE[@]}" ps
PUBIP="$(curl -s --max-time 3 http://169.254.169.254/opc/v1/vnics/0/publicIp 2>/dev/null || echo '<public-ip>')"
echo
echo "[battle] Dashboard:  http://${PUBIP}:8090"
echo "[battle] Gateway:    http://${PUBIP}:8080/v1/health"

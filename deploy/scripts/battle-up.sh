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
BASE=(docker compose -f docker-compose.yml)
BATTLE=(docker compose -f docker-compose.yml -f deploy/docker-compose.battle.yml)

echo "[battle] bringing up base blue-team stack..."
"${BASE[@]}" up -d redis ollama gateway siem policy planner executor summarizer

echo "[battle] waiting for ollama to accept commands..."
for _ in $(seq 1 60); do
  if docker exec adm-ollama ollama list >/dev/null 2>&1; then break; fi
  sleep 3
done

echo "[battle] pulling tiny model '$MODEL' (fits the 1 GB micro)..."
docker exec adm-ollama ollama pull "$MODEL" || \
  echo "[battle] WARNING: model pull failed; the gateway may not answer chat attacks"

echo "[battle] building and launching battle overlay (analysis, redteam, greenteam)..."
"${BATTLE[@]}" up -d --build

echo
echo "[battle] up. Services:"
"${BATTLE[@]}" ps
PUBIP="$(curl -s --max-time 3 http://169.254.169.254/opc/v1/vnics/0/publicIp 2>/dev/null || echo '<public-ip>')"
echo
echo "[battle] Dashboard:  http://${PUBIP}:8090"
echo "[battle] Gateway:    http://${PUBIP}:8080/v1/health"

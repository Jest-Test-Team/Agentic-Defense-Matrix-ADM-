#!/bin/bash
# ADM Restart Script
# Restarts ADM services

set -euo pipefail

cd /opt/adm/repo

# Default to all services
SERVICES=${@:-"all"}

echo "============================================"
echo "  Agentic Defense Matrix - Restart"
echo "============================================"
echo ""

if [[ "$SERVICES" == "all" ]]; then
    echo "Restarting all services..."
    docker compose restart
else
    echo "Restarting: $SERVICES"
    docker compose restart $SERVICES
fi

echo ""
echo "Waiting for health checks..."
sleep 10

if curl -sf http://localhost:8080/v1/health >/dev/null 2>&1; then
    echo "Gateway is healthy"
else
    echo "Gateway not responding yet, check logs"
fi

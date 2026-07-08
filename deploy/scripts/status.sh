#!/bin/bash
# ADM Status Script
# Shows the status of all ADM services

set -euo pipefail

cd /opt/adm/repo

echo "============================================"
echo "  Agentic Defense Matrix - Status"
echo "============================================"
echo ""

# Container status
echo "=== Containers ==="
docker compose ps
echo ""

# Health check
echo "=== Health Check ==="
if curl -sf http://localhost:8080/v1/health >/dev/null 2>&1; then
    HEALTH=$(curl -s http://localhost:8080/v1/health)
    echo "Gateway: UP"
    echo "Version: $(echo $HEALTH | jq -r '.version')"
    echo "Model: $(echo $HEALTH | jq -r '.model')"
else
    echo "Gateway: DOWN"
fi
echo ""

# Ollama status
echo "=== Ollama Models ==="
if docker exec adm-ollama ollama list >/dev/null 2>&1; then
    docker exec adm-ollama ollama list
else
    echo "Ollama not running"
fi
echo ""

# Resource usage
echo "=== Resource Usage ==="
docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}" 2>/dev/null || true
echo ""

# Disk usage
echo "=== Disk Usage ==="
df -h / | tail -1 | awk '{printf "Root: %s used of %s (%s)\n", $3, $2, $5}'
docker system df 2>/dev/null || true
echo ""

# Network
echo "=== Network ==="
echo "Gateway: http://localhost:8080"
echo "Ollama: http://localhost:11434"
echo "SIEM: http://localhost:9091"
echo "OPA: http://localhost:8181"

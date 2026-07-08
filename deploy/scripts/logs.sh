#!/bin/bash
# ADM Logs Script
# Shows logs from ADM services

set -euo pipefail

cd /opt/adm/repo

# Default to all services
SERVICES=${@:-"gateway siem ollama redis planner executor summarizer policy"}

echo "============================================"
echo "  Agentic Defense Matrix - Logs"
echo "============================================"
echo ""
echo "Press Ctrl+C to stop"
echo ""

docker compose logs -f --tail=100 $SERVICES

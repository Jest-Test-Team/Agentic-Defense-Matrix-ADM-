#!/bin/bash
# ADM Update Script
# Updates ADM to the latest version

set -euo pipefail

cd /opt/adm/repo

echo "============================================"
echo "  Agentic Defense Matrix - Update"
echo "============================================"
echo ""

# Pull latest changes
echo "Pulling latest changes..."
git pull origin dev

# Rebuild images
echo "Rebuilding images..."
docker compose build

# Restart services
echo "Restarting services..."
docker compose up -d

# Wait for health
echo "Waiting for health check..."
sleep 10

if curl -sf http://localhost:8080/v1/health >/dev/null 2>&1; then
    echo "Update complete! Gateway is healthy"
else
    echo "Update complete, but gateway not responding yet"
fi

#!/bin/bash
# ADM Oracle Cloud Always Free - Setup Script
# Runs on first boot via cloud-init or manually

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')] WARNING: $1${NC}"
}

error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: $1${NC}"
    exit 1
}

# Check if running as root or with sudo
if [[ $EUID -ne 0 ]] && ! sudo -n true 2>/dev/null; then
    error "This script must be run as root or with sudo"
fi

# Configuration
ADM_HOME="/opt/adm"
ADM_REPO="$ADM_HOME/repo"
ADM_LOGS="$ADM_HOME/logs"
ADM_DATA="$ADM_HOME/data"
ADM_USER="adm"

# Create directories
log "Creating ADM directories..."
mkdir -p "$ADM_HOME" "$ADM_LOGS" "$ADM_DATA"
chown -R $ADM_USER:$ADM_USER "$ADM_HOME"

# Wait for Docker to be ready
log "Waiting for Docker..."
for i in {1..30}; do
    if docker info >/dev/null 2>&1; then
        break
    fi
    sleep 2
done
docker info >/dev/null 2>&1 || error "Docker not ready after 60 seconds"

# Clone or update repository
if [[ ! -d "$ADM_REPO" ]]; then
    log "Cloning ADM repository..."
    sudo -u $ADM_USER git clone https://github.com/Jest-Test-Team/Agentic-Defense-Matrix-ADM-.git "$ADM_REPO"
else
    log "Updating ADM repository..."
    sudo -u $ADM_USER git -C "$ADM_REPO" pull
fi

# Pull Ollama model
log "Pulling Ollama model (llama3.1:8b)..."
docker run -d --name adm-ollama-pull \
    -v ollama-models:/root/.ollama \
    ollama/ollama:latest \
    ollama pull llama3.1:8b || warn "Ollama pull may take a while"

# Wait for model download
log "Waiting for model download..."
for i in {1..120}; do
    if docker exec adm-ollama-pull ollama list 2>/dev/null | grep -q "llama3.1:8b"; then
        break
    fi
    sleep 5
done

# Stop pull container
docker stop adm-ollama-pull 2>/dev/null || true
docker rm adm-ollama-pull 2>/dev/null || true

# Build and start ADM stack
log "Building ADM stack..."
cd "$ADM_REPO"
sudo -u $ADM_USER docker compose build

log "Starting ADM stack..."
sudo -u $ADM_USER docker compose up -d

# Wait for health checks
log "Waiting for services to be healthy..."
for service in gateway siem ollama redis; do
    for i in {1..30}; do
        if sudo -u $ADM_USER docker compose ps $service 2>/dev/null | grep -q "healthy"; then
            log "$service is healthy"
            break
        fi
        sleep 5
    done
done

# Verify deployment
log "Verifying deployment..."
if curl -sf http://localhost:8080/v1/health >/dev/null; then
    log "Gateway is responding"
else
    warn "Gateway not responding yet, may need more time"
fi

# Create systemd service for ADM
log "Creating systemd service..."
cat > /etc/systemd/system/adm.service << 'EOF'
[Unit]
Description=Agentic Defense Matrix
After=docker.service
Requires=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/opt/adm/repo
ExecStart=/usr/bin/docker compose up -d
ExecStop=/usr/bin/docker compose down
TimeoutStartSec=300

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable adm.service

# Create status script
cat > /opt/adm/scripts/status.sh << 'EOF'
#!/bin/bash
cd /opt/adm/repo
echo "=== ADM Stack Status ==="
echo ""
echo "Containers:"
docker compose ps
echo ""
echo "Health Check:"
curl -s http://localhost:8080/v1/health | jq .
echo ""
echo "Ollama Models:"
docker exec adm-ollama ollama list 2>/dev/null || echo "Ollama not running"
EOF
chmod +x /opt/adm/scripts/status.sh

# Create logs script
cat > /opt/adm/scripts/logs.sh << 'EOF'
#!/bin/bash
cd /opt/adm/repo
docker compose logs -f --tail=100 $@
EOF
chmod +x /opt/adm/scripts/logs.sh

# Create restart script
cat > /opt/adm/scripts/restart.sh << 'EOF'
#!/bin/bash
cd /opt/adm/repo
docker compose restart $@
EOF
chmod +x /opt/adm/scripts/restart.sh

log "Setup complete!"
echo ""
echo "=== Next Steps ==="
echo "1. Check status: sudo -u adm /opt/adm/scripts/status.sh"
echo "2. View logs: sudo -u adm /opt/adm/scripts/logs.sh"
echo "3. Access Gateway: http://$(curl -s http://169.254.169.254/opc/v2/instance/metadata/public_ip 2>/dev/null || echo 'YOUR_IP'):8080/v1/health"

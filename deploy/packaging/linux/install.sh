#!/bin/bash
# ADM Linux Installer Script
# Run with: sudo ./install.sh

set -e

INSTALL_DIR="/opt/adm"
SYSTEMD_DIR="/etc/systemd/system"
VERSION="0.1.0"

echo "ADM Installer v$VERSION"
echo "========================"

# Check if running as root
if [[ $EUID -ne 0 ]]; then
   echo "This script must be run as root (sudo)" 
   exit 1
fi

# Create installation directory
echo "Creating installation directory..."
mkdir -p "$INSTALL_DIR"
mkdir -p "$INSTALL_DIR/config"
mkdir -p "$INSTALL_DIR/logs"

# Copy binaries
echo "Installing binaries..."
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
BINARIES=("adm-gateway" "adm-siem" "adm-watchdog" "adm-planner" 
          "adm-executor" "adm-summarizer" "adm-control-plane")

for binary in "${BINARIES[@]}"; do
    if [[ -f "$SCRIPT_DIR/$binary" ]]; then
        cp "$SCRIPT_DIR/$binary" "$INSTALL_DIR/"
        chmod +x "$INSTALL_DIR/$binary"
        echo "  Installed: $binary"
    fi
done

# Create systemd units
echo "Creating systemd services..."

# Gateway
cat > "$SYSTEMD_DIR/adm-gateway.service" << EOF
[Unit]
Description=ADM API Gateway
After=network.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/adm-gateway
WorkingDirectory=$INSTALL_DIR
Restart=always
RestartSec=5
User=adm
Group=adm

[Install]
WantedBy=multi-user.target
EOF

# Watchdog
cat > "$SYSTEMD_DIR/adm-watchdog.service" << EOF
[Unit]
Description=ADM Watchdog Daemon
After=network.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/adm-watchdog
WorkingDirectory=$INSTALL_DIR
Restart=always
RestartSec=5
User=root
Group=root

[Install]
WantedBy=multi-user.target
EOF

# SIEM
cat > "$SYSTEMD_DIR/adm-siem.service" << EOF
[Unit]
Description=ADM SIEM Engine
After=network.target redis.service

[Service]
Type=simple
ExecStart=$INSTALL_DIR/adm-siem
WorkingDirectory=$INSTALL_DIR
Restart=always
RestartSec=5
User=adm
Group=adm

[Install]
WantedBy=multi-user.target
EOF

# Create adm user
echo "Creating adm user..."
id -u adm &>/dev/null || useradd -r -s /bin/false adm
chown -R adm:adm "$INSTALL_DIR"

# Reload systemd
echo "Reloading systemd..."
systemctl daemon-reload

# Enable and start services
echo "Enabling services..."
systemctl enable adm-gateway
systemctl enable adm-watchdog
systemctl enable adm-siem

echo "Starting services..."
systemctl start adm-gateway
systemctl start adm-watchdog
systemctl start adm-siem

echo ""
echo "Installation complete!"
echo "Services installed:"
echo "  - adm-gateway (port 8080)"
echo "  - adm-watchdog (system monitor)"
echo "  - adm-siem (port 9091)"
echo ""
echo "Configuration: $INSTALL_DIR/config"
echo "Logs: $INSTALL_DIR/logs"
echo ""
echo "Manage services:"
echo "  systemctl status adm-gateway"
echo "  systemctl status adm-watchdog"
echo "  systemctl status adm-siem"

#!/bin/bash
# ADM macOS Installer Script
# Run with: sudo ./install.sh

set -e

INSTALL_DIR="/Library/ADM"
LAUNCH_DAEMON_DIR="/Library/LaunchDaemons"
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

# Create launchd plists
echo "Creating launchd services..."

# Gateway
cat > "$LAUNCH_DAEMON_DIR/com.adm.gateway.plist" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.adm.gateway</string>
    <key>ProgramArguments</key>
    <array>
        <string>$INSTALL_DIR/adm-gateway</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>$INSTALL_DIR/logs/gateway.log</string>
    <key>StandardErrorPath</key>
    <string>$INSTALL_DIR/logs/gateway.error.log</string>
</dict>
</plist>
EOF

# Watchdog
cat > "$LAUNCH_DAEMON_DIR/com.adm.watchdog.plist" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.adm.watchdog</string>
    <key>ProgramArguments</key>
    <array>
        <string>$INSTALL_DIR/adm-watchdog</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>$INSTALL_DIR/logs/watchdog.log</string>
    <key>StandardErrorPath</key>
    <string>$INSTALL_DIR/logs/watchdog.error.log</string>
</dict>
</plist>
EOF

# SIEM
cat > "$LAUNCH_DAEMON_DIR/com.adm.siem.plist" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.adm.siem</string>
    <key>ProgramArguments</key>
    <array>
        <string>$INSTALL_DIR/adm-siem</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>$INSTALL_DIR/logs/siem.log</string>
    <key>StandardErrorPath</key>
    <string>$INSTALL_DIR/logs/siem.error.log</string>
</dict>
</plist>
EOF

# Set permissions
chown root:wheel "$LAUNCH_DAEMON_DIR/com.adm.gateway.plist"
chown root:wheel "$LAUNCH_DAEMON_DIR/com.adm.watchdog.plist"
chown root:wheel "$LAUNCH_DAEMON_DIR/com.adm.siem.plist"
chmod 644 "$LAUNCH_DAEMON_DIR/com.adm.gateway.plist"
chmod 644 "$LAUNCH_DAEMON_DIR/com.adm.watchdog.plist"
chmod 644 "$LAUNCH_DAEMON_DIR/com.adm.siem.plist"

# Load services
echo "Loading services..."
launchctl load "$LAUNCH_DAEMON_DIR/com.adm.gateway.plist"
launchctl load "$LAUNCH_DAEMON_DIR/com.adm.watchdog.plist"
launchctl load "$LAUNCH_DAEMON_DIR/com.adm.siem.plist"

echo ""
echo "Installation complete!"
echo "Services installed:"
echo "  - com.adm.gateway (port 8080)"
echo "  - com.adm.watchdog (system monitor)"
echo "  - com.adm.siem (port 9091)"
echo ""
echo "Configuration: $INSTALL_DIR/config"
echo "Logs: $INSTALL_DIR/logs"

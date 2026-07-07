# ADM Windows Installer Script (PowerShell)
# Run as Administrator

param(
    [string]$InstallDir = "C:\Program Files\ADM",
    [string]$Version = "0.1.0"
)

$ErrorActionPreference = "Stop"

Write-Host "ADM Installer v$Version" -ForegroundColor Cyan
Write-Host "========================" -ForegroundColor Cyan

# Check if running as admin
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Error "Please run this script as Administrator"
    exit 1
}

# Create installation directory
Write-Host "Creating installation directory..." -ForegroundColor Yellow
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

# Copy binaries
Write-Host "Installing binaries..." -ForegroundColor Yellow
$binaries = @("adm-gateway.exe", "adm-siem.exe", "adm-watchdog.exe", 
              "adm-planner.exe", "adm-executor.exe", "adm-summarizer.exe",
              "adm-control-plane.exe")

foreach ($binary in $binaries) {
    if (Test-Path $binary) {
        Copy-Item $binary -Destination $InstallDir -Force
        Write-Host "  Installed: $binary" -ForegroundColor Green
    }
}

# Create config directory
Write-Host "Creating configuration..." -ForegroundColor Yellow
$configDir = Join-Path $InstallDir "config"
New-Item -ItemType Directory -Force -Path $configDir | Out-Null

# Install Windows services
Write-Host "Installing services..." -ForegroundColor Yellow

# Gateway Service
New-Service -Name "ADM-Gateway" `
    -BinaryPathName "$InstallDir\adm-gateway.exe" `
    -DisplayName "ADM Gateway" `
    -Description "Agentic Defense Matrix API Gateway" `
    -StartupType Automatic

# Watchdog Service
New-Service -Name "ADM-Watchdog" `
    -BinaryPathName "$InstallDir\adm-watchdog.exe" `
    -DisplayName "ADM Watchdog" `
    -Description "Agentic Defense Matrix Endpoint Monitor" `
    -StartupType Automatic

# SIEM Service
New-Service -Name "ADM-SIEM" `
    -BinaryPathName "$InstallDir\adm-siem.exe" `
    -DisplayName "ADM SIEM Engine" `
    -Description "Agentic Defense Matrix SIEM Engine" `
    -StartupType Automatic

# Start services
Write-Host "Starting services..." -ForegroundColor Yellow
Start-Service "ADM-Gateway"
Start-Service "ADM-Watchdog"
Start-Service "ADM-SIEM"

Write-Host ""
Write-Host "Installation complete!" -ForegroundColor Green
Write-Host "Services installed:" -ForegroundColor Cyan
Write-Host "  - ADM-Gateway (port 8080)" -ForegroundColor White
Write-Host "  - ADM-Watchdog (system tray)" -ForegroundColor White
Write-Host "  - ADM-SIEM (port 9091)" -ForegroundColor White
Write-Host ""
Write-Host "Configuration: $configDir" -ForegroundColor Cyan
Write-Host "Logs: $InstallDir\logs" -ForegroundColor Cyan

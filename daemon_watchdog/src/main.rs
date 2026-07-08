mod config;
mod egress_blocker;
mod macos_es;
mod policy_enforcer;
mod telemetry;
mod wfp_filter;

use config::Config;
use std::sync::Arc;
use tokio::sync::RwLock;
use tracing::{error, info, warn};

pub struct Watchdog {
    config: Config,
    sessions: Arc<RwLock<Vec<config::SessionInfo>>>,
    policy_enforcer: policy_enforcer::PolicyEnforcer,
    egress_blocker: egress_blocker::EgressBlocker,
    telemetry: telemetry::TelemetryCollector,
}

impl Watchdog {
    pub fn new(config: Config) -> Self {
        let policy_enforcer = policy_enforcer::PolicyEnforcer::new(&config);
        let egress_blocker = egress_blocker::EgressBlocker::new(&config);
        let telemetry = telemetry::TelemetryCollector::new(&config);

        Self {
            config,
            sessions: Arc::new(RwLock::new(Vec::new())),
            policy_enforcer,
            egress_blocker,
            telemetry,
        }
    }

    pub async fn start(&mut self) -> Result<(), Box<dyn std::error::Error>> {
        info!("Starting ADM Watchdog daemon");

        // Initialize platform-specific interceptor
        if self.config.is_darwin() {
            info!("Detected macOS - initializing Endpoint Security interceptor");
            macos_es::initialize(&self.config)?;
        } else if self.config.is_windows() {
            info!("Detected Windows - initializing WFP filter interceptor");
            wfp_filter::initialize(&self.config)?;
        } else {
            warn!("Platform not fully supported - running in limited mode");
        }

        // Start telemetry export
        self.telemetry.start_export_loop().await;

        // Start Unix socket listener for Gateway communication
        let socket_path = self.config.socket_path().to_string();
        info!("Listening on socket: {}", socket_path);

        // Ensure socket directory exists
        if let Some(parent) = std::path::Path::new(&socket_path).parent() {
            std::fs::create_dir_all(parent)?;
        }

        // Remove old socket if exists
        let _ = std::fs::remove_file(&socket_path);

        let listener = tokio::net::UnixListener::bind(&socket_path)?;
        let sessions = self.sessions.clone();
        let _config = self.config.clone();

        loop {
            match listener.accept().await {
                Ok((stream, _)) => {
                    let sessions = sessions.clone();
                    tokio::spawn(async move {
                        if let Err(e) = handle_connection(stream, sessions).await {
                            error!("Connection error: {}", e);
                        }
                    });
                }
                Err(e) => {
                    error!("Accept error: {}", e);
                }
            }
        }
    }

    pub async fn add_session(&self, session: config::SessionInfo) {
        let mut sessions = self.sessions.write().await;
        sessions.push(session);
    }

    pub async fn remove_session(&self, session_id: &str) {
        let mut sessions = self.sessions.write().await;
        sessions.retain(|s| s.session_id != session_id);
    }

    pub async fn get_sessions(&self) -> Vec<config::SessionInfo> {
        self.sessions.read().await.clone()
    }

    pub fn get_stats(&self) -> WatchdogStats {
        WatchdogStats {
            active_sessions: 0,
            syscalls_intercepted: self.telemetry.syscalls_intercepted(),
            connections_blocked: self.egress_blocker.connections_blocked(),
        }
    }
}

#[derive(Debug)]
pub struct WatchdogStats {
    pub active_sessions: u32,
    pub syscalls_intercepted: u64,
    pub connections_blocked: u64,
}

async fn handle_connection(
    mut stream: tokio::net::UnixStream,
    sessions: Arc<RwLock<Vec<config::SessionInfo>>>,
) -> Result<(), Box<dyn std::error::Error>> {
    use tokio::io::{AsyncReadExt, AsyncWriteExt};

    let mut buffer = vec![0u8; 4096];
    loop {
        let n = stream.read(&mut buffer).await?;
        if n == 0 {
            break;
        }

        let request = String::from_utf8_lossy(&buffer[..n]);
        info!("Received request: {}", request);

        let response = match request.trim() {
            "status" => {
                let sessions = sessions.read().await;
                format!("active_sessions: {}", sessions.len())
            }
            "health" => "ok".to_string(),
            _ => "unknown command".to_string(),
        };

        stream.write_all(response.as_bytes()).await?;
    }

    Ok(())
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("info")),
        )
        .init();

    let config_path = std::env::var("ADM_CONFIG").unwrap_or_else(|_| {
        if cfg!(target_os = "macos") {
            "/etc/adm/watchdog.toml".to_string()
        } else if cfg!(target_os = "windows") {
            "C:\\Program Files\\ADM\\watchdog.toml".to_string()
        } else {
            "/etc/adm/watchdog.toml".to_string()
        }
    });

    let config = Config::load(&config_path).unwrap_or_else(|e| {
        warn!("Failed to load config from {}: {}, using defaults", config_path, e);
        Config {
            daemon: config::DaemonConfig {
                socket_path: "/var/run/adm/watchdog.sock".to_string(),
                log_level: "info".to_string(),
                telemetry_interval_ms: 1000,
            },
            platform: config::PlatformConfig::default(),
            egress: config::EgressConfig::default(),
            telemetry: config::TelemetryConfig::default(),
            process_monitor: config::ProcessMonitorConfig::default(),
        }
    });

    let mut watchdog = Watchdog::new(config);
    watchdog.start().await?;

    Ok(())
}

#![allow(dead_code)]

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

        // Lightweight TCP liveness endpoint so the analysis engine / Docker
        // healthcheck can see the daemon is up (the gateway channel is a Unix
        // socket, which isn't reachable across the container network).
        Self::spawn_health_server();

        self.listen_for_gateway().await
    }

    /// Bind a tiny TCP server that replies "ok" to any connection. Address comes
    /// from ADM_WATCHDOG_HEALTH_ADDR (default 0.0.0.0:9084).
    fn spawn_health_server() {
        let addr =
            std::env::var("ADM_WATCHDOG_HEALTH_ADDR").unwrap_or_else(|_| "0.0.0.0:9084".to_string());
        tokio::spawn(async move {
            match tokio::net::TcpListener::bind(&addr).await {
                Ok(listener) => {
                    info!("watchdog health listener on {}", addr);
                    loop {
                        if let Ok((mut stream, _)) = listener.accept().await {
                            use tokio::io::AsyncWriteExt;
                            let _ = stream.write_all(b"ok\n").await;
                        }
                    }
                }
                Err(e) => error!("health listener bind {} failed: {}", addr, e),
            }
        });
    }

    #[cfg(unix)]
    async fn listen_for_gateway(&self) -> Result<(), Box<dyn std::error::Error>> {
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

    #[cfg(not(unix))]
    async fn listen_for_gateway(&self) -> Result<(), Box<dyn std::error::Error>> {
        warn!("Gateway Unix socket listener is disabled on this platform");
        futures::future::pending::<()>().await;
        Ok(())
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

#[cfg(unix)]
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
    // Docker healthcheck path: `adm-watchdog --health` probes the TCP liveness
    // port and exits 0/1 instead of starting the daemon.
    if std::env::args().any(|a| a == "--health") {
        let addr = std::env::var("ADM_WATCHDOG_HEALTH_ADDR")
            .unwrap_or_else(|_| "127.0.0.1:9084".to_string())
            .replace("0.0.0.0", "127.0.0.1");
        std::process::exit(match std::net::TcpStream::connect(&addr) {
            Ok(_) => 0,
            Err(_) => 1,
        });
    }

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
        warn!(
            "Failed to load config from {}: {}, using defaults",
            config_path, e
        );
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

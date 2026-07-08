use serde::Deserialize;

#[derive(Debug, Deserialize, Clone)]
pub struct Config {
    pub daemon: DaemonConfig,
    #[serde(default)]
    pub platform: PlatformConfig,
    #[serde(default)]
    pub egress: EgressConfig,
    #[serde(default)]
    pub telemetry: TelemetryConfig,
    #[serde(default)]
    pub process_monitor: ProcessMonitorConfig,
}

#[derive(Debug, Deserialize, Clone)]
pub struct DaemonConfig {
    pub socket_path: String,
    pub log_level: String,
    pub telemetry_interval_ms: u64,
}

#[derive(Debug, Deserialize, Clone, Default)]
pub struct PlatformConfig {
    #[serde(default)]
    pub darwin: DarwinConfig,
    #[serde(default)]
    pub windows: WindowsConfig,
}

#[derive(Debug, Deserialize, Clone, Default)]
pub struct DarwinConfig {
    #[serde(default = "default_bundle_id")]
    pub bundle_id: String,
    #[serde(default = "default_darwin_events")]
    pub events: Vec<String>,
}

fn default_bundle_id() -> String {
    "com.adm.watchdog".to_string()
}

fn default_darwin_events() -> Vec<String> {
    vec![
        "ES_EVENT_TYPE_AUTH_EXEC".to_string(),
        "ES_EVENT_TYPE_AUTH_CONNECT".to_string(),
        "ES_EVENT_TYPE_AUTH_FILE_OPEN".to_string(),
    ]
}

#[derive(Debug, Deserialize, Clone, Default)]
pub struct WindowsConfig {
    #[serde(default = "default_sublayer_name")]
    pub sublayer_name: String,
    #[serde(default = "default_filter_layers")]
    pub filter_layers: Vec<String>,
}

fn default_sublayer_name() -> String {
    "ADM Watchdog Filter".to_string()
}

fn default_filter_layers() -> Vec<String> {
    vec![
        "FWPM_LAYER_ALE_AUTH_CONNECT_V4".to_string(),
        "FWPM_LAYER_ALE_AUTH_CONNECT_V6".to_string(),
    ]
}

#[derive(Debug, Deserialize, Clone)]
pub struct EgressConfig {
    #[serde(default = "default_egress_policy")]
    pub default_policy: String,
    #[serde(default)]
    pub whitelist: Vec<String>,
    #[serde(default = "default_true")]
    pub intercept_dns: bool,
    #[serde(default = "default_dns_rate_limit")]
    pub dns_rate_limit: u32,
}

impl Default for EgressConfig {
    fn default() -> Self {
        Self {
            default_policy: default_egress_policy(),
            whitelist: vec![],
            intercept_dns: default_true(),
            dns_rate_limit: default_dns_rate_limit(),
        }
    }
}

fn default_egress_policy() -> String {
    "deny".to_string()
}

fn default_true() -> bool {
    true
}

fn default_dns_rate_limit() -> u32 {
    10
}

#[derive(Debug, Deserialize, Clone)]
pub struct TelemetryConfig {
    pub otlp_endpoint: String,
    pub service_name: String,
    pub export_interval_ms: u64,
    pub enable_metrics: bool,
}

impl Default for TelemetryConfig {
    fn default() -> Self {
        Self {
            otlp_endpoint: "http://localhost:4317".to_string(),
            service_name: "adm-watchdog".to_string(),
            export_interval_ms: 5000,
            enable_metrics: true,
        }
    }
}

#[derive(Debug, Deserialize, Clone)]
pub struct ProcessMonitorConfig {
    #[serde(default = "default_allowed_binaries")]
    pub allowed_binaries: Vec<String>,
    #[serde(default = "default_blocked_patterns")]
    pub blocked_patterns: Vec<String>,
    #[serde(default = "default_max_processes")]
    pub max_processes_per_session: u32,
}

impl Default for ProcessMonitorConfig {
    fn default() -> Self {
        Self {
            allowed_binaries: default_allowed_binaries(),
            blocked_patterns: default_blocked_patterns(),
            max_processes_per_session: default_max_processes(),
        }
    }
}

fn default_allowed_binaries() -> Vec<String> {
    vec![
        "/usr/bin/python3".to_string(),
        "/usr/bin/python".to_string(),
        "/usr/bin/node".to_string(),
        "/usr/bin/java".to_string(),
        "/bin/sh".to_string(),
        "/bin/bash".to_string(),
    ]
}

fn default_blocked_patterns() -> Vec<String> {
    vec![
        r"bash\s+-i".to_string(),
        r"nc\s+.*-e".to_string(),
        r"curl\s+.*\|.*sh".to_string(),
        r"wget\s+.*\|.*sh".to_string(),
    ]
}

fn default_max_processes() -> u32 {
    50
}

impl Config {
    pub fn load(path: &str) -> Result<Self, Box<dyn std::error::Error>> {
        let content = std::fs::read_to_string(path)?;
        let config: Config = toml::from_str(&content)?;
        Ok(config)
    }

    pub fn socket_path(&self) -> &str {
        &self.daemon.socket_path
    }

    pub fn is_darwin(&self) -> bool {
        cfg!(target_os = "macos")
    }

    pub fn is_windows(&self) -> bool {
        cfg!(target_os = "windows")
    }

    pub fn is_linux(&self) -> bool {
        cfg!(target_os = "linux")
    }
}

#[derive(Debug, Clone)]
pub struct SessionInfo {
    pub session_id: String,
    pub pid: u32,
    pub container_id: Option<String>,
    pub created_at: std::time::Instant,
    pub syscall_count: u64,
    pub connections: Vec<ConnectionInfo>,
}

#[derive(Debug, Clone)]
pub struct ConnectionInfo {
    pub remote_addr: String,
    pub remote_port: u16,
    pub protocol: String,
    pub allowed: bool,
}

#[derive(Debug, Clone)]
pub struct SyscallEvent {
    pub event_id: String,
    pub event_type: String,
    pub process_name: String,
    pub process_path: String,
    pub arguments: Vec<String>,
    pub result: String,
    pub timestamp_ns: i64,
    pub session_id: String,
}

#[derive(Debug, Clone, PartialEq)]
pub enum FilterAction {
    Allow,
    Deny,
}

#[derive(Debug, Clone)]
pub struct FilterRule {
    pub rule_id: String,
    pub protocol: String,
    pub direction: String,
    pub remote_addr: String,
    pub remote_port: u16,
    pub action: FilterAction,
    pub priority: i32,
}

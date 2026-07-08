use crate::config::Config;
use anyhow::Result;
use tracing::{info, warn};

#[cfg(target_os = "macos")]
pub fn initialize(_config: &Config) -> Result<()> {
    info!("Initializing macOS Endpoint Security interceptor");

    if !is_root() {
        warn!("Watchdog should run as root for full Endpoint Security access");
    }

    info!("macOS ES interceptor initialized (placeholder)");
    Ok(())
}

#[cfg(not(target_os = "macos"))]
pub fn initialize(_config: &Config) -> Result<()> {
    warn!("macOS Endpoint Security not available on this platform");
    Ok(())
}

#[cfg(target_os = "macos")]
fn is_root() -> bool {
    std::process::Command::new("id")
        .args(["-u"])
        .output()
        .map(|o| String::from_utf8_lossy(&o.stdout).trim() == "0")
        .unwrap_or(false)
}

#[cfg(not(target_os = "macos"))]
fn is_root() -> bool {
    false
}

pub struct MacOSESEvent {
    pub event_type: String,
    pub process_id: u32,
    pub process_path: String,
    pub executable_path: String,
    pub arguments: Vec<String>,
    pub timestamp: i64,
}

impl MacOSESEvent {
    pub fn to_syscall_event(&self, session_id: &str) -> crate::config::SyscallEvent {
        crate::config::SyscallEvent {
            event_id: uuid::Uuid::new_v4().to_string(),
            event_type: self.event_type.clone(),
            process_name: std::path::Path::new(&self.executable_path)
                .file_name()
                .map(|n| n.to_string_lossy().to_string())
                .unwrap_or_default(),
            process_path: self.executable_path.clone(),
            arguments: self.arguments.clone(),
            result: "observed".to_string(),
            timestamp_ns: self.timestamp,
            session_id: session_id.to_string(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_syscall_event_conversion() {
        let event = MacOSESEvent {
            event_type: "ES_EVENT_TYPE_AUTH_EXEC".to_string(),
            process_id: 1234,
            process_path: "/bin/bash".to_string(),
            executable_path: "/bin/bash".to_string(),
            arguments: vec!["-c".to_string(), "ls".to_string()],
            timestamp: 1234567890,
        };

        let syscall = event.to_syscall_event("session-1");
        assert_eq!(syscall.event_type, "ES_EVENT_TYPE_AUTH_EXEC");
        assert_eq!(syscall.process_name, "bash");
        assert_eq!(syscall.session_id, "session-1");
    }
}

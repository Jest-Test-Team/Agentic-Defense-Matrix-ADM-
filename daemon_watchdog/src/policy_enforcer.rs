use crate::config::{Config, SyscallEvent};
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::RwLock;
use tracing::{info, warn};

pub struct PolicyEnforcer {
    policies: Arc<RwLock<HashMap<String, Policy>>>,
    blocked_patterns: Vec<regex::Regex>,
    allowed_binaries: Vec<String>,
    _max_processes_per_session: u32,
}

#[derive(Debug, Clone)]
pub struct Policy {
    pub id: String,
    pub name: String,
    pub rego_source: String,
    pub enabled: bool,
}

impl PolicyEnforcer {
    pub fn new(config: &Config) -> Self {
        let blocked_patterns: Vec<regex::Regex> = config
            .process_monitor
            .blocked_patterns
            .iter()
            .filter_map(|p| match regex::Regex::new(p) {
                Ok(r) => Some(r),
                Err(e) => {
                    warn!("Invalid regex pattern '{}': {}", p, e);
                    None
                }
            })
            .collect();

        Self {
            policies: Arc::new(RwLock::new(HashMap::new())),
            blocked_patterns,
            allowed_binaries: config.process_monitor.allowed_binaries.clone(),
            _max_processes_per_session: config.process_monitor.max_processes_per_session,
        }
    }

    pub async fn load_policy(&self, id: &str, name: &str, rego_source: &str) {
        let mut policies = self.policies.write().await;
        policies.insert(
            id.to_string(),
            Policy {
                id: id.to_string(),
                name: name.to_string(),
                rego_source: rego_source.to_string(),
                enabled: true,
            },
        );
        info!("Loaded policy: {} ({})", name, id);
    }

    pub async fn evaluate_syscall(&self, event: &SyscallEvent) -> PolicyDecision {
        for pattern in &self.blocked_patterns {
            let combined = format!("{} {}", event.process_name, event.arguments.join(" "));
            if pattern.is_match(&combined) {
                return PolicyDecision {
                    allowed: false,
                    reason: format!("Blocked by pattern: {}", pattern.as_str()),
                    action: PolicyAction::Kill,
                };
            }
        }

        if !self.allowed_binaries.is_empty() {
            let is_allowed = self
                .allowed_binaries
                .iter()
                .any(|b| event.process_path.starts_with(b));

            if !is_allowed {
                return PolicyDecision {
                    allowed: false,
                    reason: format!("Binary not in allowlist: {}", event.process_path),
                    action: PolicyAction::Deny,
                };
            }
        }

        PolicyDecision {
            allowed: true,
            reason: "Passed all policy checks".to_string(),
            action: PolicyAction::Allow,
        }
    }

    pub async fn reload_policies(&self) -> u32 {
        let policies = self.policies.read().await;
        policies.len() as u32
    }
}

#[derive(Debug)]
pub struct PolicyDecision {
    pub allowed: bool,
    pub reason: String,
    pub action: PolicyAction,
}

#[derive(Debug)]
pub enum PolicyAction {
    Allow,
    Deny,
    Kill,
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::ProcessMonitorConfig;

    fn test_config() -> Config {
        Config {
            daemon: crate::config::DaemonConfig {
                socket_path: "/tmp/test.sock".to_string(),
                log_level: "info".to_string(),
                telemetry_interval_ms: 1000,
            },
            platform: crate::config::PlatformConfig::default(),
            egress: crate::config::EgressConfig::default(),
            telemetry: crate::config::TelemetryConfig::default(),
            process_monitor: ProcessMonitorConfig {
                allowed_binaries: vec!["/bin/bash".to_string()],
                blocked_patterns: vec![r"bash\s+-i".to_string()],
                max_processes_per_session: 50,
            },
        }
    }

    #[tokio::test]
    async fn test_blocked_pattern() {
        let enforcer = PolicyEnforcer::new(&test_config());
        let event = SyscallEvent {
            event_id: "test".to_string(),
            event_type: "exec".to_string(),
            process_name: "bash".to_string(),
            process_path: "/bin/bash".to_string(),
            arguments: vec!["-i".to_string()],
            result: "observed".to_string(),
            timestamp_ns: 0,
            session_id: "session-1".to_string(),
        };

        let decision = enforcer.evaluate_syscall(&event).await;
        assert!(!decision.allowed);
    }
}

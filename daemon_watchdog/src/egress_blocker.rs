use crate::config::{Config, FilterAction, FilterRule};
use std::collections::{HashMap, HashSet};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use tokio::sync::RwLock;
use tracing::{debug, info, warn};

pub struct EgressBlocker {
    default_policy: FilterAction,
    global_whitelist: HashSet<String>,
    session_rules: Arc<RwLock<HashMap<String, Vec<FilterRule>>>>,
    connections_blocked: Arc<AtomicU64>,
    intercept_dns: bool,
    dns_rate_limit: u32,
}

impl EgressBlocker {
    pub fn new(config: &Config) -> Self {
        let default_policy = match config.egress.default_policy.as_str() {
            "deny" => FilterAction::Deny,
            "allow" => FilterAction::Allow,
            _ => {
                warn!("Unknown egress policy '{}', defaulting to deny", config.egress.default_policy);
                FilterAction::Deny
            }
        };

        let global_whitelist: HashSet<String> = config.egress.whitelist.iter().cloned().collect();

        Self {
            default_policy,
            global_whitelist,
            session_rules: Arc::new(RwLock::new(HashMap::new())),
            connections_blocked: Arc::new(AtomicU64::new(0)),
            intercept_dns: config.egress.intercept_dns,
            dns_rate_limit: config.egress.dns_rate_limit,
        }
    }

    pub async fn add_session_filter(&self, session_id: &str, rules: Vec<FilterRule>) {
        let count = rules.len();
        let mut session_rules = self.session_rules.write().await;
        session_rules.insert(session_id.to_string(), rules);
        info!("Added {} filter rules for session {}", count, session_id);
    }

    pub async fn remove_session_filter(&self, session_id: &str) {
        let mut session_rules = self.session_rules.write().await;
        session_rules.remove(session_id);
        info!("Removed filter rules for session {}", session_id);
    }

    pub async fn check_connection(
        &self,
        session_id: &str,
        remote_addr: &str,
        remote_port: u16,
    ) -> bool {
        let session_rules = self.session_rules.read().await;

        // Check session-specific rules
        if let Some(rules) = session_rules.get(session_id) {
            for rule in rules {
                if self.matches_rule(rule, remote_addr, remote_port) {
                    return rule.action == FilterAction::Allow;
                }
            }
        }

        // Check global whitelist
        if self.global_whitelist.contains(remote_addr) {
            return true;
        }

        // Apply default policy
        match self.default_policy {
            FilterAction::Allow => true,
            FilterAction::Deny => {
                self.connections_blocked.fetch_add(1, Ordering::Relaxed);
                debug!("Blocked connection to {}:{}", remote_addr, remote_port);
                false
            }
        }
    }

    fn matches_rule(&self, rule: &FilterRule, remote_addr: &str, remote_port: u16) -> bool {
        // Simple matching - in production would use CIDR matching
        if rule.remote_addr == remote_addr || rule.remote_addr == "0.0.0.0/0" {
            if rule.remote_port == 0 || rule.remote_port == remote_port {
                return true;
            }
        }
        false
    }

    pub fn connections_blocked(&self) -> u64 {
        self.connections_blocked.load(Ordering::Relaxed)
    }

    pub fn should_intercept_dns(&self) -> bool {
        self.intercept_dns
    }

    pub fn dns_rate_limit(&self) -> u32 {
        self.dns_rate_limit
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::EgressConfig;

    fn test_config() -> Config {
        Config {
            daemon: crate::config::DaemonConfig {
                socket_path: "/tmp/test.sock".to_string(),
                log_level: "info".to_string(),
                telemetry_interval_ms: 1000,
            },
            platform: crate::config::PlatformConfig::default(),
            egress: EgressConfig {
                default_policy: "deny".to_string(),
                whitelist: vec!["10.0.0.1".to_string()],
                intercept_dns: true,
                dns_rate_limit: 10,
            },
            telemetry: crate::config::TelemetryConfig::default(),
            process_monitor: crate::config::ProcessMonitorConfig::default(),
        }
    }

    #[tokio::test]
    async fn test_blocked_by_default() {
        let blocker = EgressBlocker::new(&test_config());
        let allowed = blocker.check_connection("session-1", "evil.com", 443).await;
        assert!(!allowed);
    }

    #[tokio::test]
    async fn test_whitelisted() {
        let blocker = EgressBlocker::new(&test_config());
        let allowed = blocker.check_connection("session-1", "10.0.0.1", 443).await;
        assert!(allowed);
    }
}

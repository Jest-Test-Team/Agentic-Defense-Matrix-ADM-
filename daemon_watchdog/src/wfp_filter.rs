use crate::config::Config;
use anyhow::Result;
use tracing::warn;

#[cfg(target_os = "windows")]
pub fn initialize(_config: &Config) -> Result<()> {
    use tracing::info;
    info!("Initializing Windows WFP filter interceptor");
    info!("WFP filter interceptor initialized (placeholder)");
    Ok(())
}

#[cfg(not(target_os = "windows"))]
pub fn initialize(_config: &Config) -> Result<()> {
    warn!("Windows WFP not available on this platform");
    Ok(())
}

pub struct WFPFilter {
    pub filter_id: u64,
    pub layer: String,
    pub remote_addr: String,
    pub remote_port: u16,
    pub action: FilterAction,
}

#[derive(Debug, Clone, PartialEq)]
pub enum FilterAction {
    Permit,
    Block,
}

impl WFPFilter {
    pub fn to_filter_rule(&self, _session_id: &str) -> crate::config::FilterRule {
        crate::config::FilterRule {
            rule_id: format!("wfp-{}", self.filter_id),
            protocol: "tcp".to_string(),
            direction: "outbound".to_string(),
            remote_addr: self.remote_addr.clone(),
            remote_port: self.remote_port,
            action: match self.action {
                FilterAction::Permit => crate::config::FilterAction::Allow,
                FilterAction::Block => crate::config::FilterAction::Deny,
            },
            priority: 100,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_filter_rule_conversion() {
        let filter = WFPFilter {
            filter_id: 12345,
            layer: "FWPM_LAYER_ALE_AUTH_CONNECT_V4".to_string(),
            remote_addr: "10.0.0.1".to_string(),
            remote_port: 443,
            action: FilterAction::Permit,
        };

        let rule = filter.to_filter_rule("session-1");
        assert_eq!(rule.rule_id, "wfp-12345");
        assert_eq!(rule.action, crate::config::FilterAction::Allow);
    }
}

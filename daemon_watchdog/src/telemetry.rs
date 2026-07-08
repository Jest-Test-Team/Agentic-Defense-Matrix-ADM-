use crate::config::{Config, SyscallEvent};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use tokio::sync::mpsc;
use tracing::{debug, info};

pub struct TelemetryCollector {
    syscalls_intercepted: Arc<AtomicU64>,
    export_interval_ms: u64,
    otlp_endpoint: String,
    _service_name: String,
    event_sender: mpsc::Sender<SyscallEvent>,
    event_receiver: Option<mpsc::Receiver<SyscallEvent>>,
}

impl TelemetryCollector {
    pub fn new(config: &Config) -> Self {
        let (event_sender, event_receiver) = mpsc::channel(10000);

        Self {
            syscalls_intercepted: Arc::new(AtomicU64::new(0)),
            export_interval_ms: config.telemetry.export_interval_ms,
            otlp_endpoint: config.telemetry.otlp_endpoint.clone(),
            _service_name: config.telemetry.service_name.clone(),
            event_sender,
            event_receiver: Some(event_receiver),
        }
    }

    pub async fn start_export_loop(&mut self) {
        let receiver = self.event_receiver.take().expect("Receiver already taken");
        let syscalls = self.syscalls_intercepted.clone();
        let endpoint = self.otlp_endpoint.clone();

        tokio::spawn(async move {
            Self::export_loop(receiver, syscalls, endpoint).await;
        });
    }

    async fn export_loop(
        mut receiver: mpsc::Receiver<SyscallEvent>,
        syscalls: Arc<AtomicU64>,
        endpoint: String,
    ) {
        info!("Starting telemetry export loop to {}", endpoint);

        while let Some(_event) = receiver.recv().await {
            syscalls.fetch_add(1, Ordering::Relaxed);
            debug!("Exporting syscall event");
        }
    }

    pub fn record_syscall(&self, event: &SyscallEvent) {
        let sender = self.event_sender.clone();
        let event = event.clone();
        tokio::spawn(async move {
            let _ = sender.send(event).await;
        });
    }

    pub fn syscalls_intercepted(&self) -> u64 {
        self.syscalls_intercepted.load(Ordering::Relaxed)
    }
}

impl Clone for TelemetryCollector {
    fn clone(&self) -> Self {
        Self {
            syscalls_intercepted: self.syscalls_intercepted.clone(),
            export_interval_ms: self.export_interval_ms,
            otlp_endpoint: self.otlp_endpoint.clone(),
            _service_name: self._service_name.clone(),
            event_sender: self.event_sender.clone(),
            event_receiver: None,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_syscall_counter() {
        let counter = Arc::new(AtomicU64::new(0));
        counter.fetch_add(1, Ordering::Relaxed);
        counter.fetch_add(1, Ordering::Relaxed);
        assert_eq!(counter.load(Ordering::Relaxed), 2);
    }
}

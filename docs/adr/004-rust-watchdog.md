# ADR-004: Rust for Endpoint Watchdog Daemon

## Status

Accepted

## Context

The watchdog daemon intercepts OS-level syscalls (process execution, network connections) on both macOS and Windows. This requires memory safety, low latency, and direct OS API access.

## Decision

Write the watchdog daemon in **Rust**.

### Why Rust?
- Memory safety without garbage collector (critical for kernel-level intercepts)
- Direct FFI to macOS Endpoint Security and Windows WFP APIs
- Zero-cost abstractions for high-throughput event processing
- Cross-compilation to all target platforms
- Strong async runtime (Tokio) for concurrent event handling

### Platform Implementations
- **macOS**: `security-framework` crate wrapping Endpoint Security API
- **Windows**: `windows-sys` crate wrapping WFP Filter Engine
- **Linux**: eBPF / auditd (future)

## Consequences

- Separate build pipeline (Cargo) from Go services
- GHA matrix build with cross-compilation
- Rust binary distributed as standalone daemon (no runtime dependencies)
- Team needs Rust expertise (accepted investment)

## Alternatives Considered

| Alternative | Rejected Because |
|-------------|-----------------|
| Go | CGO needed for OS APIs, GC pauses |
| C/C++ | Memory safety risks, no modern toolchain |
| eBPF only | Linux-only, limited macOS/Windows support |
| Python | Too slow for syscall interception |

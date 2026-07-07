# ADR-002: Redis Streams for SIEM Hot Path

## Status

Accepted

## Context

The SIEM engine needs to ingest, correlate, and query security events in real-time with sub-5ms latency. The system must handle 1000+ concurrent agents generating events at high throughput.

## Decision

Use **Redis Streams** as the primary hot-path storage with consumer groups for parallel processing.

### Why Redis Streams?
- Sub-millisecond read/write latency
- Consumer groups enable parallel SIEM workers
- Built-in persistence (AOF + RDB)
- Memory-efficient with MAXLEN trimming for auto-eviction
- Mature ecosystem with Go/Rust clients

### Storage Tiers
- **Hot (0-7 days)**: Redis Streams with MAXLEN auto-trimming
- **Warm (7-30 days)**: Local compressed JSONL files (rotated daily)
- **Cold (30-180 days)**: Exported to S3-compatible storage

## Consequences

- Ring buffer (in-memory) feeds Redis Streams for burst absorption
- Consumer groups allow horizontal scaling of correlation rules
- MAXLEN stream trimming provides automatic hot-path retention
- Redis failure degrades to ring buffer only (graceful degradation)

## Alternatives Considered

| Alternative | Rejected Because |
|-------------|-----------------|
| Kafka | Operational overhead too high for single-host deployment |
| NATS JetStream | Less mature streaming semantics |
| PostgreSQL | Insufficient throughput for 10K+ events/sec |
| Elasticsearch | Resource-heavy, not needed for hot path |
| Pure in-memory | No persistence, no consumer groups |

# ADR-003: Separate Agent Microservices with gRPC

## Status

Accepted

## Context

The Planner, Executor, and Summarizer agents have different resource requirements, scaling characteristics, and security boundaries. Co-locating them in a single process creates blast radius issues.

## Decision

Deploy each agent as a separate gRPC microservice with its own container.

### Why Separate Services?
- **Isolation**: Compromised agent can't pivot to others
- **Scaling**: Executor needs more resources (Docker API), Summarizer is lightweight
- **Security**: Each service has least-privilege access
- **Deployability**: Independent rollouts and rollbacks

### Why gRPC?
- Strong typing via protobuf
- Bi-directional streaming (for LLM responses)
- Built-in health checking
- Efficient binary serialization

## Consequences

- Gateway orchestrates multi-step workflows across agents
- Each agent runs in its own Docker container with resource limits
- Network policies enforce inter-service communication rules
- More moving parts than monolith (accepted tradeoff for security)

## Architecture

```
Gateway ──gRPC──► Planner ──HTTP──► Ollama
       ──gRPC──► Executor ──Docker API──► Sandbox Container
       ──gRPC──► Summarizer ──HTTP──► Ollama
```

## Alternatives Considered

| Alternative | Rejected Because |
|-------------|-----------------|
| Single monolith | Higher blast radius, no isolation |
| HTTP/REST between agents | Less efficient, no streaming |
| Message queue (async) | Adds latency for synchronous tool calls |

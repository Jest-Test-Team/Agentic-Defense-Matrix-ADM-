# ADR-005: Ollama for Local LLM Inference

## Status

Accepted

## Context

ADM requires LLM inference for planning, execution, and summarization. Cloud APIs (OpenAI, Anthropic) introduce latency, cost, data privacy concerns, and external dependencies.

## Decision

Use **Ollama** as the local LLM runtime with OpenAI-compatible API.

### Supported Models
- **llama3.1:8b** — General purpose, strong tool calling
- **qwen2.5:7b** — Compact, efficient, good tool calling
- **mistral** — Company local version, production workloads
- **nomic-embed-text** — Embedding for semantic analysis

### Why Ollama?
- OpenAI-compatible HTTP API (drop-in replacement)
- Native GPU support (Metal on macOS, CUDA on Linux)
- Model management (pull, list, run)
- Lightweight single binary deployment
- Active community and model ecosystem

## Consequences

- All LLM calls stay on-premise (data sovereignty)
- No API key management needed
- Model quality limited by local hardware
- Fallback to cloud API can be added later via adapter pattern

## Architecture

```
Agent Service ──HTTP──► Ollama ──► Local Model
                    (localhost:11434)
```

## Alternatives Considered

| Alternative | Rejected Because |
|-------------|-----------------|
| OpenAI API | Data leaves premises, cost at scale |
| vLLM | Heavier deployment, Python dependency |
| llama.cpp direct | No HTTP API, must build integration |
| LocalAI | Less mature, fewer model options |
| TGI (HuggingFace) | Docker-heavy, less flexible |

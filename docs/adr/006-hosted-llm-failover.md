# ADR-006: Hosted-LLM Mode with Groq → X.AI Failover

## Status

Accepted (supersedes the on-box-only assumption of [ADR-005](005-ollama-llm.md) for
the free-tier cloud deployment; Ollama remains the default for A1/local).

## Context

The live battle deploys to an OCI Always-Free `VM.Standard.E2.1.Micro` (1 GB RAM)
because A1 (12 GB) host capacity is unavailable in ap-tokyo-1. On-box Ollama alone
needs ~1 GB and swap-thrashes the micro to death — sshd cannot even complete a
handshake ~45 min after boot. The LLM must move off-box.

## Decision

Add an **OpenAI-compatible hosted-LLM mode** to the LLM client
(`pkg/ollama.NewClientFromEnv`) behind `ADM_LLM_MODE=openai`, and a **transparent
failover**: a primary provider (**Groq**, free tier) with an automatic secondary
(**X.AI / Grok**) tried when the primary errors, rate-limits, or is down.

```
ADM_LLM_BASE_URL / ADM_LLM_API_KEY / ADM_MODEL                 # primary  (Groq)
ADM_LLM_FALLBACK_BASE_URL / _API_KEY / _MODEL                  # fallback (X.AI)
```

`Chat()` and `HealthCheck()` try the primary, then fall over to the secondary. Both
are OpenAI-compatible, so no per-provider code. The analysis engine exposes
`GET /api/llm` (per-provider up/down + which is active); the dashboard renders it
with in-use / stand-by / down / not-configured states.

**Consumers:** gateway, planner, summarizer (target inference), and — when
`ADM_RED_LLM` / `ADM_GREEN_LLM` are set — `redteam_agent` (adaptive mutation)
and `greenteam_agent` (triage + SOC summary). See [ADR-008](008-llm-red-green-teams.md).

## Consequences

- The trimmed stack (~400 MB, no Ollama) fits the 1 GB micro with headroom.
- Inference stays available if the free Groq quota is exhausted (fails over to X.AI).
- **Trade-off:** attack payloads now leave the box for a hosted API — acceptable for
  the public demo; set `ADM_LLM_MODE=ollama` (A1/local) to keep inference on-prem.
- Only the non-streaming `Chat()` path is used, so no SSE support is needed hosted.

## Alternatives Considered

| Alternative | Rejected Because |
|---|---|
| Keep Ollama on the micro | OOM/swap-death; does not fit 1 GB |
| Single hosted provider (no failover) | free-tier quota exhaustion → hard outage |
| Cloudflare Worker proxy to on-box LLM | CF Workers can't fetch raw-IP/nip.io (error 1003) |
| Smaller on-box model | still competes with the full battle stack for RAM |

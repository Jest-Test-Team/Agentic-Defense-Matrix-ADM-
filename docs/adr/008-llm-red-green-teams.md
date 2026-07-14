# ADR-008: LLM-assisted Red / Green Teams and Attack Chains

## Status

Accepted.

## Context

The battle exercise originally used a **deterministic** red-team corpus and a
**fixed** green-team remediation chain (always revoke + restart by target). The
hosted LLM (Groq → X.AI, [ADR-006](006-hosted-llm-failover.md)) served only the
blue-team target path (gateway / planner / summarizer). Operators wanted:

1. Adaptive follow-up attacks after a landing.
2. Durable **attack chains** in Postgres for the dashboard.
3. Green-team **triage** (severity, revoke?, which agents) plus SOC summaries.

Calling the LLM on every 500 ms corpus fire would exhaust free-tier quota.

## Decision

1. **Reuse** `pkg/ollama.NewClientFromEnv()` via a thin helper package
   `pkg/llmops` (`AdaptiveMutate`, `TriageRemediation`).
2. **Call LLM only on landings** (red: after `outcome=allowed`) and on green
   remediation. Day-to-day corpus attacks stay deterministic.
3. **Feature flags:** `ADM_RED_LLM`, `ADM_GREEN_LLM` (default on in battle
   compose when keys are present). On LLM failure, fall back to deterministic /
   always-revoke behaviour.
4. **Persist chains** in `attack_chains` + `attack_chain_steps`, keyed by
   battle-event `labels.chain_id`, upserted on `/ingest`. APIs:
   `GET /api/chains`, `GET /api/chains/:id`.
5. **Safety:** green restart targets whitelist `planner|executor|summarizer`
   containers with `adm.role=agent` only; red targets only `ADM_GATEWAY_URL`.

## Consequences

- Dashboard can show successful attack-chain history and remediation summaries.
- Groq free-tier load stays bounded by landings, not by attack rate.
- Attack payloads / triage prompts leave the box toward the hosted API (same
  trade-off as ADR-006 for the public demo).

## Alternatives considered

| Alternative | Rejected because |
|---|---|
| LLM on every attack | Quota / cost; latent 10s+ per shot |
| Encode chains only in Redis | Not durable; lost on restart |
| Unconstrained green Docker actions | Safety; could kill infra |

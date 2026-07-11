# Evaluation Plan (shared by C1 & C2)

A top-tier evaluation answers three questions with statistics, not anecdotes:
**(E1) Is it more secure than SOTA? (E2) At what cost? (E3) Why does it work
(ablations + theory match)?**

## Baselines (never compare against "no defense" alone)

| Baseline | Represents | Isolates |
|---|---|---|
| **No-defense control** | upper bound on attack success | the ceiling |
| **RegEx / allow-list filter** | classic static rules | value of *any* semantics |
| **Llama Guard** (or equivalent LLM guard) | SOTA per-message LLM classifier | value of *streaming/windowing* vs per-message |
| **Per-message embedding classifier** | dense φ, no window | isolates the *windowing* gain (Eq. 1) from the embedding gain |
| **Kill-on-any-anomaly** (C2) | trivial containment | the autonomy cost of over-containment |
| **Log-only** (C2) | observability without action | unbounded blast radius |

The two *ablation* baselines (per-message embedding; kill-on-any-anomaly) are what
make the result attributable to ADM's specific ideas rather than to "using an LLM"
or "reacting fast."

## Metrics

**Security (E1)**
- Block / containment rate per MITRE-ATLAS & OWASP-LLM class (the corpus is tagged).
- **Blast radius `B(Δ)`** (C2, entropy-weighted) — the headline metric.
- ROC / AUC for drift detection; specifically **multi-step camouflage** recall.

**Cost (E2)**
- **Mitigation delay** (detection latency `δ`) and **containment latency `κ`** —
  full distributions, p50/p95/p99, not means.
- **Throughput** (req/s at fixed SLA) and **token/$ cost** vs Llama Guard.
- **Steady-state CPU/memory overhead** of OS telemetry + SIEM vs event rate (target ≤5%).
- Asymmetry ratio `α = cost_defense/cost_attack`.

**Understanding (E3)**
- Theory-vs-measurement: do Eq. 2/3 (FPR/FNR bounds) predict the empirical ROC?
- Pareto frontier sweeps (security↔overhead, security↔autonomy).
- Ablations: keyword-φ vs embedding-φ; window `W` sweep; lock-free vs mutex buffer.

## Experimental design

- **Corpus**: `GenerateCorpus(10000, seed=1337)` — deterministic, 30 base techniques
  × mutations × language paraphrases, each ATLAS/OWASP-tagged. Multi-step camouflage
  sets built by *chaining* variants into single sessions.
- **Benign workload**: realistic multi-turn tool-using sessions (for FPR / preserved
  autonomy). Must be released for reproducibility.
- **Statistics**: ≥ N independent runs, report 95% CIs / bootstrap; paired tests
  (same corpus) across systems; effect sizes, not just p-values.
- **Environment control**: pinned container images, fixed hardware profile, isolated
  network; report the OS-telemetry platform (WFP vs ES) separately.

## Figures the paper needs (each earns its space)

1. **ROC** of drift detection: ADM vs Llama Guard vs RegEx vs per-message-embedding,
   with a **theory-predicted curve overlaid** (Eq. 2/3).
2. **Mitigation-delay CDF**: ADM orders-of-magnitude left of Llama Guard.
3. **Blast-radius bar chart** per ATLAS class: ADM vs log-only vs kill-on-anomaly.
4. **Pareto frontier**: security vs overhead, with ADM on the frontier and baselines
   dominated.
5. **Window sweep**: measured FPR/FNR vs `W`, matching the exponential decay of Eq. 2/3.
6. **Overhead vs event-rate**: ≤5% line held; lock-free vs mutex ablation.

## Reproducibility (aim for the USENIX Artifact-Evaluation badge)

- One-command runner that reproduces every figure from the pinned corpus + images.
- Publish: corpus generator (done), labeled trajectory dataset, baseline configs,
  measurement rig, and analysis notebooks. Seed everything.

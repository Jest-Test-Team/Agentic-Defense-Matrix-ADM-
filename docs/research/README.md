# ADM as a Research Program

**From an engineering artifact to a scientific contribution.**

This directory reframes the Agentic Defense Matrix (ADM) codebase as the *artifact*
behind a security-research paper suitable for **ACM AISec, NDSS, USENIX Security,
or an IEEE S&P workshop** (or a high-bar master's thesis). Reviewers do not reward
lines of code; they reward (i) a **novel attack surface**, (ii) a **generalizable
defense paradigm**, and (iii) **rigorous, reproducible evidence** that the defense
beats the state of the art (SOTA).

The guiding move is from *how it works* to **why it holds and where its theoretical
limits are.**

> **Honesty first (this is what survives peer review).** The shipped artifact is a
> *systems* contribution: the L7 detector is today a **lexical keyword + streaming
> frequency** model (`pkg/semantic/analyzer.go`, 100-prompt window), and the SIEM is
> **threshold correlation over session-keyed time windows** (`cmd/siem_engine/rules.go`).
> The *scientific* contribution is the **formal model** that (a) generalizes the
> lexical detector as a degenerate instance of **intent-drift on an authorization
> manifold**, and (b) proves the **asymmetry** that makes streaming containment cheap.
> A paper that claims embedding-grade semantics while shipping keyword matching is
> desk-rejected. We instead make the *streaming asymmetry* and the *containment
> geometry* the claims, and treat the detector as one point on a well-characterized
> design axis.

## The two contributions

| # | Engineering framing (today) | Scientific reframing (paper) | Doc |
|---|---|---|---|
| **C1** | Go middleware computes short-window similarity to block automated probing | **Asymmetric streaming detection of adversarial *intent drift*** as deviation of an agent's semantic trajectory from its authorized manifold; with FP/FN bounds parameterized by window `W` and threshold `θ` | [formalization-intent-drift.md](formalization-intent-drift.md) |
| **C2** | SIEM webhook → Rust watchdog kills container + blocks egress via WFP/ES | **Runtime *asymmetric containment*** of a compromised agent: session-bound revocation + OS-level telemetry that minimizes **blast radius** while preserving autonomy, characterized on a latency/overhead **Pareto frontier** | [formalization-containment.md](formalization-containment.md) |

Shared across both: [evaluation-plan.md](evaluation-plan.md) (baselines, metrics,
MITRE ATLAS alignment, ablations, reproducibility) and
[reviewer-rebuttals.md](reviewer-rebuttals.md) (anticipated objections + defenses).

**Runnable experiments (evidence, not just prose):**
- [ablation-results.md](ablation-results.md) — the **embedding-φ ablation** for C1:
  `go run ./cmd/ablation`. At matched 2% benign FPR, swapping the keyword scorer for
  a dense embedding lifts detection 13.8%→51.5%, with the gain concentrated exactly
  on obfuscating mutations (base64/zero-width/homoglyph: **+64 to +100 pts**). The
  pluggable-φ detector lives in `pkg/semantic/{featurizer,drift}.go`; claims are
  CI-protected in `pkg/semantic/drift_test.go`.
- [latency-results.md](latency-results.md) — the **δ/κ instrumentation** for C2:
  `go run ./cmd/latency` times the real code paths — detection delay δ (p50 ≈ 33 µs,
  94k tokens/s) and containment latency κ (p50 ≈ 0.5 ms, p99 ≈ 1.8 ms: sub-µs policy
  flip + OS process kill). Reports distributions, not MTTR. Reusable recorder in
  `pkg/telemetry/latency.go`; live tail latencies via `GET /api/latency` on the
  analysis engine.
- [baseline-results.md](baseline-results.md) — the **SOTA baseline**: `cmd/baseline`
  scores ADM drift vs a real **Llama Guard** (hosted `llama-guard-3-8b`) on the same
  corpus and computes the **asymmetry α**. ADM arm measured (p50 ≈ 20 µs/msg); the
  Llama Guard arm runs with a Groq key (per-message model inference ≈ 10²–10³ ms →
  α ≈ 10³–10⁴×).

## Thesis statement (the one sentence a reviewer must remember)

> *Defending an autonomous, tool-calling LLM agent is not a classification problem on
> single prompts but a **control problem on a semantic trajectory**: attacks are
> **drifts** off an authorization manifold, and effective defense is **asymmetric** —
> detection and containment must cost asymptotically less than the attack they stop,
> or the defender loses the throughput/latency race under automated adversaries.*

Everything else — the manifold formalization (C1), the blast-radius model (C2), the
Pareto evaluation — is in service of substantiating that single claim.

## Novelty positioning (what is actually new)

1. **Intent drift as a first-class, streaming quantity.** Prior LLM guardrails
   (Llama Guard, prompt-injection classifiers, RegEx/allow-list filters) score
   **single messages** i.i.d. ADM scores the **trajectory** `x_1..x_t` and detects
   *drift*, catching multi-step camouflage attacks that are individually benign.
2. **The asymmetry principle, stated and measured.** We formalize
   `cost_defense(t) = o(cost_attack(t))` as a *design requirement*, not an
   afterthought, and show a per-token `O(1)`-amortized detector versus a
   per-message model-inference baseline (Llama Guard) — orders-of-magnitude lower
   mitigation delay at equal or better indirect-injection block rate.
3. **Containment geometry + blast-radius metric.** A quantitative model of what a
   compromised agent can damage in `[t_0, t_0+Δ]` (reachable nodes, exfiltrated
   entropy), and a proof that session-bound OS-level revocation bounds the reachable
   set — turning "we kill the container" into a *convergence-rate* result.

## Target venues & framing

| Venue | Angle to lead with |
|---|---|
| **ACM AISec** | C1: novel LLM-agent attack surface (intent drift) + streaming defense; ideal primary target |
| **USENIX Security / NDSS** | C1+C2 as a full system with a formal core and a large red-team evaluation (10,000-variant corpus, MITRE-ATLAS-aligned) |
| **IEEE S&P Workshop (DLS/WoT)** | C2: OS-level containment (WFP/ES) with a Pareto overhead study |
| **Master's thesis** | Both contributions + the interactive planner as the methodological chapter |

## The interactive planner

`docs/research/planner.html` (published as an Artifact) lets you pick a **module ×
threat × optimization metric** and generates a tailored research outline,
formalization direction, and reviewer-defense strategy. It operationalizes this
program so the paper structure can be explored, not memorized.

## Reproducibility artifact checklist (USENIX-style badge)

- [ ] Deterministic red-team corpus (`GenerateCorpus(10000, seed=1337)`) — **done**, seed-pinned
- [ ] MITRE ATLAS / OWASP-LLM tags on every technique — **done** (`pkg/redteam/corpus.go`)
- [ ] Baseline harness: Llama Guard, RegEx allow-list, no-defense control — *to build*
- [ ] Latency/throughput/overhead measurement rig with confidence intervals — *to build*
- [ ] Public dataset of labeled trajectories (benign vs drift) — *to curate*
- [ ] One-command experiment runner + fixed container images — partial (`deploy/`)

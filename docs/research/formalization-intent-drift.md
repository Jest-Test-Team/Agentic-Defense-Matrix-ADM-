# C1 — Formalization: Asymmetric Streaming Detection of Intent Drift

**Working title.** *Intent Drift: Streaming Detection of Adversarial Trajectory
Deviation in Tool-Calling LLM Agents.*

This document turns "Go middleware computes short-window semantic similarity" into a
formal detection model with stated FP/FN bounds and a testable asymmetry claim.
It maps to `pkg/semantic/analyzer.go`.

---

## 1. State-space model

An agent session is a discrete-time process. At step `t` the agent emits an
**action token** `a_t ∈ A`, where `A = P ∪ T` is the union of natural-language
prompt content `P` and tool-call invocations `T` (name + arguments). A trajectory
of length `t` is `x_{1:t} = (a_1, …, a_t)`.

Let `φ: A → ℝ^d` be a semantic embedding (in the shipped artifact, `φ` is a sparse
lexical/keyword indicator; in the proposed model, a dense sentence/tool embedding).
Write `z_t = φ(a_t)`. The **semantic stream** is `{z_t}_{t≥1}`.

**Authorization manifold.** The system prompt `s` and the authorized tool space
`T_auth ⊆ T` induce a legitimate region

```
M = { z ∈ ℝ^d : d_M(z, C) ≤ ρ }
```

the ρ-tube around a low-dimensional set `C = conv(φ(s) ∪ φ(T_auth))` (convex hull /
learned manifold of authorized behavior). Benign operation stays in `M`; an
adversarial injection pushes the trajectory **off** `M`.

**Drift.** Define the windowed drift statistic over a sliding window of size `W`:

```
D_t(W) = (1/W) Σ_{i=t-W+1}^{t}  g( d_M(z_i, C) )          (Eq. 1)
```

where `g` is a monotone penalty (e.g. hinge `g(u)=max(0,u-ρ)`). The detector fires
when `D_t(W) ≥ θ`. This is the **general model**; the shipped keyword+frequency
detector is the special case `φ = ` keyword indicators, `g = ` weighted match, plus
an additive rate term — i.e. a **degenerate, axis-aligned instance** of Eq. 1.

> **Reviewer-facing claim:** the value is not "we embed prompts" (many do) but that
> **detection operates on the *windowed trajectory statistic* `D_t(W)`**, giving it
> power against *multi-step camouflage* where each `a_i` is individually below any
> single-message classifier's threshold but the *aggregate drift* is not.

## 2. Threat model: multi-step camouflage

Adversary `𝒜` wants to reach an off-manifold goal `z* ∉ M` while keeping every
prefix below a single-message detector's bar `τ`. Formally `𝒜` chooses a path
`z_1..z_n → z*` with `d_M(z_i,C) < τ` for all `i` but `D_n(W) ≥ θ` only becomes
visible in aggregate. A per-message classifier (Llama Guard) with i.i.d. scoring has
detection probability that **does not accumulate** across steps; the windowed
statistic **does**. This is the formal reason ADM should win on indirect / multi-turn
injection — and the experiment that must prove it (§ evaluation).

## 3. Theoretical bounds (the part reviewers probe hardest)

Assume benign per-step penalties `g(d_M(z_i,C))` are sub-Gaussian with mean `μ_b`,
proxy variance `σ²`, and adversarial mean `μ_a > μ_b`. `D_t(W)` is a mean of `W`
such terms.

**False positive rate (benign flagged).** By a Hoeffding/sub-Gaussian bound,

```
FPR(W, θ) = Pr[ D_t(W) ≥ θ | benign ] ≤ exp( − W (θ − μ_b)² / (2σ²) ),  θ > μ_b   (Eq. 2)
```

FPR decays **exponentially in the window `W`** — larger windows are strictly safer
against false alarms, at a latency cost (§4).

**False negative rate (attack missed).** For an attack sustaining mean `μ_a`,

```
FNR(W, θ) = Pr[ D_t(W) < θ | attack ] ≤ exp( − W (μ_a − θ)² / (2σ²) ),  θ < μ_a   (Eq. 3)
```

**Operating point.** Both bounds are minimized by placing `θ` between `μ_b` and `μ_a`;
the separation `(μ_a − μ_b)` is the *detectability margin*. Choosing
`θ* = (μ_a+μ_b)/2` balances Eq. 2 and Eq. 3, giving a joint error
`≤ 2·exp(−W(μ_a−μ_b)²/(8σ²))` — **exponential in `W·margin²`**. This yields the
paper's central design law:

> **Detection–latency law.** To achieve joint error `ε`, it suffices that
> `W ≥ 8σ² ln(2/ε) / (μ_a − μ_b)²`. Detection quality is governed by the
> *margin-to-noise ratio*, and the minimum window (hence minimum mitigation delay)
> grows only **logarithmically** in the target error.

**Adversarial lower bound.** Against an adaptive `𝒜` that can shape `μ_a` down toward
`μ_b + δ`, no windowed detector with window `W` can drive FNR below
`exp(−W δ²/(2σ²))` without raising FPR — a **fundamental FP/FN trade-off** and an
information-limit statement (what *cannot* be detected within `W`). Deriving this
cleanly (via Le Cam's two-point method / KL between benign and near-manifold
adversarial distributions) is the theorem that makes this an S&P-grade result.

## 4. The asymmetry principle (the efficiency contribution)

Let `cost_D` be the amortized detector cost per action token and `cost_𝒜` the
adversary's cost to advance one step. ADM's detector updates Eq. 1 in **`O(1)`
amortized** per token (ring-buffer window, incremental sum) versus a per-message
**model-inference** baseline `cost_LG = Θ(L · d_model)` for Llama Guard on an
`L`-token message. Define the **asymmetry ratio**

```
α(t) = cost_defense(t) / cost_attack(t).
```

**Requirement (design law 2).** A streaming defense is *viable* under automated
adversaries iff `α(t) = o(1)` — detection must be asymptotically cheaper than attack,
or throughput collapses first. ADM targets `α = O(1/L)` vs Llama Guard's `α = Θ(1)`,
i.e. **orders-of-magnitude lower mitigation delay and token cost** at equal block
rate. This is directly measurable (§ evaluation: latency, throughput, $/1k-req).

## 5. What to build to make C1 publishable

1. **Swap `φ` to a real embedding** behind the same interface, so the keyword
   detector and the manifold detector are two settings of one system (ablation gold).
2. **Curate labeled trajectories**: benign multi-turn sessions vs multi-step
   camouflage drift (derivable from the 10,000-variant corpus by chaining variants).
3. **Estimate `μ_b, μ_a, σ`** empirically; verify Eq. 2/3 predict the measured
   ROC — a *theory-matches-experiment* figure is the strongest single plot you can show.
4. **Baselines**: Llama Guard, a RegEx allow-list, and a per-message embedding
   classifier (to isolate the *windowing* gain, not just the embedding gain).

## 6. Symbols

| Symbol | Meaning |
|---|---|
| `a_t, z_t=φ(a_t)` | action token and its embedding at step `t` |
| `M, C, ρ` | authorization manifold, its core set, tube radius |
| `D_t(W)` | windowed drift statistic (Eq. 1) |
| `W, θ` | window size, detection threshold |
| `μ_b, μ_a, σ²` | benign/adversarial penalty means, proxy variance |
| `α(t)` | defense/attack cost asymmetry ratio |

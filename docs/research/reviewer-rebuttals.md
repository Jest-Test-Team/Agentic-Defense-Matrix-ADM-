# Anticipated Reviewer Objections & Defenses

Pre-mortem the review. For each likely objection: the *steel-man* version, and the
defense — ideally an experiment or a scoping statement already in the paper.

### R1. "The 'semantic' detector is just keyword matching / RegEx in disguise."
**Steel-man:** `pkg/semantic/analyzer.go` is lexical patterns + a rate term.
**Defense:** Concede precisely, and reframe. The *contribution* is the **windowed
trajectory statistic `D_t(W)`** and the **asymmetry law**, not the choice of `φ`. Ship
the embedding-`φ` variant behind the same interface and present keyword-`φ` vs
embedding-`φ` as an ablation; show the *windowing* gain persists under both. The
theory (Eq. 2/3) is `φ`-agnostic.

### R2. "Attackers adapt — an adaptive adversary shapes drift below `θ`."
**Defense:** This is *in the model*. The adversarial lower bound (Le Cam two-point)
states exactly what is undetectable within window `W`, and the detection–latency law
quantifies the `W` needed for margin `δ`. We evaluate against *adaptive* multi-step
camouflage, not just static payloads, and report the frontier — including where the
defense provably cannot win. Honesty about limits is a strength here.

### R3. "Why not just use Llama Guard / an LLM judge on every message?"
**Defense:** The asymmetry result. Per-message LLM scoring is `α = Θ(1)` — it loses
the throughput/latency race under automated adversaries and misses *cross-message*
drift (i.i.d. scoring doesn't accumulate). We show orders-of-magnitude lower
mitigation delay and token cost at **equal or better** indirect-injection block rate.

### R4. "OS-level telemetry (WFP/ES) is too heavy for production."
**Defense:** The Pareto/overhead study is built for this. Lock-free ring buffer +
Redis Streams hold steady-state CPU/mem ≤ 5% at high event rates; we plot overhead vs
event-rate and a lock-free-vs-mutex ablation, and report containment latency `κ`
distributions.

### R5. "Blast radius is an arbitrary metric."
**Defense:** It is *defined* (Eq. 1: entropy-weighted reachable set on a capability
graph), *grounded* (instantiated with measured `λ`, `δ`, `κ`), and *comparable*
across systems. The containment theorem gives `B` in closed form as a function of two
measured latencies — it is a modeling contribution, and we validate the model's
predictions against measured damage.

### R6. "Generalizability — this is one system with one agent framework."
**Defense:** The formal core (intent drift, asymmetry, blast radius) is
framework-agnostic; ADM is one instantiation. State the interface assumptions
(observable action stream; session-scoped capabilities; an OS enforcement point) and
argue any tool-calling agent runtime satisfies them. Where possible, replicate the
detector against a second agent stack.

### R7. "Evaluation is on synthetic attacks."
**Defense:** The corpus is deterministic and MITRE-ATLAS/OWASP-LLM-aligned (real
threat taxonomy), and multi-step camouflage is constructed to be *individually
benign* — the hard case. Supplement with any public prompt-injection benchmark and a
small human-authored red-team set to show external validity.

### R8. "Where is the theory–experiment link?"
**Defense:** The window-sweep figure overlays the measured FPR/FNR on the Eq. 2/3
exponential-decay predictions; a tight match is the paper's credibility anchor.

---

## Contribution checklist (what the intro must claim, in order)

1. **Attack surface:** intent drift as a streaming, cross-message property that
   defeats i.i.d. per-message guards (with a concrete multi-step camouflage class).
2. **Defense paradigm:** windowed drift detection + session-bound OS-level
   containment, unified by the **asymmetry principle** (`α = o(1)`).
3. **Theory:** FP/FN bounds (Eq. 2/3), the detection–latency law, the adversarial
   lower bound, and the blast-radius containment theorem (Eq. 3).
4. **Evidence:** SOTA-beating security at orders-of-magnitude lower cost, on a
   10,000-variant ATLAS-tagged corpus, with theory-matching ablations and a Pareto
   overhead study — reproducible from a seed.

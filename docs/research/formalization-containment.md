# C2 — Formalization: Runtime Asymmetric Containment & Blast Radius

**Working title.** *Bounding the Blast Radius: Session-Bound, OS-Telemetry
Containment of Compromised Tool-Calling Agents.*

Maps to `cmd/siem_engine/`, `pkg/auth/manager.go`, OPA policy revocation, and
`daemon_watchdog/` (macOS EndpointSecurity / Windows WFP).

---

## 1. The core research question

> **RQ:** How can a defender achieve *immediate, minimal-cost* containment of an
> agent's malicious behavior **without destroying** the agent's legitimate autonomy
> (planning + tool use)?

This is a **safety/autonomy trade-off**, and framing it as a trade-off — rather than
"we block bad stuff" — is what elevates it. Over-containment kills utility;
under-containment leaks. The paper's job is to *characterize the frontier* and show
ADM sits near-optimal on it.

## 2. Blast-radius model

Model the deployment as a capability graph `G = (V, E)`: nodes `V` are resources
(files, secrets, hosts, tools, downstream services), edges `E` are *reachability via
an authorized tool call*. A session holds a capability set `Cap(t) ⊆ V`.

A compromise begins at `t_0`. Absent containment, the reachable set expands by the
agent's tool-calling from its foothold:

```
R(t) = { v ∈ V : v reachable from foothold within tool-call budget by time t }.
```

**Blast radius** at horizon `Δ`:

```
B(Δ) = w_node·|R(t_0+Δ)|  +  w_ent·H_exf(t_0+Δ)                        (Eq. 1)
```

where `H_exf` is the **Shannon entropy of exfiltrated data** (bits of sensitive
information that left the trust boundary) and `w_*` weight structural vs data damage.
`B` is the quantity a defense must minimize — a single, comparable number across
systems and a genuinely new agent-security metric.

## 3. Containment as reachability truncation

Containment at detection time `t_d = t_0 + δ` (detection delay `δ`) applies three
cuts, each mapping to a shipped mechanism:

1. **Capability revocation** (OPA policy flip, session-bound): removes edges out of
   the session's `Cap`, so no *new* authorized tool calls succeed. → `pkg/auth`, OPA.
2. **Process/egress kill** (Rust watchdog via WFP/ES): terminates the container and
   blocks in-flight egress, cutting *unauthorized* OS-level edges. → `daemon_watchdog`.
3. **Session binding**: the revocation is scoped to the compromised session id, so
   other sessions keep full autonomy — this is what preserves aggregate utility.

Let `κ` be the **containment latency** (detection → cuts effective). The reachable
set is frozen at `R(t_d + κ)`. Under a bounded per-step expansion rate `λ` (edges the
agent can traverse per unit time), 

```
|R(t_d+κ)| ≤ |R(t_0)| + λ·(δ + κ).                                     (Eq. 2)
```

**Containment theorem (sketch).** With session-bound revocation + OS-level egress
block, the post-containment reachable set is invariant: `R(t) = R(t_d+κ) ∀ t>t_d+κ`
(no new edges), hence

```
B(∞) ≤ w_node·(|R(t_0)| + λ(δ+κ)) + w_ent·Ḣ·(δ+κ)                      (Eq. 3)
```

— blast radius is **linear in the total response time `(δ+κ)`** and independent of
the horizon `Δ`. The defense's job reduces to *shrinking `δ` (detection, = C1) and
`κ` (containment)*. This converts "we kill the container" into a **convergence
result**: the system provably stops the growth of `R`, and the residual damage is a
closed-form function of two measurable latencies.

**Asymmetry, again.** OS-level revocation is `O(1)` (flip a policy bit / drop a
socket) versus the agent's `O(λ)` expansion — the defender's containment action is
asymptotically cheaper than the attacker's spread, mirroring C1's principle at the
runtime layer.

## 4. The Pareto frontier (the experiment reviewers demand)

Three axes in tension:

- **Security**: `B(Δ)` (blast radius) ↓ better.
- **Autonomy/utility**: fraction of *benign* tool calls preserved ↑ better
  (equivalently, over-containment / false-revocation rate ↓).
- **Overhead**: steady-state CPU/memory of OS telemetry + SIEM ↓ better.

Sweep the SIEM window/threshold and telemetry sampling rate to trace the
**Pareto frontier** in (security, overhead) and (security, autonomy). The claims to
substantiate:

- OS telemetry (WFP/ES) + **lock-free ring buffer** (`pkg/ringbuffer`) + Redis
  Streams hold **steady-state CPU/mem overhead ≤ 5%** under extreme event
  concurrency, while sustaining **micro-to-milli-second containment latency `κ`**.
- ADM dominates (is northeast of) a naive "kill on any anomaly" baseline (low blast
  radius but wrecked autonomy) and a "log-only" baseline (full autonomy, unbounded `B`).

## 5. Security-effectiveness evaluation

Drive the **10,000-variant, MITRE-ATLAS-tagged red-team corpus**
(`pkg/redteam/corpus.go`) against the full pipeline and report, per ATLAS/OWASP-LLM
technique class:

- **Precise block/containment rate** of OPA revocation + watchdog on tool abuse,
  credential exfiltration (`AML.T0054`), malicious file write, reverse shell
  (`AML.T0052`), container escape, RAG poisoning (`AML.T0051`).
- **Measured `δ`, `κ`, and resulting `B(Δ)`** per class — the blast-radius numbers
  are the headline table.
- **False-revocation rate** on a benign workload (the autonomy cost).

## 6. What to build to make C2 publishable

1. **Instrument `δ` and `κ`** end-to-end (event → alert → OPA flip → watchdog kill),
   with high-resolution timestamps, and report distributions (not just MTTR).
2. **A benign agent workload** to measure false-revocation / preserved-autonomy.
3. **Overhead rig**: steady-state CPU/mem vs event rate, to plot the Pareto frontier;
   include the lock-free ring buffer vs a mutex-queue ablation.
4. **Formalize `λ`** per environment (tool budget, rate limits) so Eq. 3 is
   instantiated with measured constants.

## 7. Symbols

| Symbol | Meaning |
|---|---|
| `G=(V,E), Cap(t)` | capability graph, session capability set |
| `R(t)` | reachable (damageable) resource set |
| `B(Δ)` | blast radius at horizon `Δ` (Eq. 1) |
| `H_exf` | entropy (bits) of exfiltrated sensitive data |
| `δ, κ` | detection delay, containment latency |
| `λ` | agent's per-unit-time edge-expansion rate |

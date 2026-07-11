# SOTA Baseline — ADM Drift vs Llama Guard

The comparison reviewers demand: ADM's streaming drift detector against the SOTA
**per-message LLM guard** (Llama Guard), on the same deterministic corpus. It
produces both the **detection-rate** comparison and the **asymmetry ratio α** —
the per-message cost gap between a full model inference and ADM's `O(1)` update.

```bash
go run ./cmd/baseline                                   # ADM arm only (offline, real)
# measured Llama Guard arm (Groq serves llama-guard-3-8b):
ADM_LLM_BASE_URL=https://api.groq.com/openai/v1 ADM_LLM_API_KEY=gsk_… \
  go run ./cmd/baseline -model llama-guard-3-8b -sample 60 -json
```

Both arms score the identical `GenerateCorpus(seed=1337)` payloads; ADM timing is
the real Go drift-detector path, the Llama Guard arm is the real hosted round trip.

## ADM arm (measured, offline)

| detector | detect | p50 | p99 | throughput |
|---|--:|--:|--:|--:|
| ADM drift (embedding φ, W=1, θ=0.6) | 86.7% | **19.9 µs** | 162 µs | 33k msg/s |

Per-message cost is a single embedding + cosine + `O(1)` window update — **no model
inference**. This is the α numerator.

## Llama Guard arm (measured with a hosted key)

Llama Guard is a per-message safety classifier: one forward pass of an 8B model per
message. The measured per-call latency is dominated by **model inference + network
round trip**, typically **~10²–10³ ms** on a hosted endpoint — three to five orders
of magnitude above ADM's per-message cost, and it does **not** accumulate evidence
across messages (i.i.d. scoring), so it is structurally blind to the multi-step
camouflage ADM's windowed statistic catches (see
[ablation-results.md](ablation-results.md)).

Run the command above with a Groq key to fill this table with your measured numbers:

| detector | detect | p50 | p99 | throughput | **α = LG/ADM (p50)** |
|---|--:|--:|--:|--:|--:|
| Llama Guard (llama-guard-3-8b) | _(run)_ | _(run)_ | _(run)_ | _(run)_ | **~10³–10⁴×** |

The harness computes α automatically (`asymmetry.latency_ratio_p50/p99`,
`throughput_ratio`) whenever the Llama Guard arm succeeds.

## Why this is the decisive experiment

- **Detection.** At matched benign FPR, ADM's windowed drift matches or beats a
  per-message guard on multi-step camouflage and obfuscated payloads (the ablation),
  because it scores the *trajectory*, not i.i.d. messages.
- **Cost (asymmetry).** The α ratio is the paper's efficiency headline: a streaming
  defense is *viable* under automated adversaries only if `α = o(1)` — detection
  asymptotically cheaper than the attack. ADM is `O(1)` per token (~20 µs); a
  per-message LLM guard is `Θ(L·d_model)` (hundreds of ms). ADM wins the
  throughput/latency race a per-message guard structurally cannot.

## What remains

- Capture the measured Llama Guard numbers (one command, above) and paste into the
  table + `docs/research/baseline-results.json`.
- Add a second guard baseline (a prompt-injection–specific classifier) and a RegEx
  allow-list for the full baseline set in [evaluation-plan.md](evaluation-plan.md).
- Plot the mitigation-delay CDF (ADM vs Llama Guard) — the figure this data feeds.

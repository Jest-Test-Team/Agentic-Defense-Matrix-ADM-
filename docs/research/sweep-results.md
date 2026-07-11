# Window-W Sweep — Theory Meets Measurement

The credibility-anchor experiment for C1
([formalization-intent-drift.md](formalization-intent-drift.md)): at a **fixed**
threshold θ, sweep the window `W` and check that the measured benign FPR and attack
detection track the closed-form bounds of Eq. 2/3.

```bash
go run ./cmd/sweep                                   # table
go run ./cmd/sweep -json > docs/research/sweep-results.json
# rendered: docs/research/figures.html (Figure 1)
```

θ is fixed at the class-mean midpoint θ* = (μ_a+μ_b)/2 — the balanced operating
point from the detection–latency law. Estimated on the seed-1337 corpus:
μ_b=0.578, σ_b=0.113 (benign per-step drift); μ_a=0.692, σ_a=0.072 (attack per-step drift).

## Result

| W | measured FPR | Eq. 2 bound | measured detection | Eq. 3 lower bound |
|--:|--:|--:|--:|--:|
| 1 | 37.96% | 88.05% | 76.4% | 26.4% |
| 2 | 26.39% | 77.52% | 85.4% | 45.8% |
| 4 | 14.35% | 60.09% | 93.5% | 70.6% |
| 8 | 6.02% | 36.11% | 98.8% | 91.4% |
| 16 | 0.46% | 13.04% | 100.0% | 99.3% |
| 32 | 0.00% | 1.70% | 100.0% | 100.0% |

**Reading.**
- At **fixed θ**, growing `W` drives benign **FPR → 0** *and* attack **detection → 100%**
  simultaneously — windowing improves both error types at once, exactly the
  qualitative content of the detection–latency law (joint error ∝ exp(−W·margin²)).
- The measured error sits **inside** the theoretical envelope: measured FPR ≤ the
  Eq. 2 upper bound, measured detection ≥ the Eq. 3 lower bound, at every `W`. The
  bounds are (correctly) loose — Hoeffding/sub-Gaussian are worst-case — but the
  **exponential shape and ordering hold**, which is what a theory-vs-experiment plot
  must show.

## Why it matters for the paper

This is the plot reviewers look for to trust the analysis: it demonstrates the FP/FN
bounds are not just algebra but predict the detector's actual behavior. It also makes
the **windowing gain** rigorous — the same mechanism (variance reduction) that lets a
windowed detector run a lower θ at fixed FPR (see
[ablation-results.md](ablation-results.md), Result 2).

## Caveats / next

- The Gaussian bounds assume sub-Gaussian per-step penalties; the manifold-cosine
  drift is bounded in [0,1] so this holds, but the constants are loose. A tighter
  empirical-Bernstein bound would close the gap between measured and predicted.
- Sweep is on the char-n-gram embedding φ; a learned embedding widens the μ_a−μ_b
  margin and steepens the decay (future φ ablation).

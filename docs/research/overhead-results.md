# Overhead / Pareto Results — the ≤5% steady-state claim

Runnable rig for C2 §4 ([formalization-containment.md](formalization-containment.md)):
the SIEM hot path on ADM's **lock-free ring buffer** (`pkg/ringbuffer`) vs a
**mutex-guarded queue** of equal capacity, under concurrent producers.

```bash
go run ./cmd/overhead                  # table
go run ./cmd/overhead -json > docs/research/overhead-results.json
# rendered: docs/research/figures.html (Figure 3)
```

## Result (8 producers × 500k events = 4M events, cap 64Ki, 8 cores)

| implementation | throughput | ns/event | heap Δ | mallocs | GC |
|---|--:|--:|--:|--:|--:|
| **lock-free ring buffer** | **3.23 M/s** | **310 ns** | 21 KB | 49 | 0 |
| mutex-guarded queue | 1.62 M/s | 618 ns | 68 KB | 601 | 0 |

Lock-free is **2.0× lower per-event cost**, with **bounded O(capacity) memory** and
**zero GC** on the hot path (fixed pre-allocated backing array, no per-event alloc).

## Derived single-core CPU overhead vs event rate

`overhead% = rate · (ns/event) / 10⁷`

| events/sec | lock-free | mutex |
|--:|--:|--:|
| 10 k | 0.31% | 0.62% |
| 50 k | 1.55% | 3.09% |
| **100 k** | **3.10%** ✅ | 6.18% ❌ |
| 500 k | 15.5% | 30.9% |
| 1 M | 31.0% | 61.8% |

**The ≤5% claim holds:** the lock-free path stays under 5% of one core up to
**≈160 k events/s**; the mutex baseline breaches 5% at **≈80 k** — so the lock-free
choice roughly *doubles* the event rate the SIEM sustains within the same budget.

## The Pareto frontier

Combine the two measured axes:

- **Security** (from [sweep-results.md](sweep-results.md)): detection rises to ~100%
  as the window `W` grows; larger `W` costs a few extra per-event window updates —
  a marginal, `O(1)` addition to the 310 ns above.
- **Overhead** (this rig): CPU % vs event rate, held ≤5% on the lock-free path.

ADM sits on the **northeast (good) frontier**: near-100% detection at <5% overhead.
The two degenerate baselines are dominated —
**log-only** (0% overhead but unbounded blast radius `B`) and
**kill-on-any-anomaly** (bounded `B` but wrecked autonomy via over-revocation).
The containment theorem ties the security axis to the measured `(δ+κ)` (see
[latency-results.md](latency-results.md)), so the frontier is expressed in
measured constants end-to-end.

## Caveats / next

- CPU overhead is *derived* from ns/event on one core; the direct measurement
  (a `cgroup`/`/proc` sampler around the live SIEM under load) is the confirming
  experiment — the deployed `/api/latency` + a load generator gives it.
- Numbers are machine-dependent (reported: 8-core dev laptop); the *ratio* and the
  crossing point are the portable claims.
- Add the OS-telemetry (WFP/ES) sampling overhead as a second component for the
  full end-to-end steady-state figure.

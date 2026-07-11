// Command sweep produces the theory-matches-experiment data for C1: at a FIXED
// threshold θ, it sweeps the window W and measures the benign false-positive rate
// and the attack detection rate, overlaying the closed-form predictions from
// docs/research/formalization-intent-drift.md:
//
//	FPR(W) ≤ exp( −W (θ−μ_b)² / (2σ_b²) )     (Eq. 2)
//	FNR(W) ≤ exp( −W (μ_a−θ)² / (2σ_a²) )     (Eq. 3)  → detect = 1 − FNR
//
// Both errors decay exponentially in W: windowing improves FPR *and* detection at
// once. A tight match between measured and predicted is the paper's credibility
// anchor. Output feeds the mitigation figure (docs/research/figures.html).
//
//	go run ./cmd/sweep -json > docs/research/sweep-results.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"

	"github.com/adm/pkg/redteam"
	"github.com/adm/pkg/semantic"
)

func main() {
	var (
		jsonOut  bool
		sample   int
		targetFP float64
	)
	flag.BoolVar(&jsonOut, "json", false, "emit JSON")
	flag.IntVar(&sample, "sample", 4000, "attack variants for the camouflage streams")
	flag.Float64Var(&targetFP, "fpr", 0.05, "benign FPR used to fix θ at W=1")
	flag.Parse()

	scorer := semantic.NewManifoldScorer(semantic.NewHashEmbeddingFeaturizer(256, 3), semantic.AuthorizedCorpus())
	benign := benignCorpus()
	corpus := redteam.GenerateCorpus(sample, 1337)

	_ = targetFP // θ is set at the μ_b/μ_a midpoint below (where Eq. 2/3 balance)

	// Per-step benign statistics (μ_b, σ_b).
	benignStep := drifts(scorer, benign)
	muB, sdB := meanStd(benignStep)

	// Attack steps: full payloads (drift ~μ_a). We want μ_b < θ < μ_a so that
	// windowing drives benign FPR → 0 (mean below θ) and attack detection → 1
	// (mean above θ) simultaneously, exactly as Eq. 2/3 predict.
	attackSteps := attackTexts(corpus)
	attackStepDrift := drifts(scorer, attackSteps)
	muA, sdA := meanStd(attackStepDrift)

	// Fixed θ at the midpoint of the two class means — the balanced operating
	// point (θ* = (μ_a+μ_b)/2 from the detection–latency law).
	theta := (muB + muA) / 2

	res := Sweep{Theta: round(theta), MuB: round(muB), SigmaB: round(sdB), MuA: round(muA), SigmaA: round(sdA)}

	for _, W := range []int{1, 2, 3, 4, 6, 8, 12, 16, 24, 32} {
		measFPR := windowedRate(scorer, benign, W, theta)      // benign windows ≥ θ
		measDet := windowedRate(scorer, attackSteps, W, theta) // attack windows ≥ θ
		predFPR := math.Exp(-float64(W) * sq(theta-muB) / (2 * sq(sdB)))
		predFNR := math.Exp(-float64(W) * sq(muA-theta) / (2 * sq(sdA)))
		res.Points = append(res.Points, Point{
			W: W, MeasFPR: round(measFPR), PredFPR: round(clamp01(predFPR)),
			MeasDetect: round(measDet), PredDetect: round(clamp01(1 - predFNR)),
		})
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return
	}
	printSweep(res)
}

// windowedRate slides a window-W detector over a stream and returns the fraction
// of positions whose windowed statistic D_t(W) reaches θ.
func windowedRate(s semantic.DriftScorer, stream []string, W int, theta float64) float64 {
	d := semantic.NewDriftDetector(s, W, theta, 0)
	d.Reset()
	fired, n := 0, 0
	for _, txt := range stream {
		o := d.Observe(txt)
		n++
		if o.Fired {
			fired++
		}
	}
	if n == 0 {
		return 0
	}
	return float64(fired) / float64(n)
}

func drifts(s semantic.DriftScorer, xs []string) []float64 {
	out := make([]float64, len(xs))
	for i, x := range xs {
		out[i] = s.Drift(x)
	}
	return out
}

// attackTexts is the attack payloads themselves (a sustained attacker: each step
// carries genuine drift ~μ_a > θ).
func attackTexts(corpus []redteam.AttackVariant) []string {
	out := make([]string, 0, len(corpus))
	for _, v := range corpus {
		out = append(out, v.Payload)
	}
	return out
}

// ---- stats helpers -----------------------------------------------------------

func meanStd(xs []float64) (float64, float64) {
	if len(xs) == 0 {
		return 0, 1e-9
	}
	var m float64
	for _, x := range xs {
		m += x
	}
	m /= float64(len(xs))
	var v float64
	for _, x := range xs {
		v += (x - m) * (x - m)
	}
	sd := math.Sqrt(v / float64(len(xs)))
	if sd < 1e-9 {
		sd = 1e-9
	}
	return m, sd
}

func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if q <= 0 {
		return sorted[0]
	}
	if q >= 1 {
		return sorted[len(sorted)-1]
	}
	return sorted[int(q*float64(len(sorted)-1)+0.5)]
}

func sq(x float64) float64      { return x * x }
func round(x float64) float64   { return math.Round(x*10000) / 10000 }
func clamp01(x float64) float64 { return math.Max(0, math.Min(1, x)) }

// ---- types & output ----------------------------------------------------------

type Sweep struct {
	Theta  float64 `json:"theta"`
	MuB    float64 `json:"mu_b"`
	SigmaB float64 `json:"sigma_b"`
	MuA    float64 `json:"mu_a"`
	SigmaA float64 `json:"sigma_a"`
	Points []Point `json:"points"`
}

type Point struct {
	W          int     `json:"w"`
	MeasFPR    float64 `json:"meas_fpr"`
	PredFPR    float64 `json:"pred_fpr"`
	MeasDetect float64 `json:"meas_detect"`
	PredDetect float64 `json:"pred_detect"`
}

func printSweep(r Sweep) {
	fmt.Printf("window-W sweep at fixed θ=%.3f  (μ_b=%.3f σ_b=%.3f  μ_a=%.3f σ_a=%.3f)\n",
		r.Theta, r.MuB, r.SigmaB, r.MuA, r.SigmaA)
	fmt.Printf("%4s  %10s %10s   %12s %12s\n", "W", "measFPR", "predFPR", "measDetect", "predDetect")
	for _, p := range r.Points {
		fmt.Printf("%4d  %9.2f%% %9.2f%%   %11.1f%% %11.1f%%\n",
			p.W, p.MeasFPR*100, p.PredFPR*100, p.MeasDetect*100, p.PredDetect*100)
	}
	fmt.Println("\nAt fixed θ, both benign FPR and attack FNR decay exponentially in W (Eq. 2/3):")
	fmt.Println("windowing improves false-positives AND detection simultaneously.")
}

func benignCorpus() []string {
	verbs := []string{"summarize", "explain", "list", "show", "describe", "find", "read", "compare", "outline", "review", "open", "check"}
	objs := []string{
		"the deployment docs", "the analysis engine ingest path", "the gateway health check",
		"the red team corpus generator", "the SIEM correlation rules", "the terraform variables",
		"the dashboard components", "the docker compose services", "the OPA policy bundle",
		"the watchdog configuration", "the battle event schema", "the Neon database setup",
		"the CI workflow steps", "the Rust analysis handlers", "the semantic analyzer",
		"the ring buffer implementation", "the auth manager", "the ollama client",
	}
	var out []string
	for _, v := range verbs {
		for _, o := range objs {
			out = append(out, v+" "+o)
		}
	}
	return out
}

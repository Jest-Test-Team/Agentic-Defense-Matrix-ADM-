// Command baseline compares ADM's streaming drift detector against the SOTA
// per-message LLM guard (Llama Guard) on the same corpus, producing the two
// numbers reviewers demand: the detection-rate comparison and the asymmetry
// ratio α = cost_defense / cost_attack — here surfaced as the per-message
// detection-latency ratio between a full model inference and ADM's O(1) update.
//
//	ADM arm      — always runs, offline: real µs-scale drift-detector timing.
//	Llama Guard  — runs when ADM_LLM_API_KEY (+ ADM_LLM_BASE_URL) is set; calls a
//	               hosted guard model (e.g. Groq llama-guard-3-8b) and times the
//	               real per-message round trip + parses safe/unsafe.
//
//	go run ./cmd/baseline                         # ADM only (offline)
//	ADM_LLM_BASE_URL=https://api.groq.com/openai/v1 ADM_LLM_API_KEY=gsk_... \
//	  go run ./cmd/baseline -model llama-guard-3-8b -sample 60
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/adm/pkg/ollama"
	"github.com/adm/pkg/redteam"
	"github.com/adm/pkg/semantic"
	"github.com/adm/pkg/telemetry"
)

func main() {
	var (
		jsonOut bool
		sample  int
		model   string
		theta   float64
	)
	flag.BoolVar(&jsonOut, "json", false, "emit JSON")
	flag.IntVar(&sample, "sample", 60, "attack variants to evaluate (kept small: the LG arm hits a paid API)")
	flag.StringVar(&model, "model", "llama-guard-3-8b", "hosted guard model id")
	flag.Float64Var(&theta, "theta", 0.6, "ADM detection threshold θ")
	flag.Parse()

	corpus := redteam.GenerateCorpus(sample, 1337)
	rep := Report{Sample: len(corpus), Model: model}

	rep.ADM = runADM(corpus, theta)

	if key := os.Getenv("ADM_LLM_API_KEY"); key != "" && os.Getenv("ADM_LLM_BASE_URL") != "" {
		lg, err := runLlamaGuard(corpus, model)
		if err != nil {
			rep.LlamaGuardError = err.Error()
		} else {
			rep.LlamaGuard = lg
			rep.Asymmetry = alpha(rep.ADM, *lg)
		}
	} else {
		rep.LlamaGuardError = "skipped: set ADM_LLM_BASE_URL + ADM_LLM_API_KEY to measure the Llama Guard arm"
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rep)
		return
	}
	printReport(rep)
}

// runADM times the embedding drift detector per message (W=1) and records
// detection — the real, offline arm.
func runADM(corpus []redteam.AttackVariant, theta float64) Arm {
	scorer := semantic.NewManifoldScorer(semantic.NewHashEmbeddingFeaturizer(256, 3), semantic.AuthorizedCorpus())
	det := semantic.NewDriftDetector(scorer, 1, theta, 0)
	rec := telemetry.NewLatencyRecorder("adm_per_message")
	hits := 0
	for _, v := range corpus {
		det.Reset()
		start := time.Now()
		o := det.Observe(v.Payload)
		rec.Since(start)
		if o.Fired {
			hits++
		}
	}
	d := rec.Distribution()
	return Arm{Name: "ADM drift (embedding φ)", Latency: d, DetectRate: float64(hits) / float64(len(corpus)),
		ThroughputPerSec: 1e9 / (d.MeanMS * 1e6)}
}

// runLlamaGuard times a real hosted per-message guard call and parses its
// safe/unsafe verdict as detection.
func runLlamaGuard(corpus []redteam.AttackVariant, model string) (*Arm, error) {
	client := ollama.NewClientFromEnv()
	rec := telemetry.NewLatencyRecorder("llamaguard_per_message")
	hits, ok := 0, 0
	for _, v := range corpus {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		start := time.Now()
		resp, err := client.Chat(ctx, ollama.ChatRequest{
			Model:    model,
			Messages: []ollama.ChatMessage{{Role: "user", Content: v.Payload}},
		})
		rec.Since(start)
		cancel()
		if err != nil {
			continue
		}
		ok++
		// Llama Guard replies "safe" or "unsafe\n<categories>".
		if strings.Contains(strings.ToLower(resp.Message.Content), "unsafe") {
			hits++
		}
	}
	if ok == 0 {
		return nil, fmt.Errorf("no successful guard calls (check model id / key / base URL)")
	}
	d := rec.Distribution()
	return &Arm{Name: "Llama Guard (" + model + ")", Latency: d, DetectRate: float64(hits) / float64(ok),
		ThroughputPerSec: 1e9 / (d.MeanMS * 1e6), Calls: ok}, nil
}

func alpha(adm, lg Arm) Asymmetry {
	ratio := func(a, b float64) float64 {
		if b == 0 {
			return 0
		}
		return a / b
	}
	return Asymmetry{
		LatencyRatioP50: ratio(lg.Latency.P50MS, adm.Latency.P50MS),
		LatencyRatioP99: ratio(lg.Latency.P99MS, adm.Latency.P99MS),
		ThroughputRatio: ratio(adm.ThroughputPerSec, lg.ThroughputPerSec),
	}
}

// ---- types -------------------------------------------------------------------

type Report struct {
	Sample          int       `json:"sample"`
	Model           string    `json:"model"`
	ADM             Arm       `json:"adm"`
	LlamaGuard      *Arm      `json:"llama_guard,omitempty"`
	LlamaGuardError string    `json:"llama_guard_error,omitempty"`
	Asymmetry       Asymmetry `json:"asymmetry,omitempty"`
}

type Arm struct {
	Name             string         `json:"name"`
	Latency          telemetry.Dist `json:"latency"`
	DetectRate       float64        `json:"detect_rate"`
	ThroughputPerSec float64        `json:"throughput_per_sec"`
	Calls            int            `json:"calls,omitempty"`
}

type Asymmetry struct {
	LatencyRatioP50 float64 `json:"latency_ratio_p50"`
	LatencyRatioP99 float64 `json:"latency_ratio_p99"`
	ThroughputRatio float64 `json:"throughput_ratio"`
}

func printReport(r Report) {
	fmt.Printf("SOTA baseline comparison — ADM drift vs Llama Guard  (sample=%d)\n\n", r.Sample)
	arm := func(a Arm) {
		fmt.Printf("  %-28s detect=%5.1f%%  p50=%s  p99=%s  throughput=%s msg/s\n",
			a.Name, a.DetectRate*100, msf(a.Latency.P50MS), msf(a.Latency.P99MS), human(a.ThroughputPerSec))
	}
	arm(r.ADM)
	if r.LlamaGuard != nil {
		arm(*r.LlamaGuard)
		fmt.Printf("\nasymmetry α (SOTA / ADM):  latency p50 ×%s   p99 ×%s   throughput ×%s\n",
			human(r.Asymmetry.LatencyRatioP50), human(r.Asymmetry.LatencyRatioP99), human(r.Asymmetry.ThroughputRatio))
		fmt.Println("→ ADM detects at O(1) per-token cost; the per-message LLM guard pays a full")
		fmt.Println("  model inference per message — orders of magnitude more latency at scale.")
	} else {
		fmt.Printf("\n  Llama Guard arm: %s\n", r.LlamaGuardError)
		fmt.Println("  (ADM arm above is real; run with a hosted guard key for the measured α.)")
	}
}

func msf(ms float64) string {
	switch {
	case ms >= 1:
		return fmt.Sprintf("%.2fms", ms)
	case ms >= 0.001:
		return fmt.Sprintf("%.1fµs", ms*1000)
	default:
		return fmt.Sprintf("%.0fns", ms*1e6)
	}
}
func human(x float64) string {
	switch {
	case x >= 1e6:
		return fmt.Sprintf("%.1fM", x/1e6)
	case x >= 1e3:
		return fmt.Sprintf("%.1fk", x/1e3)
	default:
		return fmt.Sprintf("%.0f", x)
	}
}

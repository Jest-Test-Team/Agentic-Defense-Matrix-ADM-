package semantic

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

// Analyzer performs semantic similarity analysis on prompts.
type Analyzer struct {
	mu              sync.RWMutex
	knownPatterns   []Pattern
	sensitivity     float64
	windowSize      int
	recentPrompts   []PromptRecord
}

// Pattern represents a known injection/malicious pattern.
type Pattern struct {
	Name     string
 Keywords []string
	Weight   float64
}

// PromptRecord tracks a recent prompt for clustering.
type PromptRecord struct {
	Prompt    string
	Timestamp time.Time
	Score     float64
}

// AnalysisResult is the outcome of semantic analysis.
type AnalysisResult struct {
	Score         float64  `json:"score"`
	IsSuspicious  bool     `json:"is_suspicious"`
	IsMalicious   bool     `json:"is_malicious"`
	MatchedPattern string  `json:"matched_pattern,omitempty"`
	Reasons       []string `json:"reasons"`
}

// NewAnalyzer creates a semantic analyzer with default patterns.
func NewAnalyzer(sensitivity float64) *Analyzer {
	a := &Analyzer{
		sensitivity: sensitivity,
		windowSize:  100,
		recentPrompts: make([]PromptRecord, 0),
	}

	// Default known injection patterns
	a.knownPatterns = []Pattern{
		{Name: "system_prompt_override", Keywords: []string{"ignore previous", "ignore all", "disregard instructions", "new instructions"}, Weight: 0.9},
		{Name: "role_manipulation", Keywords: []string{"you are now", "act as", "pretend to be", "your new role"}, Weight: 0.8},
		{Name: "data_exfiltration", Keywords: []string{"send to", "post to", "upload to", "exfiltrate", "leak"}, Weight: 0.85},
		{Name: "code_execution", Keywords: []string{"eval(", "exec(", "system(", "os.system", "subprocess"}, Weight: 0.7},
		{Name: "reverse_shell", Keywords: []string{"bash -i", "nc -e", "reverse shell", "/dev/tcp"}, Weight: 1.0},
		{Name: "privilege_escalation", Keywords: []string{"sudo", "chmod 777", "setuid", "root access"}, Weight: 0.9},
		{Name: "prompt_leak", Keywords: []string{"show me your prompt", "what are your instructions", "reveal your system prompt"}, Weight: 0.75},
		{Name: "delimiter_injection", Keywords: []string{"```system", "```\n\n", "---END---", "<|im_start|>system"}, Weight: 0.85},
	}

	return a
}

// Analyze scores a prompt for suspiciousness.
func (a *Analyzer) Analyze(prompt string) *AnalysisResult {
	a.mu.Lock()
	defer a.mu.Unlock()

	result := &AnalysisResult{
		Reasons: make([]string, 0),
	}

	// Pattern matching
	for _, pattern := range a.knownPatterns {
		for _, keyword := range pattern.Keywords {
			if strings.Contains(strings.ToLower(prompt), strings.ToLower(keyword)) {
				result.Score += pattern.Weight
				result.MatchedPattern = pattern.Name
				result.Reasons = append(result.Reasons,
					fmt.Sprintf("matched pattern %q: %s", pattern.Name, keyword))
			}
		}
	}

	// Frequency analysis: detect repeated similar prompts
	freqScore := a.analyzeFrequency(prompt)
	result.Score += freqScore
	if freqScore > 0.3 {
		result.Reasons = append(result.Reasons, "high frequency of similar prompts detected")
	}

	// Normalize score to [0, 1]
	result.Score = math.Min(result.Score, 1.0)

	// Classification
	result.IsSuspicious = result.Score >= a.sensitivity*0.5
	result.IsMalicious = result.Score >= a.sensitivity

	// Record for frequency analysis
	a.recentPrompts = append(a.recentPrompts, PromptRecord{
		Prompt:    prompt,
		Timestamp: time.Now(),
		Score:     result.Score,
	})

	// Trim window
	if len(a.recentPrompts) > a.windowSize {
		a.recentPrompts = a.recentPrompts[len(a.recentPrompts)-a.windowSize:]
	}

	return result
}

// analyzeFrequency detects automated high-frequency probing.
func (a *Analyzer) analyzeFrequency(prompt string) float64 {
	if len(a.recentPrompts) < 5 {
		return 0
	}

	now := time.Now()
	recent := 0
	for _, rec := range a.recentPrompts {
		if now.Sub(rec.Timestamp) < 10*time.Second {
			recent++
		}
	}

	// High frequency = automated probing
	if recent > 20 {
		return 0.5
	}
	if recent > 10 {
		return 0.3
	}
	return 0
}

// SetSensitivity changes the detection threshold.
func (a *Analyzer) SetSensitivity(s float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sensitivity = s
}

// AddPattern adds a custom detection pattern.
func (a *Analyzer) AddPattern(p Pattern) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.knownPatterns = append(a.knownPatterns, p)
}

// Stats returns analyzer statistics.
type Stats struct {
	PatternCount   int
	RecentPrompts  int
	Sensitivity    float64
	AvgScore       float64
}

// GetStats returns current statistics.
func (a *Analyzer) GetStats() Stats {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var totalScore float64
	for _, rec := range a.recentPrompts {
		totalScore += rec.Score
	}

	avgScore := 0.0
	if len(a.recentPrompts) > 0 {
		avgScore = totalScore / float64(len(a.recentPrompts))
	}

	return Stats{
		PatternCount:  len(a.knownPatterns),
		RecentPrompts: len(a.recentPrompts),
		Sensitivity:   a.sensitivity,
		AvgScore:      avgScore,
	}
}



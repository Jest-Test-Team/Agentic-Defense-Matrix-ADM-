package redteam

import "testing"

// TestCorpusScale guards the RT->10000 expansion: the generator must be able to
// produce at least 10,000 unique attack variants from the base technique
// catalog, and a requested size of 10,000 must return exactly that many.
func TestCorpusScale(t *testing.T) {
	if got := CorpusCapacity(); got < 10000 {
		t.Fatalf("corpus capacity %d < 10000; add seeds/mutations/paraphrases", got)
	}

	corpus := GenerateCorpus(10000, 1337)
	if len(corpus) != 10000 {
		t.Fatalf("GenerateCorpus(10000) returned %d variants", len(corpus))
	}
}

// TestCorpusDeterministic ensures a fixed seed yields a reproducible campaign.
func TestCorpusDeterministic(t *testing.T) {
	a := GenerateCorpus(500, 42)
	b := GenerateCorpus(500, 42)
	if len(a) != len(b) {
		t.Fatalf("length mismatch %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].VariantID != b[i].VariantID || a[i].Payload != b[i].Payload {
			t.Fatalf("non-deterministic at %d: %q vs %q", i, a[i].VariantID, b[i].VariantID)
		}
	}
}

// TestCorpusCoversAllTechniques ensures every base technique appears in a large
// corpus so the campaign exercises the full RT-001..RT-030 catalog.
func TestCorpusCoversAllTechniques(t *testing.T) {
	corpus := GenerateCorpus(0, 7) // 0 = full capacity
	seen := map[string]bool{}
	for _, v := range corpus {
		seen[v.Technique] = true
	}
	for _, tech := range BaseTechniques() {
		if !seen[tech.ID] {
			t.Errorf("technique %s (%s) missing from corpus", tech.ID, tech.Name)
		}
	}
}

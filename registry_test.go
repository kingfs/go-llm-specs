package llmspecs

import (
	"testing"
)

func TestGet(t *testing.T) {
	// Verify exact ID
	id := "openai/gpt-4"
	if m, ok := Get(id); ok {
		if m.ID() != id {
			t.Errorf("Expected %s, got %s", id, m.ID())
		}
	} else {
		t.Errorf("Failed to find model by ID: %s", id)
	}

	// Verify alias (case-insensitive)
	alias := "GpT4t" // Mixed case
	if m, ok := Get(alias); ok {
		if m.ID() != "openai/gpt-4-turbo" {
			t.Errorf("Expected openai/gpt-4-turbo for alias %s, got %s", alias, m.ID())
		}
	} else {
		t.Logf("Warning: alias %s not found (may depend on registry content)", alias)
	}

	// Verify non-existent
	if _, ok := Get("non-existent-model"); ok {
		t.Error("Expected not to find non-existent model")
	}
}

func TestQuery(t *testing.T) {
	// 1. Test Provider filtering
	p := "Anthropic"
	results := Query().Provider(p).List()
	if len(results) == 0 {
		t.Logf("Warning: no models found for provider %s", p)
	}
	for _, m := range results {
		if m.Provider() != p {
			t.Errorf("Expected provider %s, got %s for model %s", p, m.Provider(), m.ID())
		}
	}

	// 2. Test Capability filtering (AND logic)
	// Many vision models also support text
	results = Query().Has(ModalityTextIn).Has(ModalityImageIn).List()
	for _, m := range results {
		if !m.HasCapability(ModalityTextIn) || !m.HasCapability(ModalityImageIn) {
			t.Errorf("Model %s missing required capabilities", m.ID())
		}
	}

	// 3. Test Provider + Capability
	results = Query().Provider("Anthropic").Has(ModalityImageIn).List()
	for _, m := range results {
		if m.Provider() != "Anthropic" || !m.HasCapability(ModalityImageIn) {
			t.Errorf("Model %s does not match combined filter", m.ID())
		}
	}

	// 4. Empty query should return all
	all := Query().List()
	if len(all) != len(staticRegistry) {
		t.Errorf("Expected %d models, got %d", len(staticRegistry), len(all))
	}
}

// Performance Benchmarks

func BenchmarkGetByID(b *testing.B) {
	id := "openai/gpt-4"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Get(id)
	}
}

func BenchmarkGetByAlias(b *testing.B) {
	alias := "gpt4t"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Get(alias)
	}
}

func BenchmarkQueryProvider(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Query().Provider("Anthropic").List()
	}
}

func BenchmarkQueryCapabilities(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Query().Has(ModalityImageIn).Has(CapFunctionCall).List()
	}
}

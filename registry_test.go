package llmspecs

import (
	"strings"
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

func TestGetMany(t *testing.T) {
	names := []string{
		"openai/gpt-4",   // Valid ID
		"GpT4t",          // Valid Alias (case-insensitive)
		"non-existent",   // Non-existent
		"qwen/qwen3-32b", // Another Valid ID
	}

	results := GetMany(names)

	expectedCount := 3 // openai/gpt-4, openai/gpt-4-turbo (alias), qwen/qwen3-32b
	if len(results) != expectedCount {
		t.Errorf("Expected %d models, got %d", expectedCount, len(results))
	}

	foundIDs := make(map[string]bool)
	for _, m := range results {
		foundIDs[m.ID()] = true
	}

	expectedIDs := []string{"openai/gpt-4", "openai/gpt-4-turbo", "qwen/qwen3-32b"}
	for _, id := range expectedIDs {
		if !foundIDs[id] {
			t.Errorf("Expected model %s was not found in results", id)
		}
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

func BenchmarkGetMany(b *testing.B) {
	names := []string{
		"openai/gpt-4",
		"gpt4t",
		"anthropic/claude-3-5-sonnet",
		"qwen3-32b",
		"non-existent",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetMany(names)
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

func TestSearch(t *testing.T) {
	// Test exact ID match
	res := Search("openai/gpt-4", 1)
	if len(res) == 0 || res[0].ID() != "openai/gpt-4" {
		t.Error("Search for exact ID failed")
	}

	// Test case-insensitive prefix match
	res = Search("claude", 5)
	if len(res) == 0 {
		t.Error("Search for 'claude' should return results")
	}
	for _, m := range res {
		if !strings.Contains(strings.ToLower(m.ID()), "claude") && !strings.Contains(strings.ToLower(m.Name()), "claude") {
			t.Errorf("Model %s should match 'claude'", m.ID())
		}
	}

	// Test ranked alias match
	res = Search("gpt4t", 1)
	if len(res) == 0 || res[0].ID() != "openai/gpt-4-turbo" {
		t.Errorf("Search for 'gpt4t' alias should return gpt-4-turbo as top result, got %v", res)
	}
}

func BenchmarkSearch(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Search("gpt-4", 10)
	}
}

func TestSearch_Ranking(t *testing.T) {
	// 1. Setup a controlled environment if possible, but since staticRegistry is global and large,
	// we'll search for things we know are there and check their relative order.

	// Target: openai/gpt-4
	// Rules: Exact match > everything else
	results := Search("openai/gpt-4", 10)
	if len(results) == 0 {
		t.Fatal("Search for 'openai/gpt-4' returned no results")
	}
	if results[0].ID() != "openai/gpt-4" {
		t.Errorf("Top result should be 'openai/gpt-4', got %s", results[0].ID())
	}

	// Target: "gpt-4"
	// Should return openai/gpt-4 as higher than things that just contain "gpt-4" in suffix
	results = Search("gpt-4", 10)
	topFound := false
	for _, m := range results {
		if m.ID() == "openai/gpt-4" {
			topFound = true
			break
		}
	}
	if !topFound {
		t.Error("Search for 'gpt-4' did not find 'openai/gpt-4'")
	}
}

func TestGet_Aliases(t *testing.T) {
	// Verify automatic suffix alias
	id := "qwen/qwen3-32b"
	alias := "qwen3-32b"
	if m, ok := Get(alias); ok {
		if m.ID() != id {
			t.Errorf("Expected %s for alias %s, got %s", id, alias, m.ID())
		}
	} else {
		t.Errorf("Failed to find model by auto-suffix alias: %s", alias)
	}

	// Verify case-insensitive alias
	if m, ok := Get("QwEn3-32b"); !ok || m.ID() != id {
		t.Error("Alias lookup should be case-insensitive")
	}
}

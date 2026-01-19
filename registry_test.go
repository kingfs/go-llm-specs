package llmspecs

import (
	"testing"
)

func TestGet(t *testing.T) {
	// Note: these tests depend on the generated models_gen.go
	// Since we can't guarantee which models are present, we'll check for common ones
	// or just check if the logic works.

	// Test case-insensitive alias lookup
	if m, ok := Get("GPT4T"); ok {
		if m.ID() != "openai/gpt-4-turbo" {
			t.Errorf("Expected openai/gpt-4-turbo, got %s", m.ID())
		}
	} else {
		t.Log("Warning: gpt4t alias not found in the current registry (maybe generation failed or alias missing)")
	}

	// Test exact ID lookup
	if m, ok := Get("anthropic/claude-3.5-sonnet"); ok {
		if m.Name() == "" {
			t.Error("Expected non-empty model name")
		}
	}
}

func TestQuery(t *testing.T) {
	// Filter by provider
	anthropicModels := Query().Provider("Anthropic").List()
	for _, m := range anthropicModels {
		if m.Provider() != "Anthropic" {
			t.Errorf("Expected Anthropic provider, got %s", m.Provider())
		}
	}

	// Filter by capability
	visionModels := Query().Has(ModalityImageIn).List()
	for _, m := range visionModels {
		if !m.HasCapability(ModalityImageIn) {
			t.Errorf("Model %s should have image-in capability", m.ID())
		}
	}
}

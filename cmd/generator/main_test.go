package main

import (
	"go/format"
	"os"
	"path/filepath"
	"testing"

	registrymodel "github.com/kingfs/go-llm-specs/internal/registry"
)

func TestGenerateCodeWritesFormattedGo(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "models_gen.go")
	if err := generateCode(outputPath, nil, map[string]string{"short": "provider/model"}); err != nil {
		t.Fatalf("generate code: %v", err)
	}

	generated, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read generated code: %v", err)
	}
	formatted, err := format.Source(generated)
	if err != nil {
		t.Fatalf("generated invalid Go: %v", err)
	}
	if string(generated) != string(formatted) {
		t.Fatal("generated registry is not gofmt-formatted")
	}
}

func TestGenerateCodeIsDeterministic(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "models_gen.go")
	if err := generateCode(outputPath, nil, nil); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := generateCode(outputPath, nil, nil); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("identical inputs produced different generated code")
	}
}

func TestBuildProcessedModelsExcludesLifecycleCandidates(t *testing.T) {
	models := map[string]ModelRegistry{
		"org/active":    {ID: "org/active", Name: "Active"},
		"org/candidate": {ID: "org/candidate", Name: "Candidate", Lifecycle: "candidate"},
		"org/rejected":  {ID: "org/rejected", Name: "Rejected", Lifecycle: "rejected"},
	}
	processed := buildProcessedModels(models)
	if len(processed) != 1 || processed[0].ID != "org/active" {
		t.Fatalf("candidate leaked into generated registry: %#v", processed)
	}
}

func TestMergeModelRegistryLocalOverridesWin(t *testing.T) {
	upstream := OpenRouterModel{
		ID:            "openai/example",
		Name:          "OpenAI Example",
		Description:   "Upstream description",
		ContextLength: 8192,
		TopProvider: OpenRouterTopProvider{
			MaxCompletionTokens: 4096,
		},
		Architecture: OpenRouterArchitecture{
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
	}

	local := ModelRegistry{
		ID:            "openai/example",
		Name:          "Custom Name",
		Provider:      "OpenAI",
		Description:   "Local description",
		DescriptionCN: "本地中文描述",
		ContextLen:    16384,
		MaxOutput:     2048,
		Features:      []string{"CapEmbedding"},
		Aliases:       []string{"custom-alias", "custom-alias"},
	}

	merged := mergeModelRegistry(upstream, local)

	if merged.Name != "Custom Name" {
		t.Fatalf("expected local name override, got %q", merged.Name)
	}
	if merged.Description != "Local description" {
		t.Fatalf("expected local description override, got %q", merged.Description)
	}
	if merged.DescriptionCN != "本地中文描述" {
		t.Fatalf("expected local chinese description override, got %q", merged.DescriptionCN)
	}
	if merged.ContextLen != 16384 {
		t.Fatalf("expected local context length override, got %d", merged.ContextLen)
	}
	if merged.MaxOutput != 2048 {
		t.Fatalf("expected local max output override, got %d", merged.MaxOutput)
	}
	if len(merged.Features) != 2 || merged.Features[0] != "CapChat" || merged.Features[1] != "CapEmbedding" {
		t.Fatalf("unexpected merged features: %#v", merged.Features)
	}
	if len(merged.Aliases) != 1 || merged.Aliases[0] != "custom-alias" {
		t.Fatalf("unexpected merged aliases: %#v", merged.Aliases)
	}
}

func TestMergeModelRegistryPreservesRichAndUnknownFields(t *testing.T) {
	local := ModelRegistry{
		ID:            "test/model",
		Name:          "Local",
		Provider:      "Test",
		SchemaVersion: 2,
		Codex: &registrymodel.CodexMetadata{
			Enabled: true,
			Slugs:   []string{"served-model"},
		},
		Extra: map[string]any{"future": "preserved"},
	}
	merged := mergeModelRegistry(OpenRouterModel{
		ID:            "test/model",
		Name:          "Upstream",
		ContextLength: 2048,
	}, local)
	if merged.Codex == nil || !merged.Codex.Enabled || merged.Extra["future"] != "preserved" {
		t.Fatalf("rich metadata was lost: %#v", merged)
	}
}

func TestMergeNeverOverwritesPersistedFacts(t *testing.T) {
	local := ModelRegistry{ID: "test/model", Name: "Old", ContextLen: 1, Provenance: map[string]registrymodel.Provenance{
		"name": {Source: "openrouter"}, "context_length": {Source: "openrouter"},
	}}
	merged := mergeModelRegistry(OpenRouterModel{ID: "test/model", Name: "New", ContextLength: 2048}, local)
	if merged.Name != "Old" || merged.ContextLen != 1 {
		t.Fatalf("persisted facts were overwritten because of stale provenance: %#v", merged)
	}
	local.Name, local.ContextLen, local.Provenance = "Human", 4096, nil
	merged = mergeModelRegistry(OpenRouterModel{ID: "test/model", Name: "New", ContextLength: 2048}, local)
	if merged.Name != "Human" || merged.ContextLen != 4096 {
		t.Fatalf("human facts were overwritten: %#v", merged)
	}
}

func TestStringsToFeatures(t *testing.T) {
	features := stringsToFeatures("ModalityTextIn | ModalityTextOut | CapFunctionCall")
	if len(features) != 3 {
		t.Fatalf("expected 3 features, got %#v", features)
	}
	if features[0] != "CapFunctionCall" {
		t.Fatalf("expected normalized sort order, got %#v", features)
	}
}

func TestNormalizeDeveloperID(t *testing.T) {
	for input, want := range map[string]string{"Meta Llama": "meta", "Mistral AI": "mistral", "X-Ai": "xai", "OpenAI": "openai"} {
		if got := normalizeDeveloperID(input); got != want {
			t.Fatalf("normalizeDeveloperID(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCanonicalModelID(t *testing.T) {
	tests := []struct {
		id, description, want string
	}{
		{"openai/gpt-5.6-terra:batch", "", "openai/gpt-5.6-terra"},
		{"poolside/laguna:free", "", "poolside/laguna"},
		{"openai/gpt-5.6-terra-pro:batch", "same underlying model as Terra", "openai/gpt-5.6-terra"},
		{"anthropic/claude-opus-5-fast", "Identical capabilities with higher output speed", "anthropic/claude-opus-5"},
		{"openai/o3-pro", "uses more compute", "openai/o3-pro"},
	}
	for _, tt := range tests {
		if got := canonicalModelID(tt.id, tt.description); got != tt.want {
			t.Errorf("canonicalModelID(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
}

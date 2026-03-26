package main

import "testing"

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

func TestStringsToFeatures(t *testing.T) {
	features := stringsToFeatures("ModalityTextIn | ModalityTextOut | CapFunctionCall")
	if len(features) != 3 {
		t.Fatalf("expected 3 features, got %#v", features)
	}
	if features[0] != "CapFunctionCall" {
		t.Fatalf("expected normalized sort order, got %#v", features)
	}
}

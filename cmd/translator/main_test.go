package main

import "testing"

func TestCollectPendingTranslationsAppliesFilters(t *testing.T) {
	registry := []*ModelRegistry{
		{ID: "openai/a", Provider: "OpenAI", Description: "A"},
		{ID: "openai/b", Provider: "OpenAI", Description: "B", DescriptionCN: "已有"},
		{ID: "qwen/c", Provider: "Qwen", Description: "C"},
		{ID: "qwen/d", Provider: "Qwen"},
	}

	cfg := translatorConfig{
		Provider:    "openai",
		IDPrefix:    "openai/",
		OnlyMissing: true,
	}

	pending := collectPendingTranslations(registry, cfg)
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending model, got %d", len(pending))
	}
	if pending[0].ID != "openai/a" {
		t.Fatalf("expected openai/a, got %s", pending[0].ID)
	}
}

func TestCollectPendingTranslationsHonorsLimit(t *testing.T) {
	registry := []*ModelRegistry{
		{ID: "a", Description: "A"},
		{ID: "b", Description: "B"},
		{ID: "c", Description: "C"},
	}

	cfg := translatorConfig{
		OnlyMissing: true,
		Limit:       2,
	}

	pending := collectPendingTranslations(registry, cfg)
	if len(pending) != 2 {
		t.Fatalf("expected limit to cap pending set at 2, got %d", len(pending))
	}
}

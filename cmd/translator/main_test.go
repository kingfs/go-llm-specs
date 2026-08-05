package main

import (
	"testing"
	"time"
)

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
	old := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	newest := old.Add(48 * time.Hour)
	registry := []*ModelRegistry{
		{ID: "a", Description: "A", DiscoveredAt: &old},
		{ID: "b", Description: "B", DiscoveredAt: &newest},
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
	if pending[0].ID != "b" || pending[1].ID != "a" {
		t.Fatalf("expected newest models first, got %s then %s", pending[0].ID, pending[1].ID)
	}
}

func TestValidateTranslationsRequiresExactKeysAndValues(t *testing.T) {
	inputs := map[string]string{"model/a": "A", "model/b": "B"}
	if err := validateTranslations(inputs, map[string]string{"model/a": "甲", "model/b": "乙"}); err != nil {
		t.Fatal(err)
	}
	for name, result := range map[string]map[string]string{
		"missing":    {"model/a": "甲"},
		"unexpected": {"model/a": "甲", "model/c": "丙"},
		"empty":      {"model/a": "甲", "model/b": " "},
	} {
		if err := validateTranslations(inputs, result); err == nil {
			t.Fatalf("%s result unexpectedly passed validation", name)
		}
	}
}

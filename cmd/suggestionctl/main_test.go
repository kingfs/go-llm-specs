package main

import (
	"encoding/json"
	"testing"

	"github.com/kingfs/go-llm-specs/internal/provider"

	"github.com/kingfs/go-llm-specs/internal/registry"
	"github.com/kingfs/go-llm-specs/internal/suggestion"
)

func TestOfficialModelCardSourceRequiresConfiguredOrganizationAndRevision(t *testing.T) {
	model := registry.Model{Developer: "qwen", Upstream: registry.UpstreamMetadata{HuggingFace: &registry.HuggingFaceMetadata{ID: "Qwen/Model"}}}
	doc := suggestion.Document{Source: suggestion.Source{URL: "https://huggingface.co/Qwen/Model/resolve/abc/README.md", Revision: "abc"}}
	providers := map[string]provider.Provider{"qwen": {ID: "qwen", Organizations: provider.Organizations{HuggingFace: []string{"Qwen"}}}}
	if !officialModelCardSource(doc, model, providers) {
		t.Fatal("official pinned source rejected")
	}
	doc.Source.URL = "https://huggingface.co/untrusted/Model/resolve/abc/README.md"
	if officialModelCardSource(doc, model, providers) {
		t.Fatal("untrusted source accepted")
	}
}

func TestApplyClaim(t *testing.T) {
	model := registry.Model{}
	if err := applyClaim(&model, suggestion.Claim{Field: "context_length", Value: json.RawMessage(`32768`)}); err != nil {
		t.Fatal(err)
	}
	if model.ContextLen != 32768 {
		t.Fatalf("got %d", model.ContextLen)
	}
}

func TestSafeAutoClaimNeverOverwritesPersistedFacts(t *testing.T) {
	model := registry.Model{
		Description: "human correction",
		ContextLen:  32768,
		MaxOutput:   4096,
		Features:    []string{"CapChat"},
		Reasoning:   &registry.ReasoningMetadata{Supported: true},
		Provenance: map[string]registry.Provenance{
			"description":    {Source: "openrouter"},
			"context_length": {Source: "openrouter"},
			"max_output":     {Source: "openrouter"},
		},
	}
	for _, field := range []string{"description", "context_length", "max_output", "features", "reasoning.supported"} {
		if safeAutoClaim(model, field) {
			t.Fatalf("persisted %s was considered safe to overwrite", field)
		}
	}
}

func TestApplyFeaturesMergesWithoutRemovingExisting(t *testing.T) {
	model := registry.Model{Features: []string{"CapChat", "ModalityTextIn"}}
	claim := suggestion.Claim{Field: "features", Value: json.RawMessage(`["ModalityImageIn","CapChat"]`)}
	if err := applyClaim(&model, claim); err != nil {
		t.Fatal(err)
	}
	if len(model.Features) != 3 {
		t.Fatalf("features were replaced or duplicated: %v", model.Features)
	}
}

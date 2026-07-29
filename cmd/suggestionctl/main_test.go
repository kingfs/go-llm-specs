package main

import (
	"encoding/json"
	"testing"

	"github.com/kingfs/go-llm-specs/internal/registry"
	"github.com/kingfs/go-llm-specs/internal/suggestion"
)

func TestApplyClaim(t *testing.T) {
	model := registry.Model{}
	if err := applyClaim(&model, suggestion.Claim{Field: "context_length", Value: json.RawMessage(`32768`)}); err != nil {
		t.Fatal(err)
	}
	if model.ContextLen != 32768 {
		t.Fatalf("got %d", model.ContextLen)
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

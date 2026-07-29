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

package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kingfs/go-llm-specs/internal/registry"
)

func codexReadyModel() registry.Model {
	return registry.Model{
		SchemaVersion: 2,
		ID:            "qwen/qwen3.6-27b",
		Name:          "Qwen 3.6 27B",
		Description:   "Coding model",
		Provider:      "Qwen",
		ContextLen:    262144,
		Features:      []string{"CapChat", "CapFunctionCall", "ModalityTextIn", "ModalityTextOut", "ModalityImageIn"},
		Reasoning:     &registry.ReasoningMetadata{Supported: true},
		Codex: &registry.CodexMetadata{
			Enabled:            true,
			Slugs:              []string{"qwen3.6-27b"},
			ApplyPatchToolType: "freeform",
		},
	}
}

func TestGenerateCodexModel(t *testing.T) {
	generated, err := generate([]registry.Model{codexReadyModel()})
	if err != nil {
		t.Fatal(err)
	}
	if len(generated) != 1 {
		t.Fatalf("got %d entries", len(generated))
	}
	var entry map[string]any
	if err := json.Unmarshal(generated[0], &entry); err != nil {
		t.Fatal(err)
	}
	if entry["slug"] != "qwen3.6-27b" || entry["context_window"] != float64(262144) {
		t.Fatalf("unexpected entry: %#v", entry)
	}
	modalities := entry["input_modalities"].([]any)
	if len(modalities) != 2 || modalities[1] != "image" {
		t.Fatalf("unexpected modalities: %#v", modalities)
	}
}

func TestGenerateRequiresExplicitServingSlug(t *testing.T) {
	model := codexReadyModel()
	model.Codex.Slugs = nil
	_, err := generate([]registry.Model{model})
	if err == nil || !strings.Contains(err.Error(), "codex.slugs") {
		t.Fatalf("expected slug validation error, got %v", err)
	}
}

func TestGenerateRejectsDuplicateSlugs(t *testing.T) {
	one := codexReadyModel()
	two := codexReadyModel()
	two.ID = "other/model"
	_, err := generate([]registry.Model{one, two})
	if err == nil || !strings.Contains(err.Error(), "duplicate Codex slug") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestMergeCatalogsPreservesRawBundledFields(t *testing.T) {
	base := []json.RawMessage{json.RawMessage(`{"slug":"official","future_field":true}`)}
	addition := []json.RawMessage{json.RawMessage(`{"slug":"third-party"}`)}
	merged, err := mergeCatalogs(base, addition)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 2 || !strings.Contains(string(merged[0]), "future_field") {
		t.Fatalf("raw fields were not preserved: %s", merged)
	}
}

func TestMergeCatalogsRejectsCollision(t *testing.T) {
	_, err := mergeCatalogs(
		[]json.RawMessage{json.RawMessage(`{"slug":"same"}`)},
		[]json.RawMessage{json.RawMessage(`{"slug":"SAME"}`)},
	)
	if err == nil {
		t.Fatal("expected collision error")
	}
}

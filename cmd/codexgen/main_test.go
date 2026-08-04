package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kingfs/go-llm-specs/internal/provider"
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

func TestOfficialHuggingFaceIdentity(t *testing.T) {
	model := codexReadyModel()
	model.Developer = "qwen"
	model.Links.ModelCard = "https://huggingface.co/Qwen/Qwen3.6-27B"
	model.Upstream.HuggingFace = &registry.HuggingFaceMetadata{ID: "Qwen/Qwen3.6-27B", Revision: "abc123"}
	publisher := provider.Provider{Organizations: provider.Organizations{HuggingFace: []string{"Qwen"}}}
	if !officialHuggingFaceIdentity(model, publisher) {
		t.Fatal("official pinned model identity was rejected")
	}
	model.Upstream.HuggingFace.Revision = ""
	if officialHuggingFaceIdentity(model, publisher) {
		t.Fatal("unpinned model identity was accepted")
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

func TestDefaultPolicyIncludesOpenWeightFamily(t *testing.T) {
	model := codexReadyModel()
	model.Codex = nil
	model.Upstream.HuggingFace = &registry.HuggingFaceMetadata{ID: "Qwen/Qwen3.6-27B"}
	policy := defaultPolicy{SchemaVersion: 1, Families: []policyFamily{{
		Name: "qwen", IDPattern: `^qwen/qwen3\.[5-9].*`, RequireHuggingFace: true,
		SlugStrategies: []string{"model_suffix"},
	}}}
	models, err := applyDefaultPolicy([]registry.Model{model}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if models[0].Codex == nil || !models[0].Codex.Enabled {
		t.Fatal("matching model was not enabled")
	}
	want := []string{"qwen3.6-27b"}
	if strings.Join(models[0].Codex.Slugs, ",") != strings.Join(want, ",") {
		t.Fatalf("slugs = %#v", models[0].Codex.Slugs)
	}
}

func TestModelSuffixPolicySlugIsLowercaseAndVendorFree(t *testing.T) {
	model := codexReadyModel()
	model.ID = "DeepSeek/DeepSeek-V3.1"
	slugs, err := policySlugs(model, []string{"model_suffix"})
	if err != nil {
		t.Fatal(err)
	}
	if len(slugs) != 1 || slugs[0] != "deepseek-v3.1" {
		t.Fatalf("slugs = %#v", slugs)
	}
}

func TestDefaultPolicyRequiresHuggingFaceAndHonorsExplicitOptOut(t *testing.T) {
	withoutWeights := codexReadyModel()
	withoutWeights.Codex = nil
	explicitOptOut := codexReadyModel()
	explicitOptOut.ID = "qwen/qwen3.7-open"
	explicitOptOut.Codex = &registry.CodexMetadata{Enabled: false}
	explicitOptOut.Upstream.HuggingFace = &registry.HuggingFaceMetadata{ID: "Qwen/Qwen3.7-Open"}
	policy := defaultPolicy{SchemaVersion: 1, Families: []policyFamily{{
		Name: "qwen", IDPattern: `^qwen/qwen3\.[5-9].*`, RequireHuggingFace: true,
		SlugStrategies: []string{"registry_id"},
	}}}
	models, err := applyDefaultPolicy([]registry.Model{withoutWeights, explicitOptOut}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if models[0].Codex != nil {
		t.Fatal("model without weights was included")
	}
	if models[1].Codex == nil || models[1].Codex.Enabled {
		t.Fatal("explicit opt-out was not preserved")
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

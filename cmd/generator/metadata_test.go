package main

import (
	"testing"

	llmspecs "github.com/kingfs/go-llm-specs"
)

func TestDeriveStructuredMetadata(t *testing.T) {
	model := ModelRegistry{
		ID:          "qwen/qwen3-coder-next",
		Name:        "Qwen: Qwen3 Coder Next",
		Provider:    "Qwen",
		Description: "Qwen3-Coder-Next is optimized for coding agents and local development workflows.",
		Features:    []string{"CapChat", "CapFunctionCall", "ModalityTextIn", "ModalityTextOut"},
	}

	meta := deriveStructuredMetadata(model)
	if meta.Family != "Qwen" {
		t.Fatalf("expected family Qwen, got %q", meta.Family)
	}
	if meta.Series != "Qwen3 Coder Next" {
		t.Fatalf("expected series derived from display name, got %q", meta.Series)
	}
	if meta.Summary == "" {
		t.Fatal("expected non-empty summary")
	}
	if len(meta.Tags) == 0 {
		t.Fatal("expected non-empty tags")
	}
	hasCoding := false
	for _, tag := range meta.Tags {
		if tag == "coding" {
			hasCoding = true
		}
	}
	if !hasCoding {
		t.Fatalf("expected canonical coding tag, got %#v", meta.Tags)
	}
}

func TestDeriveTagsRequiresWordBoundaries(t *testing.T) {
	// Short variant markers must not be triggered by longer words: a protein
	// language model is not a "pro" variant, and "provides" is not an IDE.
	domain := ModelRegistry{
		ID:          "microsoft/dayhoff-170m-grs-ss-14000",
		Name:        "Dayhoff-170M-GRS-SS-14000",
		Provider:    "Microsoft",
		Description: "Dayhoff is a protein sequence generation model trained on metagenomic data. It provides infilling.",
	}
	for _, tag := range deriveTags(domain, "Dayhoff", "Dayhoff-170M") {
		if tag == string(llmspecs.TagPro) || tag == string(llmspecs.TagCoding) {
			t.Fatalf("false positive tag %q in %#v", tag, deriveTags(domain, "Dayhoff", "Dayhoff-170M"))
		}
	}

	// Real variant markers still match.
	pro := ModelRegistry{ID: "example/model-pro", Name: "Model Pro", Provider: "Example", Description: "A pro tier model."}
	if !containsTag(deriveTags(pro, "Model", "Model Pro"), string(llmspecs.TagPro)) {
		t.Fatalf("expected pro tag, got %#v", deriveTags(pro, "Model", "Model Pro"))
	}
	thinking := ModelRegistry{ID: "example/model:thinking", Name: "Model", Provider: "Example"}
	if !containsTag(deriveTags(thinking, "Model", "Model"), string(llmspecs.TagThinking)) {
		t.Fatalf("expected thinking tag, got %#v", deriveTags(thinking, "Model", "Model"))
	}
}

func containsTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}

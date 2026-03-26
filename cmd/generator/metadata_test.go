package main

import "testing"

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

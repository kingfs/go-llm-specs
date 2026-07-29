package main

import (
	"strings"
	"testing"

	"github.com/kingfs/go-llm-specs/internal/registry"
)

func TestExtractionPromptConstrainsClaims(t *testing.T) {
	prompt := extractionPrompt(registry.Model{ID: "qwen/test"}, "# Card\nSupports tools")
	for _, required := range []string{"exact quote", "Do not infer runtime behavior", "qwen/test"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("prompt missing %q", required)
		}
	}
}

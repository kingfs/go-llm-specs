package llmspecs

import "testing"

func TestNormalizeTag(t *testing.T) {
	tests := map[string]string{
		"coder":    "coding",
		"beta":     "preview",
		"tools":    "tool-use",
		"vision":   "vision",
		"thinking": "thinking",
		"reranker": "rerank",
		"research": "search",
		"tool_use": "tool-use",
		"preview":  "preview",
		"mini":     "mini",
	}

	for input, want := range tests {
		if got := NormalizeTag(input); got != want {
			t.Fatalf("NormalizeTag(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestKnownTags(t *testing.T) {
	tags := KnownTags()
	if len(tags) == 0 {
		t.Fatal("expected known tags")
	}
	if tags[0].Category == "" || tags[0].Label == "" || tags[0].Name == "" {
		t.Fatalf("expected populated tag descriptor, got %#v", tags[0])
	}
}

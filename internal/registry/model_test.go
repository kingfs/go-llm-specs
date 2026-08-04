package registry

import (
	"strings"
	"testing"
)

func TestRichMetadataAndUnknownFieldsRoundTrip(t *testing.T) {
	input := `schema_version: 2
id: qwen/example
name: Example
provider: Qwen
developer: qwen
context_length: 1024
links:
  model_card: https://huggingface.co/Qwen/Example
identifiers:
  huggingface: [Qwen/Example]
provenance:
  context_length:
    source: official_model_card
    url: https://huggingface.co/Qwen/Example
future_top_level: keep-me
upstream:
  huggingface:
    id: Qwen/Example
    future_hf_field: keep-hf
codex:
  enabled: true
  slugs: [example]
  future_codex_field: keep-codex
`
	model, err := Decode([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if !model.IsV2() || model.Upstream.HuggingFace == nil || model.Codex == nil {
		t.Fatalf("rich fields were not decoded: %#v", model)
	}
	if model.Developer != "qwen" || model.Links.ModelCard == "" || len(model.Identifiers.HuggingFace) != 1 {
		t.Fatalf("identity fields were not decoded: %#v", model)
	}
	encoded, err := Encode(model)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"future_top_level: keep-me", "future_hf_field: keep-hf", "future_codex_field: keep-codex"} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("round trip lost %q:\n%s", want, encoded)
		}
	}
}

func TestLegacyModelDoesNotGainSchemaVersion(t *testing.T) {
	model, err := Decode([]byte("id: old/model\nname: Old\nprovider: Old\ncontext_length: 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if model.IsV2() {
		t.Fatal("legacy model unexpectedly treated as v2")
	}
	encoded, err := Encode(model)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "schema_version") {
		t.Fatalf("legacy model was upgraded:\n%s", encoded)
	}
}

package main

import (
	"testing"

	"github.com/kingfs/go-llm-specs/internal/provider"
	"github.com/kingfs/go-llm-specs/internal/registry"
)

func testProvider(id string, hfOrgs []string) provider.Provider {
	p := provider.Provider{
		SchemaVersion: provider.CurrentSchemaVersion,
		ID:            id,
		Name:          id,
		Official:      provider.Official{Homepage: "https://example.com/"},
		Organizations: provider.Organizations{HuggingFace: hfOrgs},
	}
	if len(hfOrgs) > 0 {
		p.Identity = provider.Identity{Strategy: provider.IdentityStrategyPublisher}
	}
	return p
}

func TestBuildReportClassifiesRecords(t *testing.T) {
	providers := []provider.Provider{
		testProvider("deepseek", []string{"deepseek-ai"}),
		testProvider("openai", nil),
	}
	models := []registry.Model{
		{ID: "deepseek/deepseek-v4-flash", Provider: "DeepSeek", Developer: "deepseek", Lifecycle: "active",
			Identifiers: registry.ModelIdentifiers{HuggingFace: []string{"deepseek-ai/DeepSeek-V4-Flash"}}},
		{ID: "deepseek/deepseek-v4-flash-dspark", Provider: "DeepSeek", Developer: "deepseek", Lifecycle: "active"},
		{ID: "openai/gpt-chat-latest", Provider: "OpenAI", Developer: "openai", Lifecycle: "active"},
		{ID: "zai/glm-5.3-bf16", Provider: "Z.ai", Developer: "zai", Lifecycle: "active",
			Upstream: registry.UpstreamMetadata{HuggingFace: &registry.HuggingFaceMetadata{ID: "zai-org/GLM-5.3-BF16"}}},
		{ID: "qwen/qwen3.5-4b-base", Provider: "Qwen", Developer: "qwen", Lifecycle: "candidate",
			Identifiers: registry.ModelIdentifiers{HuggingFace: []string{"Qwen/Qwen3.5-4B-Base"}}},
	}
	report := buildReport(providers, models)

	if report.Summary.Models != 5 {
		t.Fatalf("summary models = %d", report.Summary.Models)
	}
	if report.ByKind[registry.KindDraftHead] != 1 || report.ByKind[registry.KindModel] != 4 {
		t.Fatalf("unexpected kinds: %#v", report.ByKind)
	}
	if len(report.DraftHeads) != 1 || report.DraftHeads[0] != "deepseek/deepseek-v4-flash-dspark" {
		t.Fatalf("draft heads = %#v", report.DraftHeads)
	}
	if len(report.Quantization) != 1 || report.Quantization[0] != "zai/glm-5.3-bf16" {
		t.Fatalf("quantization = %#v", report.Quantization)
	}
	if len(report.BaseVariants) != 1 || report.BaseVariants[0] != "qwen/qwen3.5-4b-base" {
		t.Fatalf("base variants = %#v", report.BaseVariants)
	}
	// openai/gpt-chat-latest is aggregator-backed and has no HF identity, so it
	// is not a gap; deepseek needs one and has it.
	for _, gap := range report.IdentityGaps {
		if gap.ID == "deepseek/deepseek-v4-flash" {
			t.Fatal("verified model reported as an identity gap")
		}
	}
	if len(report.Aggregators) != 1 || report.Aggregators[0] != "openai" {
		t.Fatalf("aggregators = %#v", report.Aggregators)
	}
}

func TestBuildReportFlagsIdentityGap(t *testing.T) {
	providers := []provider.Provider{testProvider("qwen", []string{"Qwen"})}
	models := []registry.Model{{
		ID: "qwen/qwen-max", Provider: "Qwen", Developer: "qwen", Lifecycle: "active", FilePath: "models/qwen/qwen-max.yaml",
	}}
	report := buildReport(providers, models)
	if len(report.IdentityGaps) != 1 || report.IdentityGaps[0].Provider != "qwen" {
		t.Fatalf("identity gaps = %#v", report.IdentityGaps)
	}
	if report.Summary.Unverified != 1 {
		t.Fatalf("unverified = %d, want 1", report.Summary.Unverified)
	}
	if len(report.Identity.Unverified) != 1 || report.Identity.Unverified[0].ID != "qwen/qwen-max" {
		t.Fatalf("unverified identities = %#v", report.Identity.Unverified)
	}
}

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kingfs/go-llm-specs/internal/provider"
	"github.com/kingfs/go-llm-specs/internal/registry"
)

func TestBuildCatalogMapsProviderAndCapabilities(t *testing.T) {
	catalog := buildCatalog([]provider.Provider{{
		ID: "anthropic", Name: "Anthropic", Aliases: []string{"Claude"},
		Official: provider.Official{Homepage: "https://anthropic.com"},
	}}, []registry.Model{{
		ID: "anthropic/test", Name: "Anthropic: Test Model", Provider: "Claude",
		Description: "English", DescriptionCN: "中文", ContextLen: 200000, MaxOutput: 8192,
		Features:  []string{"CapChat", "CapFunctionCall", "ModalityImageIn"},
		Reasoning: &registry.ReasoningMetadata{Supported: true},
	}}, map[string]int64{"anthropic/test": 1700000000})

	if catalog.SchemaVersion != catalogSchemaVersion || catalog.Stats.Models != 1 || catalog.Stats.Providers != 1 {
		t.Fatalf("unexpected catalog metadata: %+v", catalog)
	}
	if got := catalog.Models[0]; got.Name != "Test Model" || got.ProviderID != "anthropic" || got.ReleasedAt != 1700000000 {
		t.Fatalf("unexpected model projection: %+v", got)
	}
	for _, tag := range []string{"chat", "multimodal", "reasoning", "tool-use", "vision"} {
		if !contains(catalog.Models[0].Tags, tag) {
			t.Errorf("expected tag %q in %v", tag, catalog.Models[0].Tags)
		}
	}
}

func TestBuildCatalogCleansProviderAndOmitsFreeTag(t *testing.T) {
	catalog := buildCatalog(nil, []registry.Model{{
		ID: "~openai/gpt-test_free", Name: "~OpenAI: GPT Test Free", Provider: "~OpenAI",
		ContextLen: 128000, Features: []string{"CapChat"},
	}}, nil)
	model := catalog.Models[0]
	if model.Provider != "OpenAI" || model.ProviderID != "openai" || model.Authority != 100 {
		t.Fatalf("provider was not normalized: %+v", model)
	}
	if contains(model.Tags, "free") {
		t.Fatalf("OpenRouter free marker leaked into tags: %v", model.Tags)
	}
}

func TestRewriteDocLinks(t *testing.T) {
	html := rewriteDocLinks(`<a href="./README_EN.md">English</a><a href="./docs/DEVELOPMENT.md">Development</a><a href="./capability.go">Code</a>`)
	for _, link := range []string{`href="../about-en/"`, `href="../development/"`, `href="https://github.com/kingfs/go-llm-specs/blob/master/capability.go"`} {
		if !strings.Contains(html, link) {
			t.Errorf("rewritten HTML does not contain %s: %s", link, html)
		}
	}
}

func TestGenerateCreatesDeployableSite(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	output := t.TempDir()
	if err := generate(filepath.Join(repoRoot, "providers"), filepath.Join(repoRoot, "models"), filepath.Join(repoRoot, "docs"), filepath.Join(repoRoot, "data", "models.json"), output); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"index.html", "catalog.json", "assets/app.css", "assets/app.js", "docs/about/index.html", "docs/about-en/index.html", "404.html"} {
		if _, err := os.Stat(filepath.Join(output, path)); err != nil {
			t.Errorf("missing generated file %s: %v", path, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(output, "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog siteCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}
	if catalog.Stats.Models == 0 || catalog.Stats.Providers == 0 {
		t.Fatalf("empty catalog stats: %+v", catalog.Stats)
	}
}

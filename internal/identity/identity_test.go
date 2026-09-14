package identity

import (
	"testing"

	"github.com/kingfs/go-llm-specs/internal/provider"
	"github.com/kingfs/go-llm-specs/internal/registry"
)

func publisher(id string, hfOrgs []string) provider.Provider {
	return provider.Provider{
		SchemaVersion: provider.CurrentSchemaVersion,
		ID:            id,
		Name:          id,
		Official:      provider.Official{Homepage: "https://example.com/"},
		Organizations: provider.Organizations{HuggingFace: hfOrgs},
		Identity:      provider.Identity{Strategy: provider.IdentityStrategyPublisher},
	}
}

func aggregator(id string) provider.Provider {
	return provider.Provider{
		SchemaVersion: provider.CurrentSchemaVersion,
		ID:            id,
		Name:          id,
		Official:      provider.Official{Homepage: "https://example.com/"},
	}
}

func TestResolvePrecedence(t *testing.T) {
	qwen := publisher("qwen", []string{"Qwen"})
	openai := aggregator("openai")

	tests := []struct {
		name       string
		model      registry.Model
		provider   provider.Provider
		wantSource string
		wantOK     bool
	}{
		{
			name:       "manual override wins",
			model:      registry.Model{ID: "qwen/qwen-max", Identity: &registry.IdentityMetadata{Source: SourceManual, Verified: true}, Identifiers: registry.ModelIdentifiers{HuggingFace: []string{"Qwen/Qwen-Max"}}},
			provider:   qwen,
			wantSource: SourceManual,
			wantOK:     true,
		},
		{
			name:       "authoritative hugging face organization",
			model:      registry.Model{ID: "qwen/qwen3-32b", Identifiers: registry.ModelIdentifiers{HuggingFace: []string{"Qwen/Qwen3-32B"}}},
			provider:   qwen,
			wantSource: SourceOfficialHuggingFace,
			wantOK:     true,
		},
		{
			name:       "unrelated hugging face organization does not verify",
			model:      registry.Model{ID: "qwen/qwen3-32b", Identifiers: registry.ModelIdentifiers{HuggingFace: []string{"someone/Qwen3-32B"}}},
			provider:   qwen,
			wantSource: SourceOpenRouter,
			wantOK:     false,
		},
		{
			name:       "official link verifies a closed model",
			model:      registry.Model{ID: "google/gemini-3-pro", Links: registry.ModelLinks{Official: "https://deepmind.google/models/gemini/"}},
			provider:   publisher("google", []string{"google"}),
			wantSource: SourceOfficial,
			wantOK:     true,
		},
		{
			name:       "publisher strategy leaves openrouter identity unverified",
			model:      registry.Model{ID: "qwen/qwen-max"},
			provider:   qwen,
			wantSource: SourceOpenRouter,
			wantOK:     false,
		},
		{
			name:       "aggregator strategy trusts openrouter",
			model:      registry.Model{ID: "openai/gpt-chat-latest"},
			provider:   openai,
			wantSource: SourceOpenRouter,
			wantOK:     true,
		},
	}
	for _, tt := range tests {
		got := Resolve(tt.model, tt.provider)
		if got.Source != tt.wantSource || got.Verified != tt.wantOK {
			t.Errorf("%s: Resolve() = %+v, want source=%s verified=%v", tt.name, got, tt.wantSource, tt.wantOK)
		}
	}
}

func TestProviderFor(t *testing.T) {
	providers := []provider.Provider{publisher("deepseek", []string{"deepseek-ai"}), aggregator("openai")}
	tests := []struct {
		name  string
		model registry.Model
		want  string
	}{
		{"developer", registry.Model{ID: "deepseek/deepseek-v4-flash", Developer: "DeepSeek"}, "deepseek"},
		{"directory", registry.Model{ID: "deepseek-v4-flash", FilePath: "models/deepseek/deepseek-v4-flash.yaml"}, "deepseek"},
		{"id prefix", registry.Model{ID: "openai/gpt-6"}, "openai"},
	}
	for _, tt := range tests {
		got, ok := ProviderFor(tt.model, providers)
		if !ok || got.ID != tt.want {
			t.Errorf("%s: ProviderFor() = %q, %v; want %q", tt.name, got.ID, ok, tt.want)
		}
	}
	if _, ok := ProviderFor(registry.Model{ID: "unknown/model"}, providers); ok {
		t.Fatal("unknown publisher must not resolve")
	}
}

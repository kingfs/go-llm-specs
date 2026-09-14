package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kingfs/go-llm-specs/internal/provider"
	"github.com/kingfs/go-llm-specs/internal/registry"
	"gopkg.in/yaml.v3"
)

func TestCollectOfficialIDsShapes(t *testing.T) {
	tests := []struct {
		name string
		api  provider.OfficialAPI
		body string
		want []string
	}{
		{
			name: "openai style data array",
			api:  provider.OfficialAPI{ItemsPath: "data"},
			body: `{"data":[{"id":"gpt-5"},{"id":"gpt-5-mini"}]}`,
			want: []string{"gpt-5", "gpt-5-mini"},
		},
		{
			name: "google style names with prefix",
			api:  provider.OfficialAPI{ItemsPath: "models", IDField: "name", IDPrefix: "models/"},
			body: `{"models":[{"name":"models/gemini-2.5-pro"},{"name":"models/gemini-2.5-flash"}]}`,
			want: []string{"gemini-2.5-flash", "gemini-2.5-pro"},
		},
		{
			name: "top level array",
			api:  provider.OfficialAPI{},
			body: `[{"id":"claude-sonnet-4-5"},{"id":"claude-sonnet-4-5"}]`,
			want: []string{"claude-sonnet-4-5"},
		},
		{
			name: "missing id field is skipped",
			api:  provider.OfficialAPI{ItemsPath: "data"},
			body: `{"data":[{"id":"grok-4"},{"name":"ignored"}]}`,
			want: []string{"grok-4"},
		},
	}
	for _, tt := range tests {
		got, err := collectOfficialIDs([]byte(tt.body), tt.api)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if len(got) != len(tt.want) {
			t.Fatalf("%s: got %#v, want %#v", tt.name, got, tt.want)
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Fatalf("%s: got %#v, want %#v", tt.name, got, tt.want)
			}
		}
	}
}

func TestFetchOfficialIDsAuth(t *testing.T) {
	var gotAuth, gotKey, gotVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		w.Write([]byte(`{"data":[{"id":"m"}]}`))
	}))
	defer server.Close()

	client := &http.Client{Timeout: time.Second}
	t.Setenv("TEST_KEY", "secret")

	if _, err := fetchOfficialIDs(context.Background(), client, provider.OfficialAPI{
		URL: server.URL, Env: "TEST_KEY", Auth: provider.OfficialAPIAuthBearer, ItemsPath: "data",
	}); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("bearer auth = %q", gotAuth)
	}

	if _, err := fetchOfficialIDs(context.Background(), client, provider.OfficialAPI{
		URL: server.URL, Env: "TEST_KEY", Auth: provider.OfficialAPIAuthHeader, ItemsPath: "data",
		Headers: map[string]string{"anthropic-version": "2023-06-01"},
	}); err != nil {
		t.Fatal(err)
	}
	if gotKey != "secret" || gotVersion != "2023-06-01" {
		t.Fatalf("header auth = %q, version = %q", gotKey, gotVersion)
	}

	var gotQuery string
	queryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("key")
		w.Write([]byte(`{"models":[{"name":"models/m"}]}`))
	}))
	defer queryServer.Close()
	if _, err := fetchOfficialIDs(context.Background(), client, provider.OfficialAPI{
		URL: queryServer.URL, Env: "TEST_KEY", Auth: provider.OfficialAPIAuthQuery, QueryParam: "key",
		ItemsPath: "models", IDField: "name", IDPrefix: "models/",
	}); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "secret" {
		t.Fatalf("query auth = %q", gotQuery)
	}
}

func writeProvider(t *testing.T, dir, id string, api provider.OfficialAPI) {
	t.Helper()
	p := provider.Provider{
		SchemaVersion: provider.CurrentSchemaVersion,
		ID:            id,
		Name:          id,
		Official:      provider.Official{Homepage: "https://" + id + ".example/"},
		Identity:      provider.Identity{Strategy: provider.IdentityStrategyPublisher, OfficialAPI: &api},
	}
	data, err := yaml.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeModel(t *testing.T, dir, id string) {
	t.Helper()
	prefix, suffix, _ := strings.Cut(id, "/")
	model := registry.Model{
		SchemaVersion: registry.CurrentSchemaVersion,
		ID:            id, Name: id, Provider: "acme", Developer: "acme", ContextLen: 8192,
	}
	path := filepath.Join(dir, prefix, suffix+".yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := registry.Save(path, model); err != nil {
		t.Fatal(err)
	}
}

func TestRunAppliesOfficialIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"data":[{"id":"acme-gpt-5"},{"id":"acme-gpt-5-20250101"},{"id":"acme-brand-new"}]}`))
	}))
	defer server.Close()

	providersDir := filepath.Join(t.TempDir(), "providers")
	modelsDir := filepath.Join(t.TempDir(), "models")
	writeProvider(t, providersDir, "acme", provider.OfficialAPI{
		URL: server.URL, Env: "ACME_API_KEY", Auth: provider.OfficialAPIAuthBearer, ItemsPath: "data",
	})
	writeModel(t, modelsDir, "acme/acme-gpt-5")

	t.Setenv("ACME_API_KEY", "test-key")
	output := filepath.Join(t.TempDir(), "report.json")
	if err := run(context.Background(), config{
		ProvidersDir: providersDir, ModelsDir: modelsDir, Output: output, Timeout: time.Second, Apply: true,
	}); err != nil {
		t.Fatal(err)
	}

	saved, err := registry.Load(filepath.Join(modelsDir, "acme", "acme-gpt-5.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Identifiers.Official) != 2 || saved.Identifiers.Official[0] != "acme-gpt-5" || saved.Identifiers.Official[1] != "acme-gpt-5-20250101" {
		t.Fatalf("official identifiers = %#v", saved.Identifiers.Official)
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var r report
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Providers) != 1 {
		t.Fatalf("providers = %#v", r.Providers)
	}
	pr := r.Providers[0]
	if pr.Status != "ok" || pr.OfficialModels != 3 || pr.Matched != 2 || pr.Applied != 2 {
		t.Fatalf("unexpected report: %#v", pr)
	}
	if len(pr.UnmatchedOfficial) != 1 || pr.UnmatchedOfficial[0] != "acme-brand-new" {
		t.Fatalf("unmatched official = %#v", pr.UnmatchedOfficial)
	}
	if len(pr.UnmatchedRegistry) != 0 {
		t.Fatalf("unmatched registry = %#v", pr.UnmatchedRegistry)
	}
}

func TestRunSkipsWithoutCredentials(t *testing.T) {
	providersDir := filepath.Join(t.TempDir(), "providers")
	modelsDir := filepath.Join(t.TempDir(), "models")
	writeProvider(t, providersDir, "acme", provider.OfficialAPI{
		URL: "https://acme.example/v1/models", Env: "ACME_API_KEY", Auth: provider.OfficialAPIAuthBearer, ItemsPath: "data",
	})
	writeModel(t, modelsDir, "acme/acme-gpt-5")

	t.Setenv("ACME_API_KEY", "")
	output := filepath.Join(t.TempDir(), "report.json")
	if err := run(context.Background(), config{
		ProvidersDir: providersDir, ModelsDir: modelsDir, Output: output, Timeout: time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var r report
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Providers) != 1 || r.Providers[0].Status != "skipped_no_credentials" {
		t.Fatalf("report = %#v", r.Providers)
	}
}

func TestIndexLookupAmbiguity(t *testing.T) {
	models := []registry.Model{
		{ID: "acme/acme-gpt-5", Developer: "acme"},
		{ID: "acme/acme-gpt-5-preview", Developer: "acme"},
	}
	providers := []provider.Provider{{
		SchemaVersion: provider.CurrentSchemaVersion, ID: "acme", Name: "acme",
		Official: provider.Official{Homepage: "https://acme.example/"},
		Identity: provider.Identity{Strategy: provider.IdentityStrategyPublisher,
			OfficialAPI: &provider.OfficialAPI{URL: "https://acme.example/v1/models"}},
	}}
	index := buildIndex(models, providers, providers[0])
	if position, ambiguous := index.lookup("acme-gpt-5-preview-20250101"); position >= 0 || !ambiguous {
		t.Fatalf("expected ambiguity, got position=%d ambiguous=%v", position, ambiguous)
	}
	if position, ambiguous := index.lookup("acme-gpt-5"); position != 0 || ambiguous {
		t.Fatalf("exact match failed: position=%d ambiguous=%v", position, ambiguous)
	}
	if position, ambiguous := index.lookup("acme-unknown"); position >= 0 || ambiguous {
		t.Fatalf("unknown identifier matched: position=%d ambiguous=%v", position, ambiguous)
	}
}

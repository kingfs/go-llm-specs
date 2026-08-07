package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/kingfs/go-llm-specs/internal/provider"
	"github.com/kingfs/go-llm-specs/internal/registry"
)

func TestNormalizePublisher(t *testing.T) {
	if got := normalize("Moonshot-AI"); got != "moonshotai" {
		t.Fatalf("normalize = %q", got)
	}
}

func TestDiscoverHFPaginates(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			fmt.Fprint(w, `[{"id":"Org/B","lastModified":"2026-08-02T00:00:00Z"}]`)
			return
		}
		w.Header().Set("Link", "<"+server.URL+"/api/models?page=2>; rel=\"next\"")
		fmt.Fprint(w, `[{"id":"Org/A","lastModified":"2026-08-03T00:00:00Z"}]`)
	}))
	defer server.Close()
	models, err := discoverHF(context.Background(), server.Client(), server.URL+"/api", "Org", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[1].ID != "Org/B" {
		t.Fatalf("unexpected models: %#v", models)
	}
}

func TestReconcilePreviousIdentityMatchesAfterCanonicalization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gemma.yaml")
	models := []registry.Model{{
		ID: "google/gemma-3n-e2b-it", Developer: "google", FilePath: path,
	}}
	if err := registry.Save(path, models[0]); err != nil {
		t.Fatal(err)
	}
	candidate := hfCandidate{
		ProviderID: "google", Organization: "google", RepositoryID: "google/gemma-3n-E2B-it",
		Status: "new", URL: "https://huggingface.co/google/gemma-3n-E2B-it",
	}
	previous := map[string]hfCandidate{candidateKey(candidate.Organization, candidate.RepositoryID): candidate}
	matches := map[string][]int{"google:" + normalize("gemma-3n-e2b-it"): {0}}
	known := map[string]bool{}
	if err := reconcilePreviousIdentityMatches(previous, matches, models, known, true); err != nil {
		t.Fatal(err)
	}
	got := previous[candidateKey(candidate.Organization, candidate.RepositoryID)]
	if got.Status != "identity_applied" || got.RegistryID != models[0].ID {
		t.Fatalf("candidate was not reconciled: %#v", got)
	}
	saved, err := registry.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Links.ModelCard != candidate.URL || len(saved.Identifiers.HuggingFace) != 1 || saved.Identifiers.HuggingFace[0] != candidate.RepositoryID {
		t.Fatalf("model identity was not persisted: %#v", saved)
	}
}

func TestMaterializeCandidateIsExcludedUntilReady(t *testing.T) {
	root := t.TempDir()
	r := report{HuggingFaceCandidates: []hfCandidate{{
		ProviderID: "example", Organization: "Example", RepositoryID: "Example/New-Model",
		Status: "new", URL: "https://huggingface.co/Example/New-Model", PipelineTag: "text-generation",
	}}}
	p := provider.Provider{ID: "example", Name: "Example"}
	if err := materializeCandidates(&r, []provider.Provider{p}, nil, config{ModelsDir: root, Limit: 1}); err != nil {
		t.Fatal(err)
	}
	model, err := registry.Load(filepath.Join(root, "example", "New-Model.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if model.Lifecycle != "candidate" || readyForPromotion(model) {
		t.Fatalf("unexpected candidate: %#v", model)
	}
}

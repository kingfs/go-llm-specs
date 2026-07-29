package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kingfs/go-llm-specs/internal/registry"
)

func TestEnrichOpenRouter(t *testing.T) {
	model := registry.Model{ID: "qwen/example"}
	changed := enrichOpenRouter(&model, openRouterModel{
		CanonicalSlug:       "qwen/example-1",
		SupportedParameters: []string{"tools", "reasoning", "tools"},
		Architecture: openRouterArchitecture{
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"text"},
		},
		Reasoning: &openRouterReasoning{DefaultEnabled: true},
	}, time.Unix(1, 0).UTC())
	if !changed || model.Upstream.OpenRouter == nil || model.Reasoning == nil {
		t.Fatalf("model was not enriched: %#v", model)
	}
	if got := model.Upstream.OpenRouter.SupportedParameters; len(got) != 2 || got[0] != "reasoning" {
		t.Fatalf("parameters not normalized: %v", got)
	}
}

func TestResolveHuggingFaceByDeterministicSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
          {"id":"Other/Qwen3.6-27B","pipeline_tag":"text-generation"},
          {"id":"Qwen/Qwen3.6-27B","pipeline_tag":"image-text-to-text","config":{"model_type":"qwen3_5","architectures":["QwenModel"]},"cardData":{"license":"apache-2.0"}}
        ]`))
	}))
	defer server.Close()

	hf, status, err := resolveHuggingFace(context.Background(), server.Client(), retryPolicy{Attempts: 1}, server.URL, registry.Model{
		ID:       "qwen/qwen3.6-27b",
		Provider: "Qwen",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if status != "matched" || hf == nil || hf.ID != "Qwen/Qwen3.6-27B" {
		t.Fatalf("unexpected resolution: status=%s model=%#v", status, hf)
	}
}

func TestResolveHuggingFaceRejectsAmbiguousMatches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"Qwen/Example"},{"id":"qwen/example"}]`))
	}))
	defer server.Close()

	hf, status, err := resolveHuggingFace(context.Background(), server.Client(), retryPolicy{Attempts: 1}, server.URL, registry.Model{ID: "qwen/example", Provider: "Qwen"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if status != "ambiguous" || hf != nil {
		t.Fatalf("ambiguous result was accepted: status=%s model=%#v", status, hf)
	}
}

func TestSelectModelsProtectsLegacyByDefault(t *testing.T) {
	models := []registry.Model{
		{ID: "old/model", SchemaVersion: 0},
		{ID: "new/model", SchemaVersion: 2},
	}
	selected := selectModels(models, config{NewOnly: true})
	if len(selected) != 1 || selected[0].ID != "new/model" {
		t.Fatalf("legacy selection protection failed: %#v", selected)
	}
	selected = selectModels(models, config{NewOnly: true, Model: "old/model"})
	if len(selected) != 1 || selected[0].ID != "old/model" {
		t.Fatalf("explicit legacy selection failed: %#v", selected)
	}
	selected = selectModels(models, config{NewOnly: false, UpgradeV1: false})
	if len(selected) != 1 || selected[0].ID != "new/model" {
		t.Fatalf("new-only=false bypassed upgrade guard: %#v", selected)
	}
	selected = selectModels(models, config{NewOnly: false, UpgradeV1: true})
	if len(selected) != 2 {
		t.Fatalf("explicit upgrade did not select legacy records: %#v", selected)
	}
}

func TestEnrichmentPreservesSourceExtensionFields(t *testing.T) {
	model := registry.Model{
		Upstream: registry.UpstreamMetadata{
			OpenRouter:  &registry.OpenRouterMetadata{Extra: map[string]any{"future": "or"}},
			HuggingFace: &registry.HuggingFaceMetadata{Extra: map[string]any{"future": "hf"}},
		},
		Reasoning: &registry.ReasoningMetadata{Extra: map[string]any{"future": "reasoning"}},
	}
	enrichOpenRouter(&model, openRouterModel{Reasoning: &openRouterReasoning{}}, time.Unix(1, 0).UTC())
	enrichHuggingFace(&model, hfModel{ID: "Qwen/Test"}, time.Unix(1, 0).UTC())
	if model.Upstream.OpenRouter.Extra["future"] != "or" || model.Upstream.HuggingFace.Extra["future"] != "hf" || model.Reasoning.Extra["future"] != "reasoning" {
		t.Fatalf("extension fields were lost: %#v", model)
	}
}

func TestRetryHonorsTransientFailure(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "busy", http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"id":"Qwen/Test"}`))
	}))
	defer server.Close()
	hf, err := fetchHF(context.Background(), server.Client(), retryPolicy{Attempts: 2}, server.URL, "Qwen/Test")
	if err != nil || hf == nil || calls.Load() != 2 {
		t.Fatalf("retry failed: model=%#v calls=%d err=%v", hf, calls.Load(), err)
	}
}

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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
	if model.Lifecycle != "candidate" || readyForPromotion(model, nil) {
		t.Fatalf("unexpected candidate: %#v", model)
	}
}

func TestReadyForPromotionRequiresCorroboratedIdentity(t *testing.T) {
	providers := []provider.Provider{{
		SchemaVersion: provider.CurrentSchemaVersion,
		ID:            "qwen",
		Name:          "Qwen",
		Official:      provider.Official{Homepage: "https://qwen.ai/"},
		Organizations: provider.Organizations{HuggingFace: []string{"Qwen"}},
		Identity:      provider.Identity{Strategy: provider.IdentityStrategyPublisher, RequireCorroboration: true},
	}}
	model := registry.Model{
		ID: "qwen/qwen-max", Developer: "qwen", ContextLen: 4096, Description: "A model.",
		Features: []string{"CapChat"},
		Upstream: registry.UpstreamMetadata{HuggingFace: &registry.HuggingFaceMetadata{ID: "Qwen/Qwen-Max"}},
	}
	if readyForPromotion(model, providers) {
		t.Fatal("an openrouter-only identity must not promote for a publisher")
	}
	model.Identifiers.HuggingFace = []string{"Qwen/Qwen-Max"}
	if !readyForPromotion(model, providers) {
		t.Fatal("an official organization repository must corroborate identity")
	}
}

func TestReadyForPromotionAcceptsOfficialAPIIdentity(t *testing.T) {
	providers := []provider.Provider{{
		SchemaVersion: provider.CurrentSchemaVersion,
		ID:            "openai",
		Name:          "OpenAI",
		Official:      provider.Official{Homepage: "https://openai.com/"},
		Identity: provider.Identity{
			Strategy: provider.IdentityStrategyPublisher, RequireCorroboration: true,
			OfficialAPI: &provider.OfficialAPI{URL: "https://api.openai.com/v1/models"},
		},
	}}
	model := registry.Model{
		ID: "openai/gpt-6-astra", Developer: "openai", ContextLen: 400000, Description: "A model.",
		Features: []string{"CapChat"},
	}
	if readyForPromotion(model, providers) {
		t.Fatal("an uncorroborated closed model must not promote")
	}
	model.Identifiers.Official = []string{"gpt-6-astra"}
	if !readyForPromotion(model, providers) {
		t.Fatal("an official API identifier must corroborate identity without a repository")
	}
}

func TestMaterializeCandidatesRefusesOutOfScopeRepository(t *testing.T) {
	root := t.TempDir()
	r := report{HuggingFaceCandidates: []hfCandidate{{
		ProviderID: "microsoft", Organization: "microsoft",
		RepositoryID: "microsoft/Dayhoff-170M-GRS-SS-14000",
		Status:       "new", URL: "https://huggingface.co/microsoft/Dayhoff-170M-GRS-SS-14000",
		PipelineTag: "text-generation", Tags: []string{"protein-generation", "endpoints_compatible"},
	}}}
	p := provider.Provider{ID: "microsoft", Name: "Microsoft"}
	if err := materializeCandidates(&r, []provider.Provider{p}, nil, config{ModelsDir: root, Limit: 5}); err != nil {
		t.Fatal(err)
	}
	got := r.HuggingFaceCandidates[0]
	if got.Status != statusOutOfScope {
		t.Fatalf("status = %q, want %q", got.Status, statusOutOfScope)
	}
	if got.ScopeReason != "out-of-scope-domain:protein" {
		t.Fatalf("scope reason = %q", got.ScopeReason)
	}
	if _, err := registry.Load(filepath.Join(root, "microsoft", "Dayhoff-170M-GRS-SS-14000.yaml")); err == nil {
		t.Fatal("an out-of-scope repository must not be materialized")
	}
}

func TestMaterializeCandidatesRefusesPackagingRepository(t *testing.T) {
	root := t.TempDir()
	r := report{HuggingFaceCandidates: []hfCandidate{{
		ProviderID: "microsoft", Organization: "microsoft", RepositoryID: "microsoft/Fara-7B-onnx",
		Status: "new", URL: "https://huggingface.co/microsoft/Fara-7B-onnx",
		PipelineTag: "image-text-to-text", Tags: []string{"onnx", "endpoints_compatible"},
	}}}
	p := provider.Provider{ID: "microsoft", Name: "Microsoft"}
	if err := materializeCandidates(&r, []provider.Provider{p}, nil, config{ModelsDir: root, Limit: 5}); err != nil {
		t.Fatal(err)
	}
	if got := r.HuggingFaceCandidates[0].Status; got != statusPackaging {
		t.Fatalf("status = %q, want %q", got, statusPackaging)
	}
	if _, err := registry.Load(filepath.Join(root, "microsoft", "Fara-7B-onnx.yaml")); err == nil {
		t.Fatal("a packaging repository must not be materialized")
	}
}

func TestMaterializeCandidatesKeepsOutOfPolicyTasksQueued(t *testing.T) {
	// Ranking and speech models are within catalog scope but outside the
	// automatic intake policy, so they stay queued instead of being refused.
	root := t.TempDir()
	r := report{HuggingFaceCandidates: []hfCandidate{
		{
			ProviderID: "qwen", Organization: "Qwen", RepositoryID: "Qwen/Qwen3-Reranker-0.6B",
			Status: "new", URL: "https://huggingface.co/Qwen/Qwen3-Reranker-0.6B", PipelineTag: "text-ranking",
		},
		{
			ProviderID: "microsoft", Organization: "microsoft", RepositoryID: "microsoft/VibeVoice-ASR",
			Status: "new", URL: "https://huggingface.co/microsoft/VibeVoice-ASR", PipelineTag: "automatic-speech-recognition",
		},
		{
			ProviderID: "qwen", Organization: "Qwen", RepositoryID: "Qwen/Qwen3.5-32B",
			Status: "new", URL: "https://huggingface.co/Qwen/Qwen3.5-32B", PipelineTag: "text-generation",
		},
	}}
	providers := []provider.Provider{{ID: "qwen", Name: "Qwen"}, {ID: "microsoft", Name: "Microsoft"}}
	if err := materializeCandidates(&r, providers, nil, config{ModelsDir: root, Limit: 5}); err != nil {
		t.Fatal(err)
	}
	byRepo := map[string]hfCandidate{}
	for _, candidate := range r.HuggingFaceCandidates {
		byRepo[candidate.RepositoryID] = candidate
	}
	if got := byRepo["Qwen/Qwen3-Reranker-0.6B"].Status; got != "new" {
		t.Fatalf("reranker status = %q, want new", got)
	}
	if got := byRepo["microsoft/VibeVoice-ASR"].Status; got != "new" {
		t.Fatalf("asr status = %q, want new", got)
	}
	if _, err := registry.Load(filepath.Join(root, "qwen", "Qwen3.5-32B.yaml")); err != nil {
		t.Fatalf("an in-policy text model must be materialized: %v", err)
	}
}

func TestFeaturesForPipeline(t *testing.T) {
	cases := map[string][]string{
		"feature-extraction":        {"CapEmbedding", "ModalityTextIn"},
		"image-text-to-text":        {"ModalityImageIn", "ModalityTextIn", "ModalityTextOut"},
		"visual-question-answering": {"ModalityImageIn", "ModalityTextIn", "ModalityTextOut"},
		"text-generation":           {"ModalityTextIn", "ModalityTextOut"},
	}
	for pipeline, want := range cases {
		got := featuresForPipeline(pipeline)
		if len(got) != len(want) {
			t.Fatalf("featuresForPipeline(%q) = %#v, want %#v", pipeline, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("featuresForPipeline(%q) = %#v, want %#v", pipeline, got, want)
			}
		}
	}
}

func TestMaterializeCandidatesRecordsScopeDecisionForExistingRecords(t *testing.T) {
	// Discovery records the scope decision even when an older run already
	// materialized the repository, but it must never rewrite the local record.
	root := t.TempDir()
	path := filepath.Join(root, "google", "medgemma-1.5-4b-it.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := registry.Model{
		ID: "google/medgemma-1.5-4b-it", Name: "medgemma-1.5-4b-it", Developer: "google",
		Lifecycle: "candidate", Features: []string{"ModalityImageIn", "ModalityTextIn", "ModalityTextOut"},
		Upstream: registry.UpstreamMetadata{HuggingFace: &registry.HuggingFaceMetadata{
			ID: "google/medgemma-1.5-4b-it", PipelineTag: "image-text-to-text", Tags: []string{"medical", "radiology"},
		}},
	}
	if err := registry.Save(path, existing); err != nil {
		t.Fatal(err)
	}
	models, err := registry.Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	r := report{HuggingFaceCandidates: []hfCandidate{{
		ProviderID: "google", Organization: "google", RepositoryID: "google/medgemma-1.5-4b-it",
		Status: "materialized", URL: "https://huggingface.co/google/medgemma-1.5-4b-it",
		PipelineTag: "image-text-to-text", Tags: []string{"medical", "radiology"},
	}}}
	p := provider.Provider{ID: "google", Name: "Google"}
	if err := materializeCandidates(&r, []provider.Provider{p}, models, config{ModelsDir: root, Limit: 5}); err != nil {
		t.Fatal(err)
	}
	got := r.HuggingFaceCandidates[0]
	if got.Status != statusOutOfScope || got.ScopeReason != "out-of-scope-domain:biomedicine" {
		t.Fatalf("scope decision was not recorded: %#v", got)
	}
	saved, err := registry.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Lifecycle != "candidate" || len(saved.Features) != 3 {
		t.Fatalf("existing record was rewritten: %#v", saved)
	}
}

func TestEligiblePipelineUsesIntakePolicy(t *testing.T) {
	inPolicy := []string{"text-generation", "text2text-generation", "image-text-to-text", "visual-question-answering", "feature-extraction"}
	for _, pipeline := range inPolicy {
		if !eligiblePipeline(pipeline, nil) {
			t.Fatalf("eligiblePipeline(%q) = false, want true", pipeline)
		}
	}
	// Out-of-scope tasks and in-scope tasks outside the automatic intake policy
	// are both refused intake; only the reason differs.
	outOfPolicy := []string{"image-classification", "image-segmentation", "object-detection", "text-to-3d", "robotics", "graph-ml", "fill-mask", "text-ranking", "automatic-speech-recognition", "text-to-speech", "translation"}
	for _, pipeline := range outOfPolicy {
		if eligiblePipeline(pipeline, nil) {
			t.Fatalf("eligiblePipeline(%q) = true, want false", pipeline)
		}
	}
	if eligiblePipeline("text-generation", []string{"gguf"}) {
		t.Fatal("a packaging tag must refuse intake even for an in-policy task")
	}
}

func TestClassifyQueueDoesNotDependOnMaterializationBudget(t *testing.T) {
	// The queue is classified as a whole: a repository that sorts behind the
	// candidate which consumed a one-record budget must still carry its reason,
	// otherwise a run with a small budget would leave refused repositories
	// looking eligible.
	root := t.TempDir()
	outOfScope := hfCandidate{
		ProviderID: "microsoft", Organization: "microsoft", RepositoryID: "microsoft/Dayhoff-170M-GRS-SS-14000",
		Status: "new", URL: "https://huggingface.co/microsoft/Dayhoff-170M-GRS-SS-14000",
		PipelineTag: "text-generation", Tags: []string{"protein-generation"},
	}
	refusedOld := hfCandidate{
		ProviderID: "nvidia", Organization: "nvidia", RepositoryID: "nvidia/resnet-50",
		Status: "new", URL: "https://huggingface.co/nvidia/resnet-50", PipelineTag: "image-classification",
	}
	eligible := hfCandidate{
		ProviderID: "qwen", Organization: "Qwen", RepositoryID: "Qwen/Qwen3.5-32B",
		Status: "new", URL: "https://huggingface.co/Qwen/Qwen3.5-32B", PipelineTag: "text-generation",
	}
	r := report{HuggingFaceCandidates: []hfCandidate{outOfScope, refusedOld, eligible}}
	providers := []provider.Provider{{ID: "microsoft", Name: "Microsoft"}, {ID: "nvidia", Name: "NVIDIA"}, {ID: "qwen", Name: "Qwen"}}
	if err := materializeCandidates(&r, providers, nil, config{ModelsDir: root, Limit: 1}); err != nil {
		t.Fatal(err)
	}
	byRepo := map[string]hfCandidate{}
	for _, candidate := range r.HuggingFaceCandidates {
		byRepo[candidate.RepositoryID] = candidate
	}
	if got := byRepo["microsoft/Dayhoff-170M-GRS-SS-14000"]; got.Status != statusOutOfScope || got.ScopeReason != "out-of-scope-domain:protein" {
		t.Fatalf("domain record was not classified: %#v", got)
	}
	if got := byRepo["nvidia/resnet-50"]; got.Status != statusOutOfScope || got.ScopeReason != "out-of-scope-pipeline:image-classification" {
		t.Fatalf("pipeline refusal was not classified: %#v", got)
	}
	if got := byRepo["Qwen/Qwen3.5-32B"].Status; got != "materialized" {
		t.Fatalf("eligible candidate status = %q, want materialized", got)
	}
}

func TestClassifyQueueClearsRegistryLinkToRemovedRecord(t *testing.T) {
	// A candidate whose materialized record was deleted must not keep pointing
	// at a record that no longer exists.
	r := report{HuggingFaceCandidates: []hfCandidate{{
		ProviderID: "microsoft", Organization: "microsoft", RepositoryID: "microsoft/Dayhoff-3b-UR90-10",
		Status: "materialized", RegistryID: "microsoft/dayhoff-3b-ur90-10",
		URL: "https://huggingface.co/microsoft/Dayhoff-3b-UR90-10", PipelineTag: "text-generation",
		Tags: []string{"protein-generation", "dayhoff"},
	}}}
	classifyQueue(&r, []provider.Provider{{ID: "microsoft", Name: "Microsoft"}}, nil)
	got := r.HuggingFaceCandidates[0]
	if got.Status != statusOutOfScope || got.RegistryID != "" {
		t.Fatalf("stale registry link survived: %#v", got)
	}

	// A record that still exists keeps its link and its scope decision.
	r = report{HuggingFaceCandidates: []hfCandidate{{
		ProviderID: "google", Organization: "google", RepositoryID: "google/medgemma-1.5-4b-it",
		Status: "materialized", RegistryID: "google/medgemma-1.5-4b-it",
		URL: "https://huggingface.co/google/medgemma-1.5-4b-it", PipelineTag: "image-text-to-text",
		Tags: []string{"medical", "radiology"},
	}}}
	models := []registry.Model{{ID: "google/medgemma-1.5-4b-it"}}
	classifyQueue(&r, []provider.Provider{{ID: "google", Name: "Google"}}, models)
	got = r.HuggingFaceCandidates[0]
	if got.Status != statusOutOfScope || got.RegistryID != "google/medgemma-1.5-4b-it" {
		t.Fatalf("kept record lost its link: %#v", got)
	}
}

func TestClassifyQueueRegistersRepositoryClaimedByExistingRecord(t *testing.T) {
	// An aggregator record can already cite the repository that discovery later
	// finds in the publisher's own organization. The repository then belongs to
	// that record: writing another one would duplicate the same model under a
	// second ID, as happened for qwen/qwen3.8-flash.
	candidate := hfCandidate{
		ProviderID: "qwen", Organization: "Qwen", RepositoryID: "Qwen/Qwen3.8-Flash-Next",
		Status: "new", URL: "https://huggingface.co/Qwen/Qwen3.8-Flash-Next",
		PipelineTag: "image-text-to-text",
	}
	r := report{HuggingFaceCandidates: []hfCandidate{candidate}}
	models := []registry.Model{{
		ID:          "qwen/qwen3.8-flash",
		Identifiers: registry.ModelIdentifiers{HuggingFace: []string{"Qwen/Qwen3.8-Flash-Next"}},
	}}
	classifyQueue(&r, []provider.Provider{{ID: "qwen", Name: "Qwen"}}, models)
	got := r.HuggingFaceCandidates[0]
	if got.Status != "registered" || got.RegistryID != "qwen/qwen3.8-flash" {
		t.Fatalf("claimed repository was not registered: %#v", got)
	}

	// The upstream cache alone also proves the claim, and a candidate whose own
	// record still exists keeps its materialized status.
	r = report{HuggingFaceCandidates: []hfCandidate{{
		ProviderID: "qwen", Organization: "Qwen", RepositoryID: "Qwen/Qwen3.8-Flash-Next",
		Status: "materialized", RegistryID: "qwen/qwen3.8-flash-next",
		URL: "https://huggingface.co/Qwen/Qwen3.8-Flash-Next", PipelineTag: "image-text-to-text",
	}}}
	models = []registry.Model{
		{
			ID:          "qwen/qwen3.8-flash",
			Upstream:    registry.UpstreamMetadata{HuggingFace: &registry.HuggingFaceMetadata{ID: "Qwen/Qwen3.8-Flash-Next"}},
			Identifiers: registry.ModelIdentifiers{HuggingFace: []string{"Qwen/Qwen3.8-Flash-Next"}},
		},
		{ID: "qwen/qwen3.8-flash-next", Identifiers: registry.ModelIdentifiers{HuggingFace: []string{"Qwen/Qwen3.8-Flash-Next"}}},
	}
	classifyQueue(&r, []provider.Provider{{ID: "qwen", Name: "Qwen"}}, models)
	got = r.HuggingFaceCandidates[0]
	if got.Status != "materialized" || got.RegistryID != "qwen/qwen3.8-flash-next" {
		t.Fatalf("live materialized link was rewritten: %#v", got)
	}
}

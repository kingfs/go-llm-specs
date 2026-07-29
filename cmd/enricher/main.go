package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/kingfs/go-llm-specs/internal/registry"
)

const defaultHuggingFaceAPI = "https://huggingface.co/api"

type config struct {
	ModelsDir      string
	OpenRouterJSON string
	HuggingFaceAPI string
	Source         string
	Model          string
	Provider       string
	NewOnly        bool
	DryRun         bool
	Timeout        time.Duration
	UpgradeV1      bool
	Limit          int
	Delay          time.Duration
	Retries        int
	RetryBackoff   time.Duration
	Checkpoint     string
	FailureReport  string
	Allowlist      string
}

type openRouterResponse struct {
	Data []openRouterModel `json:"data"`
}

type openRouterModel struct {
	ID                  string                 `json:"id"`
	CanonicalSlug       string                 `json:"canonical_slug"`
	HuggingFaceID       string                 `json:"hugging_face_id"`
	KnowledgeCutoff     string                 `json:"knowledge_cutoff"`
	SupportedParameters []string               `json:"supported_parameters"`
	Architecture        openRouterArchitecture `json:"architecture"`
	Reasoning           *openRouterReasoning   `json:"reasoning"`
}

type openRouterArchitecture struct {
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`
}

type openRouterReasoning struct {
	Mandatory        bool     `json:"mandatory"`
	DefaultEnabled   bool     `json:"default_enabled"`
	DefaultEffort    string   `json:"default_effort"`
	SupportedEfforts []string `json:"supported_efforts"`
}

type hfModel struct {
	ID          string         `json:"id"`
	SHA         string         `json:"sha"`
	PipelineTag string         `json:"pipeline_tag"`
	Tags        []string       `json:"tags"`
	Config      hfConfig       `json:"config"`
	CardData    map[string]any `json:"cardData"`
	Repository  hfRepository
}

type hfRepository struct {
	ConfigContextLength     int
	TokenizerModelMaxLength int
	ProcessorClass          string
	ChatTemplateSHA256      string
	Files                   []string
}

type hfConfig struct {
	Architectures []string `json:"architectures"`
	ModelType     string   `json:"model_type"`
}

type retryPolicy struct {
	Attempts int
	Backoff  time.Duration
}

func main() {
	cfg := parseFlags()
	if err := run(context.Background(), cfg); err != nil {
		log.Fatal(err)
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.ModelsDir, "models-dir", "models", "directory containing model YAML files")
	flag.StringVar(&cfg.OpenRouterJSON, "openrouter-json", "data/models.json", "cached OpenRouter models response")
	flag.StringVar(&cfg.HuggingFaceAPI, "huggingface-api", defaultHuggingFaceAPI, "Hugging Face API base URL")
	flag.StringVar(&cfg.Source, "source", "all", "enrichment source: all, openrouter, or huggingface")
	flag.StringVar(&cfg.Model, "model", "", "explicit model ID to enrich")
	flag.StringVar(&cfg.Provider, "provider", "", "optional provider filter")
	flag.BoolVar(&cfg.NewOnly, "new-only", true, "only enrich schema-v2 records unless -model is set")
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "report changes without writing YAML")
	flag.DurationVar(&cfg.Timeout, "timeout", 20*time.Second, "HTTP request timeout")
	flag.BoolVar(&cfg.UpgradeV1, "upgrade-v1", false, "allow selected legacy records to be promoted to schema v2")
	flag.IntVar(&cfg.Limit, "limit", 0, "maximum number of models to process (0 means unlimited)")
	flag.DurationVar(&cfg.Delay, "delay", 0, "delay between models")
	flag.IntVar(&cfg.Retries, "retries", 3, "HTTP retries for rate limits and transient failures")
	flag.DurationVar(&cfg.RetryBackoff, "retry-backoff", time.Second, "initial exponential retry delay")
	flag.StringVar(&cfg.Checkpoint, "checkpoint", "", "optional JSON checkpoint used to resume a batch")
	flag.StringVar(&cfg.FailureReport, "failure-report", "", "optional JSON report for per-model failures")
	flag.StringVar(&cfg.Allowlist, "allowlist", "", "optional newline-delimited model ID allowlist")
	flag.Parse()
	return cfg
}

func run(ctx context.Context, cfg config) error {
	if cfg.Source != "all" && cfg.Source != "openrouter" && cfg.Source != "huggingface" {
		return fmt.Errorf("unsupported source %q", cfg.Source)
	}
	if cfg.Limit < 0 || cfg.Retries < 0 || cfg.Delay < 0 || cfg.RetryBackoff < 0 {
		return fmt.Errorf("limit, retries, delay, and retry-backoff cannot be negative")
	}
	models, err := registry.Scan(cfg.ModelsDir)
	if err != nil {
		return err
	}
	selected := selectModels(models, cfg)
	allowlist, err := loadAllowlist(cfg.Allowlist)
	if err != nil {
		return err
	}
	selected = filterAllowlist(selected, allowlist)
	completed, err := loadCheckpoint(cfg.Checkpoint)
	if err != nil {
		return err
	}
	selected = filterCompleted(selected, completed)
	if cfg.Limit > 0 && len(selected) > cfg.Limit {
		selected = selected[:cfg.Limit]
	}
	if len(selected) == 0 {
		log.Println("No models selected for enrichment")
		return nil
	}

	var upstream map[string]openRouterModel
	if cfg.Source == "all" || cfg.Source == "openrouter" || cfg.Source == "huggingface" {
		upstream, err = loadOpenRouter(cfg.OpenRouterJSON)
		if err != nil && (cfg.Source == "all" || cfg.Source == "openrouter") {
			return err
		}
	}
	hfClient := &http.Client{Timeout: cfg.Timeout}
	retry := retryPolicy{Attempts: cfg.Retries + 1, Backoff: cfg.RetryBackoff}
	changed := 0
	needsReview := 0
	failures := make(map[string]string)
	for i := range selected {
		if i > 0 && cfg.Delay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(cfg.Delay):
			}
		}
		model := &selected[i]
		orModel, hasOpenRouter := upstream[model.ID]
		modelChanged := false
		if !model.IsV2() && (cfg.Model != "" || cfg.UpgradeV1) {
			now := time.Now().UTC()
			model.SchemaVersion = registry.CurrentSchemaVersion
			if model.DiscoveredAt == nil {
				model.DiscoveredAt = &now
			}
			modelChanged = true
		}
		if (cfg.Source == "all" || cfg.Source == "openrouter") && hasOpenRouter {
			modelChanged = enrichOpenRouter(model, orModel, time.Now().UTC()) || modelChanged
		}
		if cfg.Source == "all" || cfg.Source == "huggingface" {
			hfID := ""
			if model.Upstream.HuggingFace != nil {
				hfID = model.Upstream.HuggingFace.ID
			}
			if hfID == "" && hasOpenRouter {
				hfID = orModel.HuggingFaceID
			}
			hf, status, err := resolveHuggingFace(ctx, hfClient, retry, cfg.HuggingFaceAPI, *model, hfID)
			if err != nil {
				failures[model.ID] = err.Error()
				log.Printf("%s: Hugging Face enrichment failed: %v", model.ID, err)
			} else if status == "ambiguous" {
				needsReview++
				log.Printf("%s: ambiguous Hugging Face match; review required", model.ID)
			} else if hf != nil {
				if err := enrichHFRepository(ctx, hfClient, retry, cfg.HuggingFaceAPI, hf); err != nil {
					failures[model.ID] = err.Error()
					log.Printf("%s: Hugging Face repository enrichment failed: %v", model.ID, err)
				}
				modelChanged = enrichHuggingFace(model, *hf, time.Now().UTC()) || modelChanged
			}
		}
		if modelChanged {
			changed++
		}
		if modelChanged && cfg.DryRun {
			log.Printf("dry-run: would update %s", model.ID)
		} else if modelChanged {
			if err := registry.Save(model.FilePath, *model); err != nil {
				return err
			}
			log.Printf("updated %s", model.ID)
		}
		if _, failed := failures[model.ID]; !failed {
			completed[model.ID] = true
			if err := saveCheckpoint(cfg.Checkpoint, completed); err != nil {
				return err
			}
		}
	}
	if err := writeFailureReport(cfg.FailureReport, failures); err != nil {
		return err
	}
	log.Printf("Enrichment complete: selected=%d changed=%d needs_review=%d", len(selected), changed, needsReview)
	return nil
}

func selectModels(models []registry.Model, cfg config) []registry.Model {
	selected := make([]registry.Model, 0)
	for _, model := range models {
		if cfg.Model != "" && !strings.EqualFold(model.ID, cfg.Model) {
			continue
		}
		if cfg.Provider != "" && !strings.EqualFold(model.Provider, cfg.Provider) {
			continue
		}
		if !model.IsV2() && cfg.Model == "" && (cfg.NewOnly || !cfg.UpgradeV1) {
			continue
		}
		selected = append(selected, model)
	}
	return selected
}

func loadOpenRouter(path string) (map[string]openRouterModel, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var response openRouterResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	models := make(map[string]openRouterModel, len(response.Data))
	for _, model := range response.Data {
		models[model.ID] = model
	}
	return models, nil
}

func enrichOpenRouter(model *registry.Model, upstream openRouterModel, fetchedAt time.Time) bool {
	before, _ := json.Marshal(model.Upstream.OpenRouter)
	reasoningBefore, _ := json.Marshal(model.Reasoning)
	var extra map[string]any
	if model.Upstream.OpenRouter != nil {
		extra = model.Upstream.OpenRouter.Extra
	}
	model.Upstream.OpenRouter = &registry.OpenRouterMetadata{
		CanonicalSlug:       upstream.CanonicalSlug,
		HuggingFaceID:       upstream.HuggingFaceID,
		SupportedParameters: sortedUnique(upstream.SupportedParameters),
		InputModalities:     sortedUnique(upstream.Architecture.InputModalities),
		OutputModalities:    sortedUnique(upstream.Architecture.OutputModalities),
		KnowledgeCutoff:     upstream.KnowledgeCutoff,
		FetchedAt:           &fetchedAt,
		Extra:               extra,
	}
	if upstream.Reasoning != nil {
		var reasoningExtra map[string]any
		if model.Reasoning != nil {
			reasoningExtra = model.Reasoning.Extra
		}
		model.Reasoning = &registry.ReasoningMetadata{
			Supported:        true,
			Mandatory:        upstream.Reasoning.Mandatory,
			DefaultEnabled:   upstream.Reasoning.DefaultEnabled,
			DefaultEffort:    upstream.Reasoning.DefaultEffort,
			SupportedEfforts: sortedUnique(upstream.Reasoning.SupportedEfforts),
			Extra:            reasoningExtra,
		}
	}
	after, _ := json.Marshal(model.Upstream.OpenRouter)
	reasoningAfter, _ := json.Marshal(model.Reasoning)
	return !stringEqualIgnoringFetchedAt(before, after) || !stringEqualIgnoringFetchedAt(reasoningBefore, reasoningAfter)
}

func resolveHuggingFace(ctx context.Context, client *http.Client, retry retryPolicy, base string, model registry.Model, explicitID string) (*hfModel, string, error) {
	if explicitID != "" {
		hf, err := fetchHF(ctx, client, retry, base, explicitID)
		return hf, "exact", err
	}
	query := modelSuffix(model.ID)
	values := url.Values{"search": []string{query}, "limit": []string{"20"}, "full": []string{"true"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/models?"+values.Encode(), nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := doWithRetry(ctx, client, retry, req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("Hugging Face search returned %s", resp.Status)
	}
	var candidates []hfModel
	if err := json.NewDecoder(resp.Body).Decode(&candidates); err != nil {
		return nil, "", err
	}
	want := normalizeIdentifier(query)
	matches := make([]hfModel, 0)
	for _, candidate := range candidates {
		if normalizeIdentifier(modelSuffix(candidate.ID)) == want && providerMatches(model.Provider, candidate.ID) {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 0 {
		return nil, "not_found", nil
	}
	if len(matches) > 1 {
		return nil, "ambiguous", nil
	}
	return &matches[0], "matched", nil
}

func fetchHF(ctx context.Context, client *http.Client, retry retryPolicy, base, id string) (*hfModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/models/"+id, nil)
	if err != nil {
		return nil, err
	}
	resp, err := doWithRetry(ctx, client, retry, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Hugging Face model request returned %s", resp.Status)
	}
	var model hfModel
	if err := json.NewDecoder(resp.Body).Decode(&model); err != nil {
		return nil, err
	}
	return &model, nil
}

func enrichHFRepository(ctx context.Context, client *http.Client, retry retryPolicy, apiBase string, model *hfModel) error {
	if model.ID == "" {
		return errors.New("Hugging Face model ID is empty")
	}
	revision := model.SHA
	if revision == "" {
		revision = "main"
	}
	rawBase := strings.TrimSuffix(strings.TrimRight(apiBase, "/"), "/api")
	for _, name := range []string{"config.json", "tokenizer_config.json", "preprocessor_config.json"} {
		target := rawBase + "/" + model.ID + "/resolve/" + url.PathEscape(revision) + "/" + name
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return err
		}
		resp, err := doWithRetry(ctx, client, retry, req)
		if err != nil {
			return err
		}
		if resp.StatusCode == http.StatusNotFound {
			_ = resp.Body.Close()
			continue
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return fmt.Errorf("Hugging Face %s returned %s", name, resp.Status)
		}
		var document map[string]any
		err = json.NewDecoder(resp.Body).Decode(&document)
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("decode Hugging Face %s: %w", name, err)
		}
		model.Repository.Files = append(model.Repository.Files, name)
		switch name {
		case "config.json":
			model.Repository.ConfigContextLength = firstPositiveInt(document, "max_position_embeddings", "seq_length", "model_max_length")
			if model.Config.ModelType == "" {
				model.Config.ModelType, _ = document["model_type"].(string)
			}
			if len(model.Config.Architectures) == 0 {
				model.Config.Architectures = stringSlice(document["architectures"])
			}
		case "tokenizer_config.json":
			model.Repository.TokenizerModelMaxLength = firstPositiveInt(document, "model_max_length")
			if template, ok := document["chat_template"]; ok {
				canonical, _ := json.Marshal(template)
				digest := sha256.Sum256(canonical)
				model.Repository.ChatTemplateSHA256 = fmt.Sprintf("%x", digest)
			}
		case "preprocessor_config.json":
			for _, key := range []string{"processor_class", "image_processor_type", "feature_extractor_type"} {
				if value, ok := document[key].(string); ok && value != "" {
					model.Repository.ProcessorClass = value
					break
				}
			}
		}
	}
	return nil
}

func firstPositiveInt(document map[string]any, keys ...string) int {
	for _, key := range keys {
		if value, ok := document[key].(float64); ok && value > 0 && value <= float64(^uint(0)>>1) {
			return int(value)
		}
	}
	return 0
}

func stringSlice(value any) []string {
	items, _ := value.([]any)
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func enrichHuggingFace(model *registry.Model, hf hfModel, fetchedAt time.Time) bool {
	license, _ := hf.CardData["license"].(string)
	before, _ := json.Marshal(model.Upstream.HuggingFace)
	var extra map[string]any
	var previous registry.HuggingFaceMetadata
	if model.Upstream.HuggingFace != nil {
		extra = model.Upstream.HuggingFace.Extra
		previous = *model.Upstream.HuggingFace
	}
	structuredFiles := hf.Repository.Files
	if len(structuredFiles) == 0 {
		structuredFiles = previous.StructuredFiles
	}
	model.Upstream.HuggingFace = &registry.HuggingFaceMetadata{
		ID:                      hf.ID,
		Revision:                firstString(hf.SHA, previous.Revision),
		PipelineTag:             hf.PipelineTag,
		ModelType:               hf.Config.ModelType,
		Architectures:           sortedUnique(hf.Config.Architectures),
		License:                 license,
		Tags:                    sortedUnique(hf.Tags),
		ConfigContextLength:     firstInt(hf.Repository.ConfigContextLength, previous.ConfigContextLength),
		TokenizerModelMaxLength: firstInt(hf.Repository.TokenizerModelMaxLength, previous.TokenizerModelMaxLength),
		ProcessorClass:          firstString(hf.Repository.ProcessorClass, previous.ProcessorClass),
		ChatTemplateSHA256:      firstString(hf.Repository.ChatTemplateSHA256, previous.ChatTemplateSHA256),
		StructuredFiles:         sortedUnique(structuredFiles),
		FetchedAt:               &fetchedAt,
		Extra:                   extra,
	}
	after, _ := json.Marshal(model.Upstream.HuggingFace)
	return !stringEqualIgnoringFetchedAt(before, after)
}

func firstString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func firstInt(value, fallback int) int {
	if value != 0 {
		return value
	}
	return fallback
}

func modelSuffix(id string) string {
	if _, suffix, ok := strings.Cut(id, "/"); ok {
		return suffix
	}
	return id
}

func normalizeIdentifier(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, value)
}

func providerMatches(provider, hfID string) bool {
	author, _, ok := strings.Cut(hfID, "/")
	return ok && normalizeIdentifier(author) == normalizeIdentifier(provider)
}

func sortedUnique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func stringEqualIgnoringFetchedAt(left, right []byte) bool {
	var a, b map[string]any
	if json.Unmarshal(left, &a) != nil || json.Unmarshal(right, &b) != nil {
		return string(left) == string(right)
	}
	delete(a, "fetched_at")
	delete(b, "fetched_at")
	return reflect.DeepEqual(a, b)
}

func doWithRetry(ctx context.Context, client *http.Client, policy retryPolicy, template *http.Request) (*http.Response, error) {
	attempts := policy.Attempts
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 0; attempt < attempts; attempt++ {
		req := template.Clone(ctx)
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
			return resp, nil
		}
		if attempt == attempts-1 {
			return resp, err
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		wait := policy.Backoff * time.Duration(1<<attempt)
		if resp != nil {
			if seconds, parseErr := strconv.Atoi(resp.Header.Get("Retry-After")); parseErr == nil && seconds >= 0 {
				wait = time.Duration(seconds) * time.Second
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, errors.New("retry loop exhausted")
}

func loadCheckpoint(path string) (map[string]bool, error) {
	completed := make(map[string]bool)
	if path == "" {
		return completed, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return completed, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &completed); err != nil {
		return nil, fmt.Errorf("load checkpoint: %w", err)
	}
	return completed, nil
}

func filterCompleted(models []registry.Model, completed map[string]bool) []registry.Model {
	result := make([]registry.Model, 0, len(models))
	for _, model := range models {
		if !completed[model.ID] {
			result = append(result, model)
		}
	}
	return result
}

func loadAllowlist(path string) (map[string]bool, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	result := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if line != "" {
			result[strings.ToLower(line)] = true
		}
	}
	return result, nil
}

func filterAllowlist(models []registry.Model, allowlist map[string]bool) []registry.Model {
	if allowlist == nil {
		return models
	}
	result := make([]registry.Model, 0, len(models))
	for _, model := range models {
		if allowlist[strings.ToLower(model.ID)] {
			result = append(result, model)
		}
	}
	return result
}

func saveCheckpoint(path string, completed map[string]bool) error {
	if path == "" {
		return nil
	}
	return writeJSONAtomic(path, completed)
}

func writeFailureReport(path string, failures map[string]string) error {
	if path == "" {
		return nil
	}
	return writeJSONAtomic(path, map[string]any{"failure_count": len(failures), "failures": failures})
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kingfs/go-llm-specs/internal/registry"
)

const codexSchemaRevision = "openai/codex@fe01054a28fa4bd04716d9ceadb410f2443a50ce"

type config struct {
	ModelsDir      string
	Output         string
	ManifestOutput string
	BundledCatalog string
	ValidateOnly   bool
}

type modelsResponse struct {
	Models []json.RawMessage `json:"models"`
}

type modelInfo struct {
	Slug                              string                  `json:"slug"`
	DisplayName                       string                  `json:"display_name"`
	Description                       *string                 `json:"description"`
	DefaultReasoningLevel             *string                 `json:"default_reasoning_level,omitempty"`
	SupportedReasoningLevels          []reasoningEffortPreset `json:"supported_reasoning_levels"`
	ShellType                         string                  `json:"shell_type"`
	Visibility                        string                  `json:"visibility"`
	SupportedInAPI                    bool                    `json:"supported_in_api"`
	Priority                          int                     `json:"priority"`
	AdditionalSpeedTiers              []string                `json:"additional_speed_tiers"`
	ServiceTiers                      []any                   `json:"service_tiers"`
	AvailabilityNux                   any                     `json:"availability_nux"`
	Upgrade                           any                     `json:"upgrade"`
	BaseInstructions                  string                  `json:"base_instructions"`
	ModelMessages                     any                     `json:"model_messages"`
	IncludeSkillsUsageInstructions    bool                    `json:"include_skills_usage_instructions"`
	SupportsReasoningSummaryParameter bool                    `json:"supports_reasoning_summary_parameter"`
	DefaultReasoningSummary           string                  `json:"default_reasoning_summary"`
	SupportVerbosity                  bool                    `json:"support_verbosity"`
	DefaultVerbosity                  *string                 `json:"default_verbosity"`
	ApplyPatchToolType                *string                 `json:"apply_patch_tool_type"`
	WebSearchToolType                 string                  `json:"web_search_tool_type"`
	TruncationPolicy                  truncationPolicy        `json:"truncation_policy"`
	SupportsParallelToolCalls         bool                    `json:"supports_parallel_tool_calls"`
	SupportsImageDetailOriginal       bool                    `json:"supports_image_detail_original"`
	ContextWindow                     int                     `json:"context_window"`
	MaxContextWindow                  int                     `json:"max_context_window"`
	AutoCompactTokenLimit             *int                    `json:"auto_compact_token_limit"`
	CompHash                          *string                 `json:"comp_hash"`
	EffectiveContextWindowPercent     int                     `json:"effective_context_window_percent"`
	ExperimentalSupportedTools        []string                `json:"experimental_supported_tools"`
	InputModalities                   []string                `json:"input_modalities"`
	SupportsSearchTool                bool                    `json:"supports_search_tool"`
	UseResponsesLite                  bool                    `json:"use_responses_lite"`
	AutoReviewModelOverride           *string                 `json:"auto_review_model_override"`
}

type reasoningEffortPreset struct {
	Effort      string `json:"effort"`
	Description string `json:"description"`
}

type truncationPolicy struct {
	Mode  string `json:"mode"`
	Limit int    `json:"limit"`
}

type manifest struct {
	SchemaRevision string   `json:"schema_revision"`
	GeneratedAt    string   `json:"generated_at"`
	ModelCount     int      `json:"model_count"`
	Slugs          []string `json:"slugs"`
}

func main() {
	cfg := parseFlags()
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.ModelsDir, "models-dir", "models", "directory containing model YAML files")
	flag.StringVar(&cfg.Output, "output", "dist/codex/models.json", "output catalog path")
	flag.StringVar(&cfg.ManifestOutput, "manifest", "", "manifest output path (default: <output>.manifest.json)")
	flag.StringVar(&cfg.BundledCatalog, "bundled-catalog", "", "optional catalog from `codex debug models --bundled`")
	flag.BoolVar(&cfg.ValidateOnly, "validate", false, "validate inputs without writing output")
	flag.Parse()
	return cfg
}

func run(cfg config) error {
	models, err := registry.Scan(cfg.ModelsDir)
	if err != nil {
		return err
	}
	generated, err := generate(models)
	if err != nil {
		return err
	}
	all := generated
	if cfg.BundledCatalog != "" {
		bundled, err := loadCatalog(cfg.BundledCatalog)
		if err != nil {
			return fmt.Errorf("load bundled catalog: %w", err)
		}
		all, err = mergeCatalogs(bundled, generated)
		if err != nil {
			return err
		}
	}
	if len(all) == 0 {
		return errors.New("no Codex-enabled models found")
	}
	if cfg.ValidateOnly {
		fmt.Printf("validated %d Codex model entries\n", len(all))
		return nil
	}
	if err := writeCatalog(cfg.Output, all); err != nil {
		return err
	}
	manifestPath := cfg.ManifestOutput
	if manifestPath == "" {
		manifestPath = cfg.Output + ".manifest.json"
	}
	return writeManifest(manifestPath, all)
}

func generate(models []registry.Model) ([]json.RawMessage, error) {
	entries := make([]modelInfo, 0)
	seen := make(map[string]string)
	for _, model := range models {
		if model.Codex == nil || !model.Codex.Enabled {
			continue
		}
		if err := validateModel(model); err != nil {
			return nil, fmt.Errorf("%s: %w", model.ID, err)
		}
		for _, slug := range model.Codex.Slugs {
			key := strings.ToLower(slug)
			if previous, ok := seen[key]; ok {
				return nil, fmt.Errorf("duplicate Codex slug %q in %s and %s", slug, previous, model.ID)
			}
			seen[key] = model.ID
			entries = append(entries, buildModelInfo(model, slug))
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Slug < entries[j].Slug })
	result := make([]json.RawMessage, 0, len(entries))
	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			return nil, err
		}
		result = append(result, data)
	}
	return result, nil
}

func validateModel(model registry.Model) error {
	var problems []string
	if model.SchemaVersion < registry.CurrentSchemaVersion {
		problems = append(problems, "Codex export requires schema_version 2")
	}
	if len(model.Codex.Slugs) == 0 {
		problems = append(problems, "codex.slugs must contain at least one serving model ID")
	}
	if model.ContextLen <= 0 {
		problems = append(problems, "context_length must be positive")
	}
	if !hasFeature(model.Features, "CapChat") {
		problems = append(problems, "CapChat is required")
	}
	if !hasFeature(model.Features, "ModalityTextIn") || !hasFeature(model.Features, "ModalityTextOut") {
		problems = append(problems, "text input and output modalities are required")
	}
	if model.Codex.TruncationPolicy.Limit < 0 {
		problems = append(problems, "truncation policy limit cannot be negative")
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func buildModelInfo(model registry.Model, slug string) modelInfo {
	codex := model.Codex
	visibility := defaultString(codex.Visibility, "list")
	shellType := defaultString(codex.ShellType, "unified_exec")
	webSearchType := defaultString(codex.WebSearchToolType, "text")
	effectivePercent := codex.EffectiveContextWindowPercent
	if effectivePercent == 0 {
		effectivePercent = 90
	}
	policy := truncationPolicy{Mode: "tokens", Limit: 10_000}
	if codex.TruncationPolicy.Mode != "" {
		policy.Mode = codex.TruncationPolicy.Mode
	}
	if codex.TruncationPolicy.Limit > 0 {
		policy.Limit = codex.TruncationPolicy.Limit
	}
	supportedInAPI := true
	if codex.SupportedInAPI != nil {
		supportedInAPI = *codex.SupportedInAPI
	}
	priority := codex.Priority
	if priority == 0 {
		priority = 100
	}
	reasoningLevels, defaultReasoning := buildReasoning(model.Reasoning)
	modalities := codex.InputModalities
	if len(modalities) == 0 {
		modalities = codexModalities(model.Features)
	}
	description := model.Description
	var applyPatch *string
	if codex.ApplyPatchToolType != "" {
		value := codex.ApplyPatchToolType
		applyPatch = &value
	}
	var defaultVerbosity *string
	if codex.DefaultVerbosity != "" {
		value := codex.DefaultVerbosity
		defaultVerbosity = &value
	}
	return modelInfo{
		Slug:                              slug,
		DisplayName:                       model.Name,
		Description:                       &description,
		DefaultReasoningLevel:             defaultReasoning,
		SupportedReasoningLevels:          reasoningLevels,
		ShellType:                         shellType,
		Visibility:                        visibility,
		SupportedInAPI:                    supportedInAPI,
		Priority:                          priority,
		AdditionalSpeedTiers:              []string{},
		ServiceTiers:                      []any{},
		AvailabilityNux:                   nil,
		Upgrade:                           nil,
		BaseInstructions:                  codex.BaseInstructions,
		ModelMessages:                     nil,
		IncludeSkillsUsageInstructions:    false,
		SupportsReasoningSummaryParameter: codex.SupportsReasoningSummaryParameter,
		DefaultReasoningSummary:           "none",
		SupportVerbosity:                  codex.SupportVerbosity,
		DefaultVerbosity:                  defaultVerbosity,
		ApplyPatchToolType:                applyPatch,
		WebSearchToolType:                 webSearchType,
		TruncationPolicy:                  policy,
		SupportsParallelToolCalls:         codex.SupportsParallelToolCalls,
		SupportsImageDetailOriginal:       false,
		ContextWindow:                     model.ContextLen,
		MaxContextWindow:                  model.ContextLen,
		AutoCompactTokenLimit:             nil,
		CompHash:                          nil,
		EffectiveContextWindowPercent:     effectivePercent,
		ExperimentalSupportedTools:        []string{},
		InputModalities:                   modalities,
		SupportsSearchTool:                false,
		UseResponsesLite:                  false,
		AutoReviewModelOverride:           nil,
	}
}

func buildReasoning(reasoning *registry.ReasoningMetadata) ([]reasoningEffortPreset, *string) {
	if reasoning == nil || !reasoning.Supported {
		return []reasoningEffortPreset{}, nil
	}
	levels := make([]reasoningEffortPreset, 0, len(reasoning.SupportedEfforts))
	for _, effort := range reasoning.SupportedEfforts {
		levels = append(levels, reasoningEffortPreset{Effort: effort, Description: effort + " reasoning effort"})
	}
	var defaultLevel *string
	if reasoning.DefaultEffort != "" {
		value := reasoning.DefaultEffort
		defaultLevel = &value
	}
	return levels, defaultLevel
}

func codexModalities(features []string) []string {
	modalities := []string{"text"}
	if hasFeature(features, "ModalityImageIn") {
		modalities = append(modalities, "image")
	}
	if hasFeature(features, "ModalityAudioIn") {
		modalities = append(modalities, "audio")
	}
	return modalities
}

func hasFeature(features []string, target string) bool {
	for _, feature := range features {
		if strings.EqualFold(feature, target) {
			return true
		}
	}
	return false
}

func loadCatalog(path string) ([]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var response modelsResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	return response.Models, nil
}

func mergeCatalogs(base, additions []json.RawMessage) ([]json.RawMessage, error) {
	bySlug := make(map[string]json.RawMessage, len(base)+len(additions))
	for _, group := range [][]json.RawMessage{base, additions} {
		for _, raw := range group {
			var identity struct {
				Slug string `json:"slug"`
			}
			if err := json.Unmarshal(raw, &identity); err != nil || identity.Slug == "" {
				return nil, errors.New("catalog entry is missing a valid slug")
			}
			key := strings.ToLower(identity.Slug)
			if _, exists := bySlug[key]; exists {
				return nil, fmt.Errorf("duplicate catalog slug %q while merging", identity.Slug)
			}
			bySlug[key] = raw
		}
	}
	keys := make([]string, 0, len(bySlug))
	for key := range bySlug {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]json.RawMessage, 0, len(keys))
	for _, key := range keys {
		result = append(result, bySlug[key])
	}
	return result, nil
}

func writeCatalog(path string, models []json.RawMessage) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(modelsResponse{Models: models}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeManifest(path string, models []json.RawMessage) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	slugs := make([]string, 0, len(models))
	for _, raw := range models {
		var identity struct {
			Slug string `json:"slug"`
		}
		_ = json.Unmarshal(raw, &identity)
		slugs = append(slugs, identity.Slug)
	}
	sort.Strings(slugs)
	data, err := json.MarshalIndent(manifest{
		SchemaRevision: codexSchemaRevision,
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		ModelCount:     len(models),
		Slugs:          slugs,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

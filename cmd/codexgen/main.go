package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/kingfs/go-llm-specs/internal/provider"
	"github.com/kingfs/go-llm-specs/internal/registry"
	"gopkg.in/yaml.v3"
)

const codexSchemaRevision = "openai/codex@fe01054a28fa4bd04716d9ceadb410f2443a50ce"

type config struct {
	ModelsDir      string
	Output         string
	ManifestOutput string
	BundledCatalog string
	ValidateOnly   bool
	GeneratedAt    string
	PolicyPath     string
	ProvidersDir   string
}

type defaultPolicy struct {
	SchemaVersion int            `yaml:"schema_version"`
	Families      []policyFamily `yaml:"families"`
}

type policyFamily struct {
	Name               string   `yaml:"name"`
	IDPattern          string   `yaml:"id_pattern"`
	RequireHuggingFace bool     `yaml:"require_hugging_face"`
	SlugStrategies     []string `yaml:"slug_strategies"`
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
	CatalogKind    string   `json:"catalog_kind"`
	GeneratedAt    string   `json:"generated_at,omitempty"`
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
	flag.StringVar(&cfg.Output, "output", "dist/codex/third-party-models.json", "output catalog path")
	flag.StringVar(&cfg.ManifestOutput, "manifest", "", "manifest output path (default: <output>.manifest.json)")
	flag.StringVar(&cfg.BundledCatalog, "bundled-catalog", "", "optional catalog from `codex debug models --bundled`")
	flag.BoolVar(&cfg.ValidateOnly, "validate", false, "validate inputs without writing output")
	flag.StringVar(&cfg.GeneratedAt, "generated-at", "", "optional RFC3339 timestamp for the manifest")
	flag.StringVar(&cfg.PolicyPath, "policy", "data/codex/default-open-models.yaml", "default open-model inclusion policy (empty disables it)")
	flag.StringVar(&cfg.ProvidersDir, "providers-dir", "providers", "authoritative publisher catalog directory")
	flag.Parse()
	return cfg
}

func run(cfg config) error {
	models, err := registry.Scan(cfg.ModelsDir)
	if err != nil {
		return err
	}
	if cfg.PolicyPath != "" {
		policy, err := loadDefaultPolicy(cfg.PolicyPath)
		if err != nil {
			return fmt.Errorf("load default policy: %w", err)
		}
		models, err = applyDefaultPolicy(models, policy)
		if err != nil {
			return fmt.Errorf("apply default policy: %w", err)
		}
	}
	publishers, err := provider.Scan(cfg.ProvidersDir)
	if err != nil {
		return fmt.Errorf("load publisher catalog: %w", err)
	}
	models, err = enforceCodexAuthority(models, publishers)
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
	return writeManifest(manifestPath, all, cfg.GeneratedAt)
}

// enforceCodexAuthority separates historical collection from executable
// Codex metadata. A model may remain in the museum without an authoritative
// publisher identity, but it cannot be exported as a runnable Codex model.
func enforceCodexAuthority(models []registry.Model, publishers []provider.Provider) ([]registry.Model, error) {
	byID := make(map[string]provider.Provider, len(publishers))
	for _, publisher := range publishers {
		byID[strings.ToLower(publisher.ID)] = publisher
		for _, alias := range publisher.Aliases {
			byID[strings.ToLower(alias)] = publisher
		}
	}
	for i := range models {
		model := &models[i]
		if model.Codex == nil || !model.Codex.Enabled {
			continue
		}
		publisher, ok := byID[strings.ToLower(model.Developer)]
		if !ok || !officialHuggingFaceIdentity(*model, publisher) {
			// Explicit Codex blocks are human assertions and should fail loudly;
			// policy-derived entries are omitted until their identity is verified.
			if model.FilePath != "" && hasExplicitCodexBlock(model.FilePath) {
				return nil, fmt.Errorf("%s: Codex export requires a cataloged publisher and pinned official Hugging Face model card", model.ID)
			}
			model.Codex = nil
		}
	}
	return models, nil
}

func officialHuggingFaceIdentity(model registry.Model, publisher provider.Provider) bool {
	if model.Upstream.HuggingFace == nil || model.Upstream.HuggingFace.ID == "" || model.Upstream.HuggingFace.Revision == "" {
		return false
	}
	parts := strings.SplitN(model.Upstream.HuggingFace.ID, "/", 2)
	if len(parts) != 2 || !strings.EqualFold(strings.TrimSuffix(model.Links.ModelCard, "/"), "https://huggingface.co/"+model.Upstream.HuggingFace.ID) {
		return false
	}
	for _, organization := range publisher.Organizations.HuggingFace {
		if strings.EqualFold(organization, parts[0]) {
			return true
		}
	}
	return false
}

func hasExplicitCodexBlock(path string) bool {
	data, err := os.ReadFile(path)
	return err == nil && regexp.MustCompile(`(?m)^codex:`).Match(data)
}

func loadDefaultPolicy(path string) (defaultPolicy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultPolicy{}, err
	}
	var policy defaultPolicy
	if err := yaml.Unmarshal(data, &policy); err != nil {
		return defaultPolicy{}, err
	}
	if policy.SchemaVersion != 1 || len(policy.Families) == 0 {
		return defaultPolicy{}, errors.New("policy requires schema_version 1 and at least one family")
	}
	return policy, nil
}

func applyDefaultPolicy(models []registry.Model, policy defaultPolicy) ([]registry.Model, error) {
	patterns := make([]*regexp.Regexp, len(policy.Families))
	for i, family := range policy.Families {
		if family.Name == "" || len(family.SlugStrategies) == 0 {
			return nil, fmt.Errorf("policy family %d requires name and slug_strategies", i)
		}
		pattern, err := regexp.Compile(family.IDPattern)
		if err != nil {
			return nil, fmt.Errorf("family %q: %w", family.Name, err)
		}
		patterns[i] = pattern
	}
	result := append([]registry.Model(nil), models...)
	for i := range result {
		model := &result[i]
		// Any explicit Codex block, including enabled: false, is authoritative.
		if model.Codex != nil {
			continue
		}
		for familyIndex, family := range policy.Families {
			if !patterns[familyIndex].MatchString(strings.ToLower(model.ID)) {
				continue
			}
			if family.RequireHuggingFace && (model.Upstream.HuggingFace == nil || model.Upstream.HuggingFace.ID == "") {
				continue
			}
			if reason := policyIneligibleReason(*model); reason != "" {
				continue
			}
			slugs, err := policySlugs(*model, family.SlugStrategies)
			if err != nil {
				return nil, fmt.Errorf("family %q model %s: %w", family.Name, model.ID, err)
			}
			model.Codex = &registry.CodexMetadata{
				Enabled:                   true,
				Slugs:                     slugs,
				ShellType:                 "unified_exec",
				ApplyPatchToolType:        "freeform",
				SupportsParallelToolCalls: false,
				InputModalities:           codexModalities(model.Features),
			}
			break
		}
	}
	return result, nil
}

func policyIneligibleReason(model registry.Model) string {
	if !model.IsV2() || model.ContextLen <= 0 {
		return "schema v2 and positive context are required"
	}
	for _, feature := range []string{"CapChat", "CapFunctionCall", "ModalityTextIn", "ModalityTextOut"} {
		if !hasFeature(model.Features, feature) {
			return "missing " + feature
		}
	}
	return ""
}

func policySlugs(model registry.Model, strategies []string) ([]string, error) {
	seen := map[string]bool{}
	var slugs []string
	for _, strategy := range strategies {
		var slug string
		switch strategy {
		case "registry_id":
			slug = model.ID
		case "model_suffix":
			_, slug, _ = strings.Cut(model.ID, "/")
			slug = strings.ToLower(slug)
		case "huggingface_id":
			if model.Upstream.HuggingFace != nil {
				slug = model.Upstream.HuggingFace.ID
			}
		default:
			return nil, fmt.Errorf("unknown slug strategy %q", strategy)
		}
		key := strings.ToLower(slug)
		if slug != "" && !seen[key] {
			seen[key] = true
			slugs = append(slugs, slug)
		}
	}
	if len(slugs) == 0 {
		return nil, errors.New("slug strategies produced no serving IDs")
	}
	return slugs, nil
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

func writeManifest(path string, models []json.RawMessage, generatedAt string) error {
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
		CatalogKind:    "standalone-third-party",
		GeneratedAt:    generatedAt,
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

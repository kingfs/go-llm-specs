package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	registrymodel "github.com/kingfs/go-llm-specs/internal/registry"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
)

const defaultOpenRouterModelsURL = "https://openrouter.ai/api/v1/models"

// OpenRouter model structures
type OpenRouterModel struct {
	ID                  string                 `json:"id"`
	Name                string                 `json:"name"`
	Description         string                 `json:"description"`
	ContextLength       int                    `json:"context_length"`
	TopProvider         OpenRouterTopProvider  `json:"top_provider"`
	Architecture        OpenRouterArchitecture `json:"architecture"`
	Pricing             OpenRouterPricing      `json:"pricing"`
	SupportedParameters []string               `json:"supported_parameters"`
}

type OpenRouterTopProvider struct {
	ContextLength       int `json:"context_length"`
	MaxCompletionTokens int `json:"max_completion_tokens"`
}

type OpenRouterArchitecture struct {
	Modality         string   `json:"modality"`
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`
}

type OpenRouterPricing struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
}

type OpenRouterResponse struct {
	Data []OpenRouterModel `json:"data"`
}

// Registry structures
type RegistryData struct {
	Models map[string]ModelRegistry `yaml:"models"`
}

type ModelRegistry = registrymodel.Model

type ProcessedModel struct {
	ID            string
	Name          string
	Provider      string
	Description   string
	DescriptionCN string
	Family        string
	Series        string
	Summary       string
	Tags          []string
	ContextLen    int
	MaxOutput     int
	Features      string
	Aliases       []string
}

type generatorConfig struct {
	Source           string
	APIURL           string
	ModelsDir        string
	CachePath        string
	OutputGo         string
	SyncRegistry     bool
	FetchOnly        bool
	UseCacheFallback bool
	HTTPTimeout      time.Duration
}

func main() {
	cfg := parseFlags()
	if err := run(cfg); err != nil {
		log.Fatal(err)
	}
}

func parseFlags() generatorConfig {
	defaultURL := os.Getenv("OPENROUTER_MODELS_URL")
	if defaultURL == "" {
		defaultURL = defaultOpenRouterModelsURL
	}

	cfg := generatorConfig{}
	flag.StringVar(&cfg.Source, "source", "openrouter", "upstream source used to fetch models")
	flag.StringVar(&cfg.APIURL, "api-url", defaultURL, "upstream models API URL")
	flag.StringVar(&cfg.ModelsDir, "models-dir", "models", "directory containing model yaml files")
	flag.StringVar(&cfg.CachePath, "cache-path", "data/models.json", "path for upstream raw models cache")
	flag.StringVar(&cfg.OutputGo, "output-go", "models_gen.go", "generated Go registry output path")
	flag.BoolVar(&cfg.SyncRegistry, "sync-registry", true, "write fetched upstream data back into models directory before codegen")
	flag.BoolVar(&cfg.FetchOnly, "fetch-only", false, "only fetch/cache upstream metadata, skip registry sync and code generation")
	flag.BoolVar(&cfg.UseCacheFallback, "use-cache-fallback", true, "fall back to the cached raw upstream payload when HTTP fetch fails")
	flag.DurationVar(&cfg.HTTPTimeout, "timeout", 30*time.Second, "HTTP timeout used when fetching upstream models")
	flag.Parse()
	return cfg
}

func run(cfg generatorConfig) error {
	log.Printf("Starting llm-specs generator (source=%s)", cfg.Source)

	apiModels, err := fetchSourceModels(cfg)
	if err != nil {
		return fmt.Errorf("fetch upstream models: %w", err)
	}
	log.Printf("Fetched %d models from %s", len(apiModels), cfg.Source)

	if cfg.FetchOnly {
		log.Println("Fetch-only mode enabled, skipping registry sync and code generation")
		return nil
	}

	localModels, err := loadRegistry(cfg.ModelsDir)
	if err != nil {
		return fmt.Errorf("load local registry from %s: %w", cfg.ModelsDir, err)
	}
	log.Printf("Loaded %d local model files", len(localModels))

	if cfg.SyncRegistry {
		if err := syncToDisk(apiModels, localModels, cfg.ModelsDir); err != nil {
			return fmt.Errorf("sync models to disk: %w", err)
		}
	}

	finalModels, err := loadRegistry(cfg.ModelsDir)
	if err != nil {
		return fmt.Errorf("reload local registry from %s: %w", cfg.ModelsDir, err)
	}
	log.Printf("Final registry contains %d models", len(finalModels))

	processedModels := buildProcessedModels(finalModels)
	aliasMap := buildAliasMap(processedModels)

	if err := generateCode(cfg.OutputGo, processedModels, aliasMap); err != nil {
		return fmt.Errorf("generate code: %w", err)
	}

	log.Printf("Generator finished successfully, wrote %s", cfg.OutputGo)
	return nil
}

func buildProcessedModels(finalModels map[string]ModelRegistry) []*ProcessedModel {
	processedModels := make([]*ProcessedModel, 0, len(finalModels))

	for id, m := range finalModels {
		p := &ProcessedModel{
			ID:            id,
			Name:          m.Name,
			Provider:      m.Provider,
			Description:   m.Description,
			DescriptionCN: m.DescriptionCN,
			ContextLen:    m.ContextLen,
			MaxOutput:     m.MaxOutput,
			Aliases:       normalizeStringList(m.Aliases),
		}
		meta := deriveStructuredMetadata(m)
		p.Family = meta.Family
		p.Series = meta.Series
		p.Summary = meta.Summary
		p.Tags = meta.Tags

		if len(m.Features) > 0 {
			p.Features = strings.Join(normalizeStringList(m.Features), " | ")
		} else {
			p.Features = "0"
		}

		if strings.Contains(p.Features, "ImageIn") || strings.Contains(p.Features, "VideoIn") {
			if !strings.Contains(p.Features, "CapMultimodal") {
				if p.Features == "0" {
					p.Features = "CapMultimodal"
				} else {
					p.Features += " | CapMultimodal"
				}
			}
		}

		processedModels = append(processedModels, p)
	}

	autoAddUniqueSuffixAliases(processedModels)

	sort.Slice(processedModels, func(i, j int) bool {
		return processedModels[i].ID < processedModels[j].ID
	})

	for _, model := range processedModels {
		model.Aliases = normalizeStringList(model.Aliases)
	}

	return processedModels
}

func autoAddUniqueSuffixAliases(processedModels []*ProcessedModel) {
	suffixCounts := make(map[string]int, len(processedModels))
	for _, p := range processedModels {
		parts := strings.Split(p.ID, "/")
		if len(parts) > 1 {
			suffixCounts[parts[len(parts)-1]]++
		}
	}

	for _, p := range processedModels {
		parts := strings.Split(p.ID, "/")
		if len(parts) <= 1 {
			continue
		}

		suffix := parts[len(parts)-1]
		if suffixCounts[suffix] != 1 {
			continue
		}

		exists := false
		for _, alias := range p.Aliases {
			if strings.EqualFold(alias, suffix) {
				exists = true
				break
			}
		}
		if !exists {
			p.Aliases = append(p.Aliases, suffix)
		}
	}
}

func buildAliasMap(processedModels []*ProcessedModel) map[string]string {
	aliasMap := make(map[string]string)
	for _, p := range processedModels {
		for _, alias := range p.Aliases {
			lowerAlias := strings.ToLower(alias)
			if existingID, ok := aliasMap[lowerAlias]; ok && existingID != p.ID {
				log.Printf("Skipping alias collision for %q: %s already maps to %s", alias, p.ID, existingID)
				continue
			}
			aliasMap[lowerAlias] = p.ID
		}
	}
	return aliasMap
}

func syncToDisk(apiModels []OpenRouterModel, localModels map[string]ModelRegistry, modelsDir string) error {
	for _, upstream := range apiModels {
		local := localModels[upstream.ID]
		if local.ID == "" {
			now := time.Now().UTC()
			local.SchemaVersion = registrymodel.CurrentSchemaVersion
			local.DiscoveredAt = &now
		}
		merged := mergeModelRegistry(upstream, local)
		if err := saveModelToDisk(merged, modelsDir); err != nil {
			return fmt.Errorf("save model %s: %w", upstream.ID, err)
		}
	}
	return nil
}

func mergeModelRegistry(upstream OpenRouterModel, local ModelRegistry) ModelRegistry {
	// Start from the complete local object so v2 and forward-compatible fields
	// survive synchronization. Upstream only fills fields that are not locally set.
	merged := local
	if merged.ID == "" {
		merged.ID = upstream.ID
	}
	if merged.Name == "" {
		merged.Name = upstream.Name
	}
	if merged.Provider == "" {
		merged.Provider = normalizeProvider(strings.Split(upstream.ID, "/")[0])
	}
	if merged.Description == "" {
		merged.Description = upstream.Description
	}
	if merged.ContextLen <= 0 {
		merged.ContextLen = upstream.ContextLength
	}
	if merged.MaxOutput <= 0 {
		merged.MaxOutput = upstream.TopProvider.MaxCompletionTokens
	}
	if len(merged.Features) == 0 {
		merged.Features = stringsToFeatures(calculateFeatures(upstream))
	}

	merged.Features = normalizeStringList(merged.Features)
	merged.Aliases = normalizeStringList(merged.Aliases)

	if len(merged.Features) == 0 {
		merged.Features = []string{"CapChat"}
	} else if !containsFold(merged.Features, "CapChat") {
		merged.Features = append([]string{"CapChat"}, merged.Features...)
		merged.Features = normalizeStringList(merged.Features)
	}

	return merged
}

func stringsToFeatures(featureExpr string) []string {
	if featureExpr == "" || featureExpr == "0" {
		return nil
	}

	parts := strings.Split(featureExpr, "|")
	features := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "0" {
			continue
		}
		features = append(features, part)
	}
	return normalizeStringList(features)
}

func saveModelToDisk(m ModelRegistry, modelsDir string) error {
	parts := strings.SplitN(m.ID, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid model ID: %s", m.ID)
	}

	provider := parts[0]
	modelName := parts[1]
	safeModelName := strings.NewReplacer(":", "_", "/", "_").Replace(modelName)

	dir := filepath.Join(modelsDir, provider)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	filePath := filepath.Join(dir, safeModelName+".yaml")

	return registrymodel.Save(filePath, m)
}

func calculateFeatures(m OpenRouterModel) string {
	var features []string

	for _, mod := range m.Architecture.InputModalities {
		switch strings.ToLower(mod) {
		case "text":
			features = append(features, "ModalityTextIn")
		case "image":
			features = append(features, "ModalityImageIn")
		case "audio":
			features = append(features, "ModalityAudioIn")
		case "video":
			features = append(features, "ModalityVideoIn")
		case "file":
			features = append(features, "ModalityFileIn")
		}
	}

	for _, mod := range m.Architecture.OutputModalities {
		switch strings.ToLower(mod) {
		case "text":
			features = append(features, "ModalityTextOut")
		case "image":
			features = append(features, "ModalityImageOut")
		case "audio":
			features = append(features, "ModalityAudioOut")
		case "video":
			features = append(features, "ModalityVideoOut")
		case "file":
			features = append(features, "ModalityFileOut")
		}
	}

	hasTools := false
	for _, p := range m.SupportedParameters {
		if p == "tools" || p == "tool_choice" {
			hasTools = true
			break
		}
	}
	if !hasTools && (strings.Contains(strings.ToLower(m.Description), "function calling") || strings.Contains(strings.ToLower(m.Description), "tools")) {
		hasTools = true
	}
	if hasTools {
		features = append(features, "CapFunctionCall")
	}

	for _, p := range m.SupportedParameters {
		if p == "response_format" || p == "structured_outputs" {
			features = append(features, "CapJsonMode")
			break
		}
	}

	if strings.Contains(strings.ToLower(m.Description), "#multimodal") {
		features = append(features, "ModalityImageIn")
	}

	if len(features) == 0 {
		return "0"
	}

	return strings.Join(normalizeStringList(features), " | ")
}

func normalizeProvider(idPrefix string) string {
	lower := strings.ToLower(idPrefix)
	switch lower {
	case "alibaba", "qwen":
		return "Qwen"
	case "01-ai", "01.ai":
		return "01.AI"
	case "mistralai", "mistral":
		return "Mistral"
	case "meta-llama", "llama":
		return "Meta"
	case "google":
		return "Google"
	case "anthropic":
		return "Anthropic"
	case "openai":
		return "OpenAI"
	case "microsoft":
		return "Microsoft"
	case "perplexity":
		return "Perplexity"
	case "cohere":
		return "Cohere"
	case "nousresearch":
		return "Nous Research"
	case "deepseek":
		return "DeepSeek"
	default:
		caser := cases.Title(language.English)
		return caser.String(lower)
	}
}

const modelTemplate = `// Code generated by llm-specs-gen. DO NOT EDIT.
// Generated at: {{ .GeneratedAt }}

package llmspecs

func init() {
	staticRegistry = map[string]*modelData{
		{{- range .Models }}
		"{{ .ID }}": {
			IDVal:         "{{ .ID }}",
			NameVal:       "{{ .Name }}",
			ProviderVal:   "{{ .Provider }}",
			DescVal:       {{ printf "%q" .Description }},
			DescCNVal:     {{ printf "%q" .DescriptionCN }},
			FamilyVal:     {{ printf "%q" .Family }},
			SeriesVal:     {{ printf "%q" .Series }},
			SummaryVal:    {{ printf "%q" .Summary }},
			TagList:       []string{ {{- range $i, $tag := .Tags }}{{ if $i }}, {{ end }}"{{ $tag }}"{{ end -}} },
			ContextLenVal: {{ .ContextLen }},
			MaxOutputVal:  {{ .MaxOutput }},
			FeaturesVal:   {{ .Features }},
			AliasList:     []string{ {{- range $i, $alias := .Aliases }}{{ if $i }}, {{ end }}"{{ $alias }}"{{ end -}} },
		},
		{{- end }}
	}

	aliasIndex = map[string]string{
		{{- range $alias, $id := .AliasMap }}
		"{{ $alias }}": "{{ $id }}",
		{{- end }}
	}
}
`

func generateCode(outputPath string, models []*ProcessedModel, aliasMap map[string]string) error {
	tmpl, err := template.New("gen").Parse(modelTemplate)
	if err != nil {
		return err
	}

	data := struct {
		GeneratedAt string
		Models      []*ProcessedModel
		AliasMap    map[string]string
	}{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Models:      models,
		AliasMap:    aliasMap,
	}

	var generated bytes.Buffer
	if err := tmpl.Execute(&generated, data); err != nil {
		return err
	}
	formatted, err := format.Source(generated.Bytes())
	if err != nil {
		return fmt.Errorf("format generated registry: %w", err)
	}
	return os.WriteFile(outputPath, formatted, 0o644)
}

func fetchSourceModels(cfg generatorConfig) ([]OpenRouterModel, error) {
	switch strings.ToLower(cfg.Source) {
	case "openrouter":
		return fetchOpenRouterModels(cfg.APIURL, cfg.CachePath, cfg.UseCacheFallback, cfg.HTTPTimeout)
	default:
		return nil, fmt.Errorf("unsupported source %q", cfg.Source)
	}
}

func fetchOpenRouterModels(apiURL, cachePath string, useCacheFallback bool, timeout time.Duration) ([]OpenRouterModel, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(apiURL)
	if err != nil {
		if !useCacheFallback {
			return nil, err
		}
		return loadOpenRouterModelsFromCache(cachePath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if !useCacheFallback {
			return nil, fmt.Errorf("unexpected status: %s", resp.Status)
		}
		return loadOpenRouterModelsFromCache(cachePath, fmt.Errorf("unexpected status: %s", resp.Status))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if cachePath != "" {
		cacheDir := filepath.Dir(cachePath)
		if cacheDir != "." {
			if err := os.MkdirAll(cacheDir, 0o755); err != nil {
				log.Printf("Warning: failed to create cache directory %s: %v", cacheDir, err)
			}
		}
		if err := os.WriteFile(cachePath, body, 0o644); err != nil {
			log.Printf("Warning: failed to save raw JSON cache to %s: %v", cachePath, err)
		}
	}

	var orResp OpenRouterResponse
	if err := json.Unmarshal(body, &orResp); err != nil {
		return nil, err
	}

	return orResp.Data, nil
}

func loadOpenRouterModelsFromCache(cachePath string, fetchErr error) ([]OpenRouterModel, error) {
	if cachePath == "" {
		return nil, fetchErr
	}

	log.Printf("Fetch failed (%v), attempting cache fallback from %s", fetchErr, cachePath)
	body, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, errors.Join(fetchErr, fmt.Errorf("read cache %s: %w", cachePath, err))
	}

	var orResp OpenRouterResponse
	if err := json.Unmarshal(body, &orResp); err != nil {
		return nil, errors.Join(fetchErr, fmt.Errorf("decode cache %s: %w", cachePath, err))
	}

	return orResp.Data, nil
}

func loadRegistry(root string) (map[string]ModelRegistry, error) {
	models := make(map[string]ModelRegistry)

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || (!strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml")) {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		var data RegistryData
		decoder := yaml.NewDecoder(f)
		if err := decoder.Decode(&data); err == nil && len(data.Models) > 0 {
			for id, m := range data.Models {
				if m.ID == "" {
					m.ID = id
				}
				models[m.ID] = m
			}
			return nil
		}

		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}

		var single ModelRegistry
		if err := yaml.NewDecoder(f).Decode(&single); err == nil && (single.ID != "" || single.Name != "") {
			if single.ID != "" {
				models[single.ID] = single
			}
		}

		return nil
	})

	return models, err
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]string, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}

		lower := strings.ToLower(value)
		if _, ok := seen[lower]; !ok {
			seen[lower] = value
		}
	}

	if len(seen) == 0 {
		return nil
	}

	result := make([]string, 0, len(seen))
	for _, value := range seen {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i]) < strings.ToLower(result[j])
	})
	return result
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

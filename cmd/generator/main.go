package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
)

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
type ModelRegistry struct {
	ID            string   `yaml:"id"`
	Name          string   `yaml:"name"`
	NameCN        string   `yaml:"name_cn,omitempty"`
	Provider      string   `yaml:"provider"`
	Description   string   `yaml:"description,omitempty"`
	DescriptionCN string   `yaml:"description_cn,omitempty"`
	ContextLen    int      `yaml:"context_length"`
	MaxOutput     int      `yaml:"max_output,omitempty"`
	Features      []string `yaml:"features,omitempty"`
	Aliases       []string `yaml:"aliases,omitempty"`
}

func main() {
	log.Println("Starting llm-specs generator...")

	// 1. Fetch data from OpenRouter
	apiModels, err := fetchOpenRouterModels()
	if err != nil {
		log.Fatalf("Failed to fetch models: %v", err)
	}
	log.Printf("Fetched %d models from OpenRouter", len(apiModels))

	// 2. Load Existing Local Registry
	localModels, err := loadRegistry("models")
	if err != nil {
		log.Printf("Warning: failed to load local registry: %v (skipping sync, continuing with current files)", err)
		localModels = make(map[string]ModelRegistry)
	}
	log.Printf("Loaded %d models from local registry", len(localModels))

	// 3. Sync API data to Local Registry
	if err := syncToDisk(apiModels, localModels); err != nil {
		log.Fatalf("Failed to sync models to disk: %v", err)
	}

	// 4. Reload Local Registry (Sole Source of Truth)
	finalModels, err := loadRegistry("models")
	if err != nil {
		log.Fatalf("Failed to reload local registry: %v", err)
	}
	log.Printf("Final registry has %d models", len(finalModels))

	// 5. Process for Code Generation
	processedModels := make([]*ProcessedModel, 0)
	for id, m := range finalModels {
		p := &ProcessedModel{
			ID:            id,
			Name:          m.Name,
			Provider:      m.Provider,
			Description:   m.Description,
			DescriptionCN: m.DescriptionCN,
			ContextLen:    m.ContextLen,
			MaxOutput:     m.MaxOutput,
			Aliases:       m.Aliases,
		}
		if len(m.Features) > 0 {
			p.Features = strings.Join(m.Features, " | ")
		} else {
			p.Features = "0"
		}

		// Auto-detect Multimodal for local models too if not explicitly tagged
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

	// 6. Auto-generate aliases from unique suffixes
	suffixCounts := make(map[string]int)
	for _, p := range processedModels {
		parts := strings.Split(p.ID, "/")
		if len(parts) > 1 {
			suffix := parts[len(parts)-1]
			suffixCounts[suffix]++
		}
	}

	for _, p := range processedModels {
		parts := strings.Split(p.ID, "/")
		if len(parts) > 1 {
			suffix := parts[len(parts)-1]
			if suffixCounts[suffix] == 1 {
				exists := false
				for _, a := range p.Aliases {
					if strings.EqualFold(a, suffix) {
						exists = true
						break
					}
				}
				if !exists {
					p.Aliases = append(p.Aliases, suffix)
				}
			}
		}
	}

	// Sort for deterministic alias map and output
	sort.Slice(processedModels, func(i, j int) bool {
		return processedModels[i].ID < processedModels[j].ID
	})

	// 7. Populate alias map
	aliasMap := make(map[string]string)
	for _, p := range processedModels {
		for _, alias := range p.Aliases {
			lowerAlias := strings.ToLower(alias)
			if existingID, ok := aliasMap[lowerAlias]; ok && existingID != p.ID {
				// Log collision but keep existing (usually manual alias wins if it came first, but here it's processed order)
				// We should probably sort processedModels by ID first to be deterministic.
			} else {
				aliasMap[lowerAlias] = p.ID
			}
		}
	}

	// 8. Generate Code
	if err := generateCode(processedModels, aliasMap); err != nil {
		log.Fatalf("Failed to generate code: %v", err)
	}

	log.Println("Generator finished successfully.")
}

func syncToDisk(apiModels []OpenRouterModel, localModels map[string]ModelRegistry) error {
	for _, m := range apiModels {
		local, _ := localModels[m.ID]

		// Update fields from API
		local.ID = m.ID
		local.Name = m.Name
		local.Description = m.Description
		local.ContextLen = m.ContextLength
		local.MaxOutput = m.TopProvider.MaxCompletionTokens
		local.Provider = normalizeProvider(strings.Split(m.ID, "/")[0])

		// Derived features from API (only if local features are empty)
		if len(local.Features) == 0 {
			featStr := calculateFeatures(m)
			if featStr != "0" {
				local.Features = strings.Split(featStr, " | ")
			}
			// Default to CapChat for OR models
			hasChat := false
			for _, f := range local.Features {
				if f == "CapChat" {
					hasChat = true
					break
				}
			}
			if !hasChat {
				local.Features = append([]string{"CapChat"}, local.Features...)
			}
		}

		// Save back to disk
		if err := saveModelToDisk(local); err != nil {
			log.Printf("Error saving model %s: %v", m.ID, err)
		}
	}
	return nil
}

func saveModelToDisk(m ModelRegistry) error {
	parts := strings.SplitN(m.ID, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid model ID: %s", m.ID)
	}
	provider := parts[0]
	modelName := parts[1]
	safeModelName := strings.ReplaceAll(modelName, ":", "_")
	safeModelName = strings.ReplaceAll(safeModelName, "/", "_")

	dir := filepath.Join("models", provider)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(dir, safeModelName+".yaml")

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(m); err != nil {
		return err
	}

	return os.WriteFile(filePath, buf.Bytes(), 0644)
}

type ProcessedModel struct {
	ID            string
	Name          string
	Provider      string
	Description   string
	DescriptionCN string
	ContextLen    int
	MaxOutput     int
	PriceIn       float64
	PriceOut      float64
	Features      string // String representation for template
	Aliases       []string
}

func calculateFeatures(m OpenRouterModel) string {
	var features []string

	// Map Input Modalities
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

	// Map Output Modalities
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

	// Function Calling: Check parameters or description
	hasTools := false
	for _, p := range m.SupportedParameters {
		if p == "tools" || p == "tool_choice" {
			hasTools = true
			break
		}
	}
	// Fallback to description check if parameters missing (older models)
	if !hasTools && (strings.Contains(strings.ToLower(m.Description), "function calling") || strings.Contains(strings.ToLower(m.Description), "tools")) {
		hasTools = true
	}
	if hasTools {
		features = append(features, "CapFunctionCall")
	}

	// JSON Mode / Structured Outputs
	for _, p := range m.SupportedParameters {
		if p == "response_format" || p == "structured_outputs" {
			features = append(features, "CapJsonMode")
			break
		}
	}

	// System Prompt support is very common, usually assumed, but if we want to be strict check parameters?
	// For now, let's leave it as is or add detection if "system" roles are supported, but the API doesn't expose roles directly in this JSON.
	// Many models support system prompts implicitly. We won't inadvertently set it to avoid false positives unless we have a clear signal.

	if strings.Contains(strings.ToLower(m.Description), "#multimodal") {
		// Only add if not already present, though deduplication handles it
		features = append(features, "ModalityImageIn")
	}

	if len(features) == 0 {
		return "0"
	}
	// Deduplicate
	featureMap := make(map[string]bool)
	var uniqueFeatures []string
	for _, f := range features {
		if !featureMap[f] {
			featureMap[f] = true
			uniqueFeatures = append(uniqueFeatures, f)
		}
	}
	// Sort for deterministic output
	sort.Strings(uniqueFeatures)

	return strings.Join(uniqueFeatures, " | ")
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
			ContextLenVal: {{ .ContextLen }},
			MaxOutputVal:  {{ .MaxOutput }},
			FeaturesVal:   {{ .Features }},
			AliasList:     []string{ {{ range $i, $alias := .Aliases }}{{ if $i }}, {{ end }}"{{ $alias }}"{{ end }} },
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

func generateCode(models []*ProcessedModel, aliasMap map[string]string) error {
	tmpl, err := template.New("gen").Parse(modelTemplate)
	if err != nil {
		return err
	}

	f, err := os.Create("models_gen.go")
	if err != nil {
		return err
	}
	defer f.Close()

	data := struct {
		GeneratedAt string
		Models      []*ProcessedModel
		AliasMap    map[string]string
	}{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Models:      models,
		AliasMap:    aliasMap,
	}

	return tmpl.Execute(f, data)
}

func fetchOpenRouterModels() ([]OpenRouterModel, error) {
	resp, err := http.Get("https://openrouter.ai/api/v1/models")
	if err != nil {
		// Fallback to local cache if available
		log.Printf("Network error: %v. Attempting to use local cache data/models.json", err)
		body, err := os.ReadFile("data/models.json")
		if err != nil {
			return nil, fmt.Errorf("failed to fetch from network and failed to read local cache: %v", err)
		}
		var orResp OpenRouterResponse
		if err := json.Unmarshal(body, &orResp); err != nil {
			return nil, err
		}
		return orResp.Data, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Save raw JSON as asset
	os.MkdirAll("data", 0755)
	if err := os.WriteFile("data/models.json", body, 0644); err != nil {
		log.Printf("Warning: failed to save raw JSON: %v", err)
	}

	var orResp OpenRouterResponse
	if err := json.Unmarshal(body, &orResp); err != nil {
		return nil, err
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

		// Try to decode as RegistryData (map of models)
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

		// Seek back and try as a single ModelRegistry
		f.Seek(0, 0)
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

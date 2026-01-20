package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
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

// Override structures
type OverrideData struct {
	Models map[string]ModelOverride `yaml:"models"`
}

type ModelOverride struct {
	NameCN        string   `yaml:"name_cn"`
	Description   string   `yaml:"description"`
	DescriptionCN string   `yaml:"description_cn"`
	Aliases       []string `yaml:"aliases"`
	Provider      string   `yaml:"provider"`
}

func main() {
	log.Println("Starting llm-specs generator...")

	// 1. Fetch data from OpenRouter
	models, err := fetchOpenRouterModels()
	if err != nil {
		log.Fatalf("Failed to fetch models: %v", err)
	}
	log.Printf("Fetched %d models from OpenRouter", len(models))

	// 2. Load overrides
	overrides, err := loadOverrides("data/overrides.yaml")
	if err != nil {
		log.Printf("Warning: failed to load overrides: %v (skipping)", err)
		overrides = &OverrideData{Models: make(map[string]ModelOverride)}
	}

	// 3. Process and Normalize
	processedModels := make([]*ProcessedModel, 0, len(models))
	aliasMap := make(map[string]string)

	for _, m := range models {
		ov, hasOverride := overrides.Models[m.ID]

		p := &ProcessedModel{
			ID:          m.ID,
			Name:        m.Name,
			Provider:    normalizeProvider(strings.Split(m.ID, "/")[0]), // Default normalized provider from ID
			Description: m.Description,
			ContextLen:  m.ContextLength,
			MaxOutput:   m.TopProvider.MaxCompletionTokens,
		}

		// Apply overrides
		if hasOverride {
			if ov.Provider != "" {
				p.Provider = ov.Provider
			}
			if ov.Description != "" {
				p.Description = ov.Description
			}
			p.DescriptionCN = ov.DescriptionCN
			p.Aliases = ov.Aliases
		}

		// Normalize Provider name (Title case again just in case)
		p.Provider = strings.Title(p.Provider)

		// Parse Pricing
		var pIn, pOut float64
		fmt.Sscanf(m.Pricing.Prompt, "%f", &pIn)
		fmt.Sscanf(m.Pricing.Completion, "%f", &pOut)
		p.PriceIn = pIn
		p.PriceOut = pOut

		// Calculate Capabilities
		p.Features = calculateFeatures(m)

		processedModels = append(processedModels, p)
	}

	// 3a. Auto-generate aliases from unique suffixes
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
			// If suffix is unique and not already an alias
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

	// 3b. Populate alias map
	for _, p := range processedModels {
		for _, alias := range p.Aliases {
			// Check for collisions in alias map (should be rare with unique suffixes + overrides)
			// But prioritizing overrides or first come. Sort later handles deterministic order if needed.
			// Ideally we warn on collision.
			lowerAlias := strings.ToLower(alias)
			if existingID, ok := aliasMap[lowerAlias]; ok && existingID != p.ID {
				log.Printf("Warning: Alias collision '%s' for models %s and %s. Keeping %s.", alias, existingID, p.ID, existingID)
			} else {
				aliasMap[lowerAlias] = p.ID
			}
		}
	}

	// Sort models for deterministic output
	sort.Slice(processedModels, func(i, j int) bool {
		return processedModels[i].ID < processedModels[j].ID
	})

	// 4. Generate Code
	if err := generateCode(processedModels, aliasMap); err != nil {
		log.Fatalf("Failed to generate code: %v", err)
	}

	log.Println("Generator finished successfully.")
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
			PriceInVal:    {{ printf "%f" .PriceIn }},
			PriceOutVal:   {{ printf "%f" .PriceOut }},
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

func loadOverrides(path string) (*OverrideData, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var overrides OverrideData
	if err := yaml.NewDecoder(f).Decode(&overrides); err != nil {
		return nil, err
	}
	return &overrides, nil
}

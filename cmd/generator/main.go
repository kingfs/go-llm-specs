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

	"gopkg.in/yaml.v3"
)

// OpenRouter model structures
type OpenRouterModel struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	ContextLength int                    `json:"context_length"`
	TopProvider   OpenRouterTopProvider  `json:"top_provider"`
	Architecture  OpenRouterArchitecture `json:"architecture"`
	Pricing       OpenRouterPricing      `json:"pricing"`
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

		// Popuplate alias map
		for _, alias := range p.Aliases {
			aliasMap[strings.ToLower(alias)] = p.ID
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
	modalities := strings.Split(m.Architecture.Modality, "+")
	for _, mod := range modalities {
		switch strings.TrimSpace(mod) {
		case "text":
			features = append(features, "ModalityTextIn", "ModalityTextOut")
		case "image":
			features = append(features, "ModalityImageIn")
		case "audio":
			features = append(features, "ModalityAudioIn", "ModalityAudioOut")
		}
	}

	if strings.Contains(strings.ToLower(m.Description), "function calling") || strings.Contains(strings.ToLower(m.Description), "tools") {
		features = append(features, "CapFunctionCall")
	}

	if strings.Contains(strings.ToLower(m.Description), "#multimodal") {
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
		return strings.Title(lower)
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
			Features:      {{ .Features }},
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

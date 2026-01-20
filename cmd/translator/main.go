package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// -- Data Structures --

type OpenRouterModel struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type OpenRouterResponse struct {
	Data []OpenRouterModel `json:"data"`
}

type OverrideData struct {
	Models map[string]ModelOverride `yaml:"models"`
}

type ModelOverride struct {
	NameCN        string   `yaml:"name_cn,omitempty"`
	Description   string   `yaml:"description,omitempty"`
	DescriptionCN string   `yaml:"description_cn,omitempty"`
	Aliases       []string `yaml:"aliases,omitempty"`
	Provider      string   `yaml:"provider,omitempty"`
}

// -- API Types --

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
}

type ChatResponse struct {
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
}

// -- Main --

func main() {
	godotenv.Load()

	apiKey := os.Getenv("LLM_API_KEY")
	if apiKey == "" {
		log.Fatal("LLM_API_KEY environment variable is required")
	}
	// Defaults
	apiBase := os.Getenv("LLM_BASE_URL")
	if apiBase == "" {
		apiBase = "https://api.openai.com/v1"
	}
	modelName := os.Getenv("LLM_MODEL")
	if modelName == "" {
		modelName = "gpt-4o-mini"
	}

	// 1. Load models.json
	models, err := loadModels("data/models.json")
	if err != nil {
		log.Fatalf("Failed to load models.json: %v", err)
	}

	// 2. Load overrides.yaml
	overridesPath := "data/overrides.yaml"
	overrides, err := loadOverrides(overridesPath)
	if err != nil {
		log.Fatalf("Failed to load overrides.yaml: %v", err)
	}
	if overrides.Models == nil {
		overrides.Models = make(map[string]ModelOverride)
	}

	// 3. Identify missing translations
	var pending []OpenRouterModel
	for _, m := range models {
		ov, exists := overrides.Models[m.ID]
		// Condition: Has English desc, but NO Chinese desc in override
		if m.Description != "" && (!exists || ov.DescriptionCN == "") {
			pending = append(pending, m)
		}
	}

	log.Printf("Found %d models needing translation.", len(pending))
	if len(pending) == 0 {
		return
	}

	// 4. Batch Process
	batchSize := 10
	totalBatches := (len(pending) + batchSize - 1) / batchSize

	for i := 0; i < len(pending); i += batchSize {
		end := i + batchSize
		if end > len(pending) {
			end = len(pending)
		}
		batch := pending[i:end]
		batchIdx := (i / batchSize) + 1
		log.Printf("Processing batch %d/%d (%d items)...", batchIdx, totalBatches, len(batch))

		translations, err := translateBatch(batch, apiKey, apiBase, modelName)
		if err != nil {
			log.Printf("Error translating batch %d: %v", batchIdx, err)
			continue // Skip to next batch, don't crash entire process
		}

		// Update overrides
		changes := 0
		for id, cnDesc := range translations {
			newDesc := cleanResult(cnDesc)
			if newDesc == "" {
				continue
			}

			entry := overrides.Models[id] // Get copy
			entry.DescriptionCN = newDesc
			overrides.Models[id] = entry // Set back
			changes++
		}

		// Save IMMEDIATELY after each batch to avoid data loss
		if changes > 0 {
			if err := saveOverrides(overridesPath, overrides); err != nil {
				log.Printf("Error saving overrides: %v", err)
			} else {
				log.Printf("Saved %d new translations.", changes)
			}
		}

		// Rate limit protection
		time.Sleep(1 * time.Second)
	}
}

// -- Helpers --

func loadModels(path string) ([]OpenRouterModel, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var resp OpenRouterResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

func loadOverrides(path string) (*OverrideData, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &OverrideData{}, nil
		}
		return nil, err
	}
	defer f.Close()
	var out OverrideData
	if err := yaml.NewDecoder(f).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func saveOverrides(path string, data *OverrideData) error {
	// Marshal first
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(data); err != nil {
		return err
	}

	// Write file
	return os.WriteFile(path, buf.Bytes(), 0644)
}

func translateBatch(batch []OpenRouterModel, key, base, model string) (map[string]string, error) {
	// Prepare input map: ID -> English Desc
	inputs := make(map[string]string)
	for _, m := range batch {
		inputs[m.ID] = m.Description
	}

	inputJSON, _ := json.MarshalIndent(inputs, "", "  ")

	prompt := fmt.Sprintf(`You are a professional technical translator for LLM (Large Language Specs).
Translate the values of the following JSON object into professional, concise Chinese.
Do not translate keys (Model IDs). Keep the structure exactly the same: valid JSON {"id": "translated_description"}.

Content to translate:
%s`, string(inputJSON))

	reqBody := ChatRequest{
		Model: model,
		Messages: []ChatMessage{
			{Role: "user", Content: prompt},
		},
	}

	jsonBody, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", base+"/chat/completions", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API Error %d: %s", resp.StatusCode, string(body))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, err
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from LLM")
	}

	rawContent := chatResp.Choices[0].Message.Content
	// Extract JSON from potential code blocks
	rawContent = strings.TrimPrefix(rawContent, "```json")
	rawContent = strings.TrimPrefix(rawContent, "```")
	rawContent = strings.TrimSuffix(rawContent, "```")

	var results map[string]string
	if err := json.Unmarshal([]byte(rawContent), &results); err != nil {
		log.Printf("Failed to parse LLM response as JSON: %s", rawContent)
		return nil, err
	}

	return results, nil
}

func cleanResult(s string) string {
	return strings.TrimSpace(s)
}

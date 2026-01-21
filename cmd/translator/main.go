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
	"strings"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// -- Data Structures --

type ModelRegistry struct {
	ID            string   `yaml:"id"`
	Name          string   `yaml:"name"`
	NameCN        string   `yaml:"name_cn,omitempty"`
	Provider      string   `yaml:"provider"`
	Description   string   `yaml:"description,omitempty"`
	DescriptionCN string   `yaml:"description_cn,omitempty"`
	ContextLen    int      `yaml:"context_length"`
	MaxOutput     int      `yaml:"max_output,omitempty"`
	PriceIn       float64  `yaml:"price_in"`
	PriceOut      float64  `yaml:"price_out"`
	Features      []string `yaml:"features,omitempty"`
	Aliases       []string `yaml:"aliases,omitempty"`

	// Internal helper
	filePath string `yaml:"-"`
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

	// 1. Scan models/ directory recursively
	log.Println("Scanning models/ directory...")
	registry, err := scanRegistry("models")
	if err != nil {
		log.Fatalf("Failed to scan models directory: %v", err)
	}
	log.Printf("Found %d models in registry.", len(registry))

	// 2. Identify missing translations
	var pending []*ModelRegistry
	for _, m := range registry {
		// Condition: Has English desc, but NO Chinese desc
		if m.Description != "" && m.DescriptionCN == "" {
			pending = append(pending, m)
		}
	}

	log.Printf("Found %d models needing translation.", len(pending))
	if len(pending) == 0 {
		return
	}

	// 3. Batch Process
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

		// Update and Save individually
		changes := 0
		for id, cnDesc := range translations {
			newDesc := cleanResult(cnDesc)
			if newDesc == "" {
				continue
			}

			// Find model in batch
			var target *ModelRegistry
			for _, m := range batch {
				if m.ID == id {
					target = m
					break
				}
			}

			if target != nil {
				target.DescriptionCN = newDesc
				if err := saveModel(target); err != nil {
					log.Printf("Error saving model %s: %v", id, err)
				} else {
					changes++
				}
			}
		}
		log.Printf("Saved %d new translations in batch %d.", changes, batchIdx)

		// Rate limit protection
		time.Sleep(1 * time.Second)
	}
}

// -- Helpers --

func scanRegistry(root string) ([]*ModelRegistry, error) {
	var models []*ModelRegistry
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

		var m ModelRegistry
		if err := yaml.NewDecoder(f).Decode(&m); err == nil && m.ID != "" {
			m.filePath = path
			models = append(models, &m)
		}
		return nil
	})
	return models, err
}

func saveModel(m *ModelRegistry) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(m); err != nil {
		return err
	}
	return os.WriteFile(m.filePath, buf.Bytes(), 0644)
}

func translateBatch(batch []*ModelRegistry, key, base, model string) (map[string]string, error) {
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
	rawContent = strings.TrimSpace(rawContent)
	if strings.HasPrefix(rawContent, "```json") {
		rawContent = strings.TrimPrefix(rawContent, "```json")
		rawContent = strings.TrimSuffix(rawContent, "```")
	} else if strings.HasPrefix(rawContent, "```") {
		rawContent = strings.TrimPrefix(rawContent, "```")
		rawContent = strings.TrimSuffix(rawContent, "```")
	}
	rawContent = strings.TrimSpace(rawContent)

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

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// -- Data Structures --

type ModelRegistry struct {
	ID           string            `yaml:"id"`
	Name         string            `yaml:"name"`
	NameCN       string            `yaml:"name_cn,omitempty"`
	Provider     string            `yaml:"provider"`
	Description  string            `yaml:"description,omitempty"`
	Descriptions map[string]string `yaml:"descriptions,omitempty"`
	ContextLen   int               `yaml:"context_length"`
	MaxOutput    int               `yaml:"max_output,omitempty"`
	Features     []string          `yaml:"features,omitempty"`
	Aliases      []string          `yaml:"aliases,omitempty"`

	// Legacy field for migration
	DescriptionCN string `yaml:"description_cn,omitempty"`

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

	var langsStr string
	flag.StringVar(&langsStr, "langs", "cn", "Comma-separated target languages for translation (e.g., cn, jp, ru, fr, vi)")

	var concurrency int
	flag.IntVar(&concurrency, "concurrency", 5, "Number of concurrent translation tasks")
	flag.Parse()

	targetLangs := strings.Split(langsStr, ",")
	for i, l := range targetLangs {
		targetLangs[i] = strings.TrimSpace(l)
	}

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

	// 2. Identify models missing any of the target languages
	var pending []*ModelRegistry
	for _, m := range registry {
		if m.Description == "" {
			continue
		}
		if m.Descriptions == nil {
			m.Descriptions = make(map[string]string)
		}
		missing := false
		for _, lang := range targetLangs {
			if lang == "" || lang == "en" {
				continue
			}
			if _, ok := m.Descriptions[lang]; !ok {
				missing = true
				break
			}
		}
		if missing {
			pending = append(pending, m)
		}
	}

	log.Printf("Found %d models needing translation to %v.", len(pending), targetLangs)
	if len(pending) == 0 {
		return
	}

	// 3. Process concurrently
	// Use a semaphore to limit concurrency
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	log.Printf("Starting translation with concurrency=%d...", concurrency)

	for i, m := range pending {
		wg.Add(1)
		sem <- struct{}{} // Acquire token

		go func(idx int, target *ModelRegistry) {
			defer wg.Done()
			defer func() { <-sem }() // Release token

			log.Printf("[%d/%d] Processing %s...", idx+1, len(pending), target.ID)

			// We treat each model as a batch of 1
			batch := []*ModelRegistry{target}
			translations, err := translateBatchMulti(batch, targetLangs, apiKey, apiBase, modelName)
			if err != nil {
				log.Printf("[%d/%d] Error translating %s: %v", idx+1, len(pending), target.ID, err)
				return
			}

			// Update and Save
			langMap, ok := translations[target.ID]
			if !ok {
				// Might happen if LLM returns weird ID or format
				log.Printf("[%d/%d] No translation returned for %s", idx+1, len(pending), target.ID)
				return
			}

			updated := false
			if target.Descriptions == nil {
				target.Descriptions = make(map[string]string)
			}
			for lang, desc := range langMap {
				cleanDesc := cleanResult(desc)
				if cleanDesc != "" {
					target.Descriptions[lang] = cleanDesc
					updated = true
				}
			}

			if updated {
				if err := saveModel(target); err != nil {
					log.Printf("[%d/%d] Error saving model %s: %v", idx+1, len(pending), target.ID, err)
				} else {
					log.Printf("[%d/%d] Saved updates for %s", idx+1, len(pending), target.ID)
				}
			}
		}(i, m)
	}

	wg.Wait()
	log.Println("All tasks completed.")
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

func translateBatchMulti(batch []*ModelRegistry, targetLangs []string, key, base, model string) (map[string]map[string]string, error) {
	// Prepare input map: ID -> English Desc
	inputs := make(map[string]string)
	for _, m := range batch {
		inputs[m.ID] = m.Description
	}

	inputJSON, _ := json.MarshalIndent(inputs, "", "  ")

	prompt := fmt.Sprintf(`You are a professional technical translator for Large Language Models.
Translate the descriptions in the following JSON object into these languages: %s.

Format your response as a valid JSON object where keys are Model IDs and values are objects containing the translations for each language.
Example structure:
{
  "model-id": {
    "lang-code": "translated description",
    ...
  }
}

Content to translate:
%s`, strings.Join(targetLangs, ", "), string(inputJSON))

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

	client := &http.Client{Timeout: 3 * time.Minute} // Longer timeout for multi-lang
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
	// Extract JSON
	rawContent = strings.TrimSpace(rawContent)
	if strings.HasPrefix(rawContent, "```json") {
		rawContent = strings.TrimPrefix(rawContent, "```json")
		rawContent = strings.TrimSuffix(rawContent, "```")
	} else if strings.HasPrefix(rawContent, "```") {
		rawContent = strings.TrimPrefix(rawContent, "```")
		rawContent = strings.TrimSuffix(rawContent, "```")
	}
	rawContent = strings.TrimSpace(rawContent)

	var results map[string]map[string]string
	if err := json.Unmarshal([]byte(rawContent), &results); err != nil {
		log.Printf("Failed to parse LLM response as JSON: %s", rawContent)
		return nil, err
	}

	return results, nil
}

func cleanResult(s string) string {
	return strings.TrimSpace(s)
}

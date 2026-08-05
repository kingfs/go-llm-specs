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
	"sort"
	"strings"
	"time"

	"github.com/joho/godotenv"
	registrymodel "github.com/kingfs/go-llm-specs/internal/registry"
)

type ModelRegistry = registrymodel.Model

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

type translatorConfig struct {
	ModelsDir    string
	BatchSize    int
	Limit        int
	Provider     string
	IDPrefix     string
	OnlyMissing  bool
	DryRun       bool
	RequestDelay time.Duration
	APIBase      string
	APIKey       string
	Model        string
}

func main() {
	godotenv.Load()

	cfg, err := parseFlags()
	if err != nil {
		log.Fatal(err)
	}

	if err := run(cfg); err != nil {
		log.Fatal(err)
	}
}

func parseFlags() (translatorConfig, error) {
	cfg := translatorConfig{}

	flag.StringVar(&cfg.ModelsDir, "models-dir", "models", "directory containing model yaml files")
	flag.IntVar(&cfg.BatchSize, "batch-size", 10, "number of models translated per API request")
	flag.IntVar(&cfg.Limit, "limit", 0, "maximum number of models to translate in this run, 0 means no limit")
	flag.StringVar(&cfg.Provider, "provider", "", "optional provider filter, matched case-insensitively against yaml provider")
	flag.StringVar(&cfg.IDPrefix, "id-prefix", "", "optional model ID prefix filter, for example openai/ or qwen/")
	flag.BoolVar(&cfg.OnlyMissing, "only-missing", true, "translate only models without description_cn")
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "print selected models without calling the translation API")
	flag.DurationVar(&cfg.RequestDelay, "delay", time.Second, "delay between translation batches")
	flag.Parse()

	cfg.APIKey = strings.TrimSpace(os.Getenv("LLM_API_KEY"))
	cfg.APIBase = strings.TrimSpace(os.Getenv("LLM_BASE_URL"))
	cfg.Model = strings.TrimSpace(os.Getenv("LLM_MODEL"))

	if cfg.APIBase == "" {
		cfg.APIBase = "https://api.openai.com/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-4o-mini"
	}
	if !cfg.DryRun && cfg.APIKey == "" {
		return cfg, fmt.Errorf("LLM_API_KEY environment variable is required")
	}
	if cfg.BatchSize <= 0 {
		return cfg, fmt.Errorf("batch-size must be greater than 0")
	}

	return cfg, nil
}

func run(cfg translatorConfig) error {
	log.Printf("Scanning %s for model yaml files...", cfg.ModelsDir)
	registry, err := scanRegistry(cfg.ModelsDir)
	if err != nil {
		return fmt.Errorf("scan models directory: %w", err)
	}
	log.Printf("Loaded %d models from registry", len(registry))

	pending := collectPendingTranslations(registry, cfg)
	log.Printf("Selected %d models for translation", len(pending))
	if len(pending) == 0 {
		return nil
	}

	if cfg.DryRun {
		for _, model := range pending {
			log.Printf("dry-run: %s (%s)", model.ID, model.FilePath)
		}
		return nil
	}

	totalBatches := (len(pending) + cfg.BatchSize - 1) / cfg.BatchSize
	failedBatches := 0
	for i := 0; i < len(pending); i += cfg.BatchSize {
		end := min(i+cfg.BatchSize, len(pending))
		batch := pending[i:end]
		batchIdx := (i / cfg.BatchSize) + 1

		log.Printf("Translating batch %d/%d (%d items)", batchIdx, totalBatches, len(batch))
		translations, err := translateBatch(batch, cfg.APIKey, cfg.APIBase, cfg.Model)
		if err != nil {
			log.Printf("Batch %d failed: %v", batchIdx, err)
			failedBatches++
			continue
		}

		changed := 0
		for _, target := range batch {
			newDesc := cleanResult(translations[target.ID])
			if newDesc == "" || newDesc == target.DescriptionCN {
				continue
			}

			target.DescriptionCN = newDesc
			if err := saveModel(target); err != nil {
				log.Printf("Save failed for %s: %v", target.ID, err)
				continue
			}
			changed++
		}

		log.Printf("Saved %d translated models in batch %d", changed, batchIdx)
		if end < len(pending) {
			time.Sleep(cfg.RequestDelay)
		}
	}
	if failedBatches > 0 {
		return fmt.Errorf("translation completed with %d/%d failed batches", failedBatches, totalBatches)
	}
	return nil
}

func scanRegistry(root string) ([]*ModelRegistry, error) {
	var models []*ModelRegistry
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || (!strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml")) {
			return nil
		}

		m, err := registrymodel.Load(path)
		if err == nil && m.ID != "" {
			models = append(models, &m)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})
	return models, nil
}

func collectPendingTranslations(registry []*ModelRegistry, cfg translatorConfig) []*ModelRegistry {
	pending := make([]*ModelRegistry, 0, len(registry))
	for _, m := range registry {
		if m.Description == "" {
			continue
		}
		if cfg.OnlyMissing && strings.TrimSpace(m.DescriptionCN) != "" {
			continue
		}
		if cfg.Provider != "" && !strings.EqualFold(strings.TrimSpace(m.Provider), strings.TrimSpace(cfg.Provider)) {
			continue
		}
		if cfg.IDPrefix != "" && !strings.HasPrefix(strings.ToLower(m.ID), strings.ToLower(cfg.IDPrefix)) {
			continue
		}
		pending = append(pending, m)
	}
	sort.SliceStable(pending, func(i, j int) bool {
		left, right := pending[i].DiscoveredAt, pending[j].DiscoveredAt
		if left != nil && right != nil && !left.Equal(*right) {
			return left.After(*right)
		}
		if left != nil && right == nil {
			return true
		}
		if left == nil && right != nil {
			return false
		}
		return pending[i].ID < pending[j].ID
	})

	if cfg.Limit > 0 && len(pending) > cfg.Limit {
		pending = pending[:cfg.Limit]
	}
	return pending
}

func saveModel(m *ModelRegistry) error {
	return registrymodel.Save(m.FilePath, *m)
}

func translateBatch(batch []*ModelRegistry, key, base, model string) (map[string]string, error) {
	inputs := make(map[string]string, len(batch))
	for _, m := range batch {
		inputs[m.ID] = m.Description
	}

	inputJSON, _ := json.MarshalIndent(inputs, "", "  ")
	prompt := fmt.Sprintf(`You are a professional technical translator for LLM model metadata.
Translate only the JSON values into concise, accurate Simplified Chinese.
Do not translate keys.
Return valid JSON only, with the exact same keys.

Input JSON:
%s`, string(inputJSON))

	reqBody := ChatRequest{
		Model: model,
		Messages: []ChatMessage{
			{Role: "user", Content: prompt},
		},
	}

	jsonBody, _ := json.Marshal(reqBody)

	req, err := http.NewRequest("POST", strings.TrimRight(base, "/")+"/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, err
	}
	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from LLM")
	}

	rawContent := strings.TrimSpace(chatResp.Choices[0].Message.Content)
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

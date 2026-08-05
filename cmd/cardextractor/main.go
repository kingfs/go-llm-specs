package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kingfs/go-llm-specs/internal/aiclient"
	"github.com/kingfs/go-llm-specs/internal/registry"
	"github.com/kingfs/go-llm-specs/internal/suggestion"
)

type config struct {
	ModelsDir, ModelID, Input, Output, HFBase string
	SuggestionsDir                            string
	APIBase, APIKeyEnv, AIModel, WireAPI      string
	ReasoningEffort                           string
	MaxChars                                  int
	Timeout                                   time.Duration
	SkipCurrent                               bool
}

type aiClaims struct {
	Claims []suggestion.Claim `json:"claims"`
}

func main() {
	cfg := parseFlags()
	if err := run(context.Background(), cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.ModelsDir, "models-dir", "models", "directory containing model YAML files")
	flag.StringVar(&cfg.ModelID, "model", "", "registry model ID")
	flag.StringVar(&cfg.Input, "input", "", "optional local model-card Markdown path")
	flag.StringVar(&cfg.Output, "output", "", "suggestion output path")
	flag.StringVar(&cfg.SuggestionsDir, "suggestions-dir", "data/suggestions", "directory used for content-addressed suggestion reuse")
	flag.StringVar(&cfg.HFBase, "hf-base", "https://huggingface.co", "Hugging Face base URL")
	flag.StringVar(&cfg.APIBase, "api-base", envOr("LLM_BASE_URL", "http://localhost:8000/v1"), "OpenAI-compatible API base URL")
	flag.StringVar(&cfg.APIKeyEnv, "api-key-env", "LLM_API_KEY", "environment variable containing API key")
	flag.StringVar(&cfg.AIModel, "ai-model", envOr("LLM_MODEL", ""), "extractor model serving ID")
	flag.StringVar(&cfg.WireAPI, "wire-api", "responses", "wire API: responses or chat")
	flag.StringVar(&cfg.ReasoningEffort, "reasoning-effort", envOr("LLM_REASONING_EFFORT", "none"), "optional reasoning effort; none is recommended for structured JSON")
	flag.IntVar(&cfg.MaxChars, "max-chars", 60000, "maximum model-card characters sent to AI")
	flag.DurationVar(&cfg.Timeout, "timeout", 5*time.Minute, "AI and download timeout")
	flag.BoolVar(&cfg.SkipCurrent, "skip-current", true, "skip when output matches source revision and SHA")
	flag.Parse()
	return cfg
}

func run(ctx context.Context, cfg config) error {
	if cfg.ModelID == "" || cfg.AIModel == "" || cfg.MaxChars <= 0 {
		return fmt.Errorf("-model, -ai-model, and positive -max-chars are required")
	}
	model, err := findModel(cfg.ModelsDir, cfg.ModelID)
	if err != nil {
		return err
	}
	content, sourceURL, revision, err := loadCard(ctx, cfg, model)
	if err != nil {
		return err
	}
	if len(content) > cfg.MaxChars {
		content = content[:cfg.MaxChars]
	}
	digest := sha256.Sum256(content)
	digestText := fmt.Sprintf("%x", digest)
	output := cfg.Output
	if output == "" {
		output = filepath.Join(cfg.SuggestionsDir, filepath.FromSlash(safeSuggestionID(model.ID))+".model-card.json")
	}
	if cfg.SkipCurrent {
		if existing, loadErr := suggestion.Load(output); loadErr == nil && existing.Source.Revision == revision && existing.Source.SHA256 == digestText {
			fmt.Printf("current suggestion already exists at %s\n", output)
			return nil
		}
	}
	if reused, reuseErr := findReusable(cfg.SuggestionsDir, output, revision, digestText); reuseErr != nil {
		return reuseErr
	} else if reused != nil {
		reused.ModelID, reused.CreatedAt, reused.Status = model.ID, time.Now().UTC(), "pending"
		if err := suggestion.Save(output, *reused); err != nil {
			return err
		}
		fmt.Printf("reused %d claims from identical source at %s\n", len(reused.Claims), output)
		return nil
	}
	client, err := aiclient.New(aiclient.Config{
		BaseURL: cfg.APIBase, APIKey: strings.TrimSpace(os.Getenv(cfg.APIKeyEnv)), Model: cfg.AIModel,
		WireAPI: cfg.WireAPI, Timeout: cfg.Timeout, Retries: 2, ReasoningEffort: cfg.ReasoningEffort, JSONSchema: claimsSchema(),
	})
	if err != nil {
		return err
	}
	var extracted aiClaims
	if err := client.JSON(ctx, extractionPrompt(model, string(content)), &extracted); err != nil {
		return err
	}
	validClaims := make([]suggestion.Claim, 0, len(extracted.Claims))
	for _, claim := range extracted.Claims {
		if err := suggestion.ValidateClaim(claim); err != nil {
			fmt.Fprintf(os.Stderr, "dropping invalid %s claim: %v\n", claim.Field, err)
			continue
		}
		validClaims = append(validClaims, claim)
	}
	if len(validClaims) == 0 {
		return fmt.Errorf("AI response contained no valid claims")
	}
	document := suggestion.Document{
		SchemaVersion: suggestion.CurrentSchemaVersion, Kind: "model_card", ModelID: model.ID,
		Status: "pending", CreatedAt: time.Now().UTC(),
		Source:    suggestion.Source{URL: sourceURL, Revision: revision, SHA256: digestText},
		Generator: suggestion.Generator{Model: cfg.AIModel, WireAPI: cfg.WireAPI}, Claims: validClaims,
	}
	if err := suggestion.Save(output, document); err != nil {
		return err
	}
	fmt.Printf("wrote %d pending claims to %s\n", len(document.Claims), output)
	return nil
}

func safeSuggestionID(id string) string { return strings.ReplaceAll(id, ":", "_") }

func claimsSchema() map[string]any {
	claim := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"field", "value", "evidence", "section", "confidence"},
		"properties": map[string]any{
			"field": map[string]any{"type": "string", "enum": []string{"description", "context_length", "max_output", "features", "reasoning.supported", "reasoning.parser"}},
			"value": map[string]any{}, "evidence": map[string]any{"type": "string"}, "section": map[string]any{"type": "string"},
			"confidence": map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}},
		},
	}
	return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"claims"}, "properties": map[string]any{"claims": map[string]any{"type": "array", "maxItems": 12, "items": claim}}}
}

func findReusable(root, target, revision, digest string) (*suggestion.Document, error) {
	var match *suggestion.Document
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil || info.IsDir() || path == target || !strings.HasSuffix(path, ".model-card.json") {
			return err
		}
		document, loadErr := suggestion.Load(path)
		if loadErr != nil {
			return loadErr
		}
		if document.Source.Revision == revision && document.Source.SHA256 == digest {
			copy := document
			match = &copy
		}
		return nil
	})
	return match, err
}

func findModel(root, id string) (registry.Model, error) {
	models, err := registry.Scan(root)
	if err != nil {
		return registry.Model{}, err
	}
	for _, model := range models {
		if strings.EqualFold(model.ID, id) {
			return model, nil
		}
	}
	return registry.Model{}, fmt.Errorf("model %q not found", id)
}

func loadCard(ctx context.Context, cfg config, model registry.Model) ([]byte, string, string, error) {
	if cfg.Input != "" {
		data, err := os.ReadFile(cfg.Input)
		return data, cfg.Input, "local", err
	}
	if model.Upstream.HuggingFace == nil || model.Upstream.HuggingFace.ID == "" {
		return nil, "", "", fmt.Errorf("%s has no Hugging Face ID", model.ID)
	}
	hfID := model.Upstream.HuggingFace.ID
	revision := model.Upstream.HuggingFace.Revision
	client := &http.Client{Timeout: cfg.Timeout}
	if revision == "" {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(cfg.HFBase, "/")+"/api/models/"+hfID, nil)
		resp, err := client.Do(req)
		if err != nil {
			return nil, "", "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, "", "", fmt.Errorf("Hugging Face metadata returned %s", resp.Status)
		}
		var metadata struct {
			SHA string `json:"sha"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
			return nil, "", "", err
		}
		revision = metadata.SHA
	}
	if revision == "" {
		return nil, "", "", fmt.Errorf("Hugging Face revision is unavailable")
	}
	url := strings.TrimRight(cfg.HFBase, "/") + "/" + hfID + "/resolve/" + revision + "/README.md"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", "", fmt.Errorf("Hugging Face README returned %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(cfg.MaxChars)+1))
	return data, url, revision, err
}

func extractionPrompt(model registry.Model, card string) string {
	return fmt.Sprintf(`Extract only explicitly supported facts from this model card for registry model %q.
Return one JSON object: {"claims":[...]}, with at most 12 claims. Each claim must have:
- field: one of description, context_length, max_output, features, reasoning.supported, reasoning.parser
- value: correctly typed JSON value
- evidence: an exact quote copied from the card, at most 200 characters
- section: nearest Markdown heading
- confidence: high, medium, or low
For features, value must be an array containing only applicable registry constants:
CapChat, CapFunctionCall, CapJsonMode, ModalityTextIn, ModalityTextOut, ModalityImageIn,
ModalityAudioIn, ModalityAudioOut, ModalityVideoIn. Do not put prose in features.
Do not infer runtime behavior. Do not claim tool calling, JSON mode, parallel tools, or context limits unless the quoted text explicitly states it. Omit unsupported facts. Keep description factual and under 600 characters.

MODEL CARD:
%s`, model.ID, card)
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

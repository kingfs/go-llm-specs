package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type config struct {
	BaseURL   string
	Model     string
	WireAPI   string
	Server    string
	APIKeyEnv string
	Output    string
	Timeout   time.Duration
	Image     bool
}

type report struct {
	Model         string            `json:"model"`
	BaseURL       string            `json:"base_url"`
	WireAPI       string            `json:"wire_api"`
	Server        string            `json:"server,omitempty"`
	ServerVersion string            `json:"server_version,omitempty"`
	TestedAt      string            `json:"tested_at"`
	Results       map[string]result `json:"results"`
}

type result struct {
	Status     string `json:"status"`
	HTTPStatus int    `json:"http_status,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

type probeCase struct {
	Name string
	Body map[string]any
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
	flag.StringVar(&cfg.BaseURL, "base-url", "http://localhost:8000/v1", "OpenAI-compatible API base URL")
	flag.StringVar(&cfg.Model, "model", "", "serving model ID")
	flag.StringVar(&cfg.WireAPI, "wire-api", "chat", "wire API: chat or responses")
	flag.StringVar(&cfg.Server, "server", "", "server label, for example vllm or sglang")
	flag.StringVar(&cfg.APIKeyEnv, "api-key-env", "LLM_API_KEY", "environment variable containing the API key")
	flag.StringVar(&cfg.Output, "output", "", "optional JSON report path; stdout when empty")
	flag.DurationVar(&cfg.Timeout, "timeout", 30*time.Second, "timeout per probe request")
	flag.BoolVar(&cfg.Image, "image", false, "include a minimal image-input probe")
	flag.Parse()
	return cfg
}

func run(ctx context.Context, cfg config) error {
	if cfg.Model == "" {
		return fmt.Errorf("-model is required")
	}
	if cfg.WireAPI != "chat" && cfg.WireAPI != "responses" {
		return fmt.Errorf("unsupported wire API %q", cfg.WireAPI)
	}
	client := &http.Client{Timeout: cfg.Timeout}
	key := strings.TrimSpace(os.Getenv(cfg.APIKeyEnv))
	report, err := executeProbes(ctx, client, cfg, key)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if cfg.Output == "" {
		_, err = os.Stdout.Write(data)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Output), 0o755); err != nil {
		return err
	}
	return os.WriteFile(cfg.Output, data, 0o644)
}

func executeProbes(ctx context.Context, client *http.Client, cfg config, key string) (report, error) {
	results := make(map[string]result)
	modelsURL := strings.TrimRight(cfg.BaseURL, "/") + "/models"
	modelsResult, err := discoverModel(ctx, client, modelsURL, key, cfg.Model)
	if err != nil {
		return report{}, fmt.Errorf("model discovery: %w", err)
	}
	results["model_discovery"] = modelsResult

	version := fetchVersion(ctx, client, cfg.BaseURL, key)
	for _, probe := range buildProbeCases(cfg) {
		probeResult, err := postProbe(ctx, client, requestURL(cfg), key, probe.Body)
		if err != nil {
			results[probe.Name] = result{Status: "error", Detail: err.Error()}
			continue
		}
		results[probe.Name] = probeResult
	}
	return report{
		Model:         cfg.Model,
		BaseURL:       cfg.BaseURL,
		WireAPI:       cfg.WireAPI,
		Server:        cfg.Server,
		ServerVersion: version,
		TestedAt:      time.Now().UTC().Format(time.RFC3339),
		Results:       results,
	}, nil
}

func buildProbeCases(cfg config) []probeCase {
	base := baseRequest(cfg, "Reply with exactly: ok")
	cases := []probeCase{{Name: "text_generation", Body: base}}

	tools := []any{map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "get_weather",
			"description": "Get weather for a city",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{"city": map[string]any{"type": "string"}},
				"required":   []string{"city"},
			},
		},
	}}
	if cfg.WireAPI == "responses" {
		tools = []any{map[string]any{
			"type":        "function",
			"name":        "get_weather",
			"description": "Get weather for a city",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{"city": map[string]any{"type": "string"}},
				"required":   []string{"city"},
			},
		}}
	}
	toolBody := baseRequest(cfg, "Call get_weather for Shanghai.")
	toolBody["tools"] = tools
	cases = append(cases, probeCase{Name: "tool_calling", Body: toolBody})

	parallelBody := baseRequest(cfg, "Call get_weather for Shanghai and Beijing.")
	parallelBody["tools"] = tools
	parallelBody["parallel_tool_calls"] = true
	cases = append(cases, probeCase{Name: "parallel_tool_calls", Body: parallelBody})

	jsonBody := baseRequest(cfg, "Return JSON with one boolean field named ok.")
	if cfg.WireAPI == "responses" {
		jsonBody["text"] = map[string]any{"format": map[string]any{"type": "json_object"}}
	} else {
		jsonBody["response_format"] = map[string]any{"type": "json_object"}
	}
	cases = append(cases, probeCase{Name: "json_output", Body: jsonBody})

	reasoningBody := baseRequest(cfg, "What is 2+2?")
	reasoningBody["reasoning"] = map[string]any{"effort": "medium", "summary": "auto"}
	cases = append(cases, probeCase{Name: "reasoning", Body: reasoningBody})

	if cfg.Image {
		imageBody := imageRequest(cfg)
		cases = append(cases, probeCase{Name: "image_input", Body: imageBody})
	}
	sort.SliceStable(cases, func(i, j int) bool { return cases[i].Name < cases[j].Name })
	return cases
}

func baseRequest(cfg config, prompt string) map[string]any {
	request := map[string]any{"model": cfg.Model, "max_tokens": 32}
	if cfg.WireAPI == "responses" {
		request["input"] = prompt
		request["max_output_tokens"] = 32
		delete(request, "max_tokens")
	} else {
		request["messages"] = []any{map[string]any{"role": "user", "content": prompt}}
	}
	return request
}

func imageRequest(cfg config) map[string]any {
	// One transparent PNG pixel. The probe verifies request acceptance, not vision quality.
	imageURL := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	request := map[string]any{"model": cfg.Model}
	content := []any{
		map[string]any{"type": "input_text", "text": "Describe this image briefly."},
		map[string]any{"type": "input_image", "image_url": imageURL},
	}
	if cfg.WireAPI == "responses" {
		request["input"] = []any{map[string]any{"role": "user", "content": content}}
	} else {
		content[0] = map[string]any{"type": "text", "text": "Describe this image briefly."}
		content[1] = map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageURL}}
		request["messages"] = []any{map[string]any{"role": "user", "content": content}}
	}
	return request
}

func requestURL(cfg config) string {
	base := strings.TrimRight(cfg.BaseURL, "/")
	if cfg.WireAPI == "responses" {
		return base + "/responses"
	}
	return base + "/chat/completions"
}

func getProbe(ctx context.Context, client *http.Client, target, key string) (result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return result{}, err
	}
	setHeaders(req, key)
	resp, err := client.Do(req)
	if err != nil {
		return result{}, err
	}
	defer resp.Body.Close()
	return classifyResponse(resp), nil
}

func discoverModel(ctx context.Context, client *http.Client, target, key, modelID string) (result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return result{}, err
	}
	setHeaders(req, key)
	resp, err := client.Do(req)
	if err != nil {
		return result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return classifyResponse(resp), nil
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return result{Status: "fail", HTTPStatus: resp.StatusCode, Detail: "invalid models response: " + err.Error()}, nil
	}
	for _, candidate := range payload.Data {
		if candidate.ID == modelID {
			return result{Status: "pass", HTTPStatus: resp.StatusCode}, nil
		}
	}
	return result{Status: "fail", HTTPStatus: resp.StatusCode, Detail: "model ID not advertised by endpoint"}, nil
}

func postProbe(ctx context.Context, client *http.Client, target, key string, body map[string]any) (result, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(data))
	if err != nil {
		return result{}, err
	}
	setHeaders(req, key)
	resp, err := client.Do(req)
	if err != nil {
		return result{}, err
	}
	defer resp.Body.Close()
	return classifyResponse(resp), nil
}

func classifyResponse(resp *http.Response) result {
	detailBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	detail := strings.TrimSpace(string(detailBytes))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return result{Status: "pass", HTTPStatus: resp.StatusCode}
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusUnprocessableEntity {
		return result{Status: "unsupported", HTTPStatus: resp.StatusCode, Detail: detail}
	}
	return result{Status: "fail", HTTPStatus: resp.StatusCode, Detail: detail}
}

func fetchVersion(ctx context.Context, client *http.Client, baseURL, key string) string {
	base := strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/v1")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/version", nil)
	if err != nil {
		return ""
	}
	setHeaders(req, key)
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var payload map[string]any
	if json.NewDecoder(resp.Body).Decode(&payload) != nil {
		return ""
	}
	for _, key := range []string{"version", "vllm_version", "sglang_version"} {
		if value, ok := payload[key].(string); ok {
			return value
		}
	}
	return ""
}

func setHeaders(req *http.Request, key string) {
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
}

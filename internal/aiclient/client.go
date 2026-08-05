package aiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	BaseURL         string
	APIKey          string
	Model           string
	WireAPI         string
	Timeout         time.Duration
	Retries         int
	ReasoningEffort string
	JSONSchema      map[string]any
}

type Client struct {
	config Config
	http   *http.Client
}

func New(config Config) (*Client, error) {
	if config.BaseURL == "" || config.Model == "" {
		return nil, fmt.Errorf("AI base URL and model are required")
	}
	if config.WireAPI == "" {
		config.WireAPI = "responses"
	}
	if config.WireAPI != "responses" && config.WireAPI != "chat" {
		return nil, fmt.Errorf("unsupported wire API %q", config.WireAPI)
	}
	if config.Timeout == 0 {
		config.Timeout = 2 * time.Minute
	}
	return &Client{config: config, http: &http.Client{Timeout: config.Timeout}}, nil
}

func (c *Client) JSON(ctx context.Context, prompt string, target any) error {
	text, err := c.Complete(ctx, prompt)
	if err != nil {
		return err
	}
	text = normalizeJSONText(stripFence(stripLeadingThinkBlocks(text)))
	if err := json.Unmarshal([]byte(text), target); err != nil {
		return fmt.Errorf("decode AI JSON output: %w", err)
	}
	return nil
}

// stripLeadingThinkBlocks removes reasoning emitted ahead of the requested
// answer by models that serialize their chain of thought in <think> tags.
// Only complete, leading blocks are removed so JSON string values containing
// the same literal text are never modified.
func stripLeadingThinkBlocks(text string) string {
	text = strings.TrimSpace(text)
	for strings.HasPrefix(strings.ToLower(text), "<think>") {
		end := strings.Index(strings.ToLower(text), "</think>")
		if end < 0 {
			return text
		}
		text = strings.TrimSpace(text[end+len("</think>"):])
	}
	return text
}

func normalizeJSONText(text string) string {
	text = strings.TrimSpace(text)
	var wrapped string
	if json.Unmarshal([]byte(text), &wrapped) == nil {
		return strings.TrimSpace(wrapped)
	}
	if strings.Contains(text, `\"`) {
		candidate := strings.ReplaceAll(text, `\"`, `"`)
		candidate = strings.ReplaceAll(candidate, `\n`, "\n")
		if json.Valid([]byte(candidate)) {
			return candidate
		}
	}
	if start, end := strings.IndexByte(text, '{'), strings.LastIndexByte(text, '}'); start >= 0 && end > start {
		candidate := text[start : end+1]
		if json.Valid([]byte(candidate)) {
			return candidate
		}
	}
	return text
}

func (c *Client) Complete(ctx context.Context, prompt string) (string, error) {
	body := map[string]any{"model": c.config.Model}
	endpoint := strings.TrimRight(c.config.BaseURL, "/")
	if c.config.WireAPI == "responses" {
		endpoint += "/responses"
		body["input"] = prompt
		body["max_output_tokens"] = 8192
		if c.config.ReasoningEffort != "" {
			body["reasoning"] = map[string]any{"effort": c.config.ReasoningEffort}
		}
		format := map[string]any{"type": "json_object"}
		if c.config.JSONSchema != nil {
			format = map[string]any{"type": "json_schema", "name": "structured_output", "strict": true, "schema": c.config.JSONSchema}
		}
		body["text"] = map[string]any{"format": format}
	} else {
		endpoint += "/chat/completions"
		body["messages"] = []any{map[string]any{"role": "user", "content": prompt}}
		body["max_tokens"] = 4096
		body["response_format"] = map[string]any{"type": "json_object"}
		if c.config.ReasoningEffort != "" {
			body["reasoning_effort"] = c.config.ReasoningEffort
		}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	attempts := c.config.Retries + 1
	for attempt := 0; attempt < attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		if c.config.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
		}
		resp, err := c.http.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			defer resp.Body.Close()
			return decodeText(resp.Body, c.config.WireAPI)
		}
		if resp != nil {
			detail, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			_ = resp.Body.Close()
			if resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
				return "", fmt.Errorf("AI API returned %s: %s", resp.Status, strings.TrimSpace(string(detail)))
			}
		}
		if attempt+1 == attempts {
			if err != nil {
				return "", err
			}
			return "", fmt.Errorf("AI API retries exhausted")
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Second * time.Duration(1<<attempt)):
		}
	}
	return "", fmt.Errorf("AI API retries exhausted")
}

func decodeText(reader io.Reader, wireAPI string) (string, error) {
	var document map[string]any
	if err := json.NewDecoder(reader).Decode(&document); err != nil {
		return "", err
	}
	if wireAPI == "responses" {
		var parts []string
		output, _ := document["output"].([]any)
		for _, item := range output {
			object, _ := item.(map[string]any)
			content, _ := object["content"].([]any)
			for _, raw := range content {
				part, _ := raw.(map[string]any)
				if part["type"] == "output_text" {
					if text, ok := part["text"].(string); ok {
						parts = append(parts, text)
					}
				}
			}
		}
		if len(parts) == 0 {
			return "", fmt.Errorf("AI response contained no output_text")
		}
		return strings.Join(parts, "\n"), nil
	}
	choices, _ := document["choices"].([]any)
	if len(choices) == 0 {
		return "", fmt.Errorf("AI response contained no choices")
	}
	choice, _ := choices[0].(map[string]any)
	message, _ := choice["message"].(map[string]any)
	text, _ := message["content"].(string)
	if text == "" {
		return "", fmt.Errorf("AI response contained no message content")
	}
	return text, nil
}

func stripFence(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		if newline := strings.IndexByte(text, '\n'); newline >= 0 {
			text = text[newline+1:]
		}
		text = strings.TrimSuffix(strings.TrimSpace(text), "```")
	}
	return strings.TrimSpace(text)
}

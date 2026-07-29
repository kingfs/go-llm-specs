package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteProbesClassifiesCapabilities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version":
			_ = json.NewEncoder(w).Encode(map[string]any{"version": "test-vllm"})
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "qwen"}}})
		case "/v1/chat/completions":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if parallel, _ := body["parallel_tool_calls"].(bool); parallel {
				_ = json.NewEncoder(w).Encode(chatToolResponse(1))
				return
			}
			if _, ok := body["tools"]; ok {
				_ = json.NewEncoder(w).Encode(chatToolResponse(1))
				return
			}
			if _, ok := body["response_format"]; ok {
				_ = json.NewEncoder(w).Encode(chatTextResponse(`{"ok":true}`))
				return
			}
			_ = json.NewEncoder(w).Encode(chatTextResponse("ok"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	report, err := executeProbes(context.Background(), server.Client(), config{
		BaseURL: server.URL + "/v1",
		Model:   "qwen",
		WireAPI: "chat",
		Server:  "vllm",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if report.ServerVersion != "test-vllm" {
		t.Fatalf("version not detected: %#v", report)
	}
	if report.Results["text_generation"].Status != "pass" {
		t.Fatalf("text probe failed: %#v", report.Results)
	}
	if report.Results["model_discovery"].Status != "pass" {
		t.Fatalf("model discovery failed: %#v", report.Results)
	}
	if report.Results["parallel_tool_calls"].Status != "fail" {
		t.Fatalf("parallel probe misclassified: %#v", report.Results)
	}
}

func chatTextResponse(text string) map[string]any {
	return map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": text}}}}
}

func chatToolResponse(count int) map[string]any {
	calls := make([]any, count)
	for i := range calls {
		calls[i] = map[string]any{"id": "call", "type": "function", "function": map[string]any{"name": "get_weather", "arguments": `{"city":"Shanghai"}`}}
	}
	return map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": nil, "tool_calls": calls}}}}
}

func TestBuildResponsesProbeCases(t *testing.T) {
	cases := buildProbeCases(config{Model: "qwen", WireAPI: "responses", Image: true})
	if len(cases) != 6 {
		t.Fatalf("expected 6 probes, got %d", len(cases))
	}
	for _, probe := range cases {
		if probe.Body["model"] != "qwen" {
			t.Fatalf("model missing from %s", probe.Name)
		}
		if _, exists := probe.Body["messages"]; exists {
			t.Fatalf("responses probe contains chat messages: %s", probe.Name)
		}
	}
}

func TestClassifyResponseLimitsErrorDetail(t *testing.T) {
	response := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       ioNopCloser{strings.NewReader(strings.Repeat("x", 2048))},
	}
	result := classifyResponse(response)
	if result.Status != "fail" || len(result.Detail) != 1024 {
		t.Fatalf("unexpected classification: %#v", result)
	}
}

func TestValidateProbePayloadRejectsFalsePositive2xx(t *testing.T) {
	payload, _ := json.Marshal(chatTextResponse("I cannot call tools"))
	if err := validateProbePayload("tool_calling", "chat", payload); err == nil {
		t.Fatal("expected missing tool call to fail semantic validation")
	}
	payload, _ = json.Marshal(chatTextResponse("not-json"))
	if err := validateProbePayload("json_output", "chat", payload); err == nil {
		t.Fatal("expected invalid JSON output to fail semantic validation")
	}
}

func TestValidateResponsesPayload(t *testing.T) {
	payload := []byte(`{"output":[{"type":"function_call","name":"get_weather"},{"type":"function_call","name":"get_weather"}]}`)
	if err := validateProbePayload("parallel_tool_calls", "responses", payload); err != nil {
		t.Fatal(err)
	}
}

func TestImportReportWritesOnlyVerifiedObservation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "qwen.yaml")
	if err := os.WriteFile(path, []byte("schema_version: 2\nid: qwen/model\nname: Qwen\nprovider: Qwen\ncontext_length: 8192\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	observation := report{
		Model: "served-qwen", WireAPI: "chat", Server: "vllm", ServerVersion: "1.0",
		TestedAt: "2026-07-29T00:00:00Z",
		Results: map[string]result{
			"model_discovery": {Status: "pass"}, "text_generation": {Status: "pass"},
			"tool_calling": {Status: "pass"}, "parallel_tool_calls": {Status: "fail"},
		},
	}
	if err := importReport(config{ModelsDir: dir, ImportModel: "qwen/model", Server: "vllm"}, observation); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "model_slug: served-qwen") || !strings.Contains(text, "parallel_tool_calls: false") {
		t.Fatalf("observation not imported correctly:\n%s", text)
	}
}

type ioNopCloser struct{ *strings.Reader }

func (ioNopCloser) Close() error { return nil }

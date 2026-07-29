package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
				http.Error(w, "parallel unsupported", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{}})
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
	if report.Results["parallel_tool_calls"].Status != "unsupported" {
		t.Fatalf("parallel probe misclassified: %#v", report.Results)
	}
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

type ioNopCloser struct{ *strings.Reader }

func (ioNopCloser) Close() error { return nil }

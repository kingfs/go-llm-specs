package aiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDecodeResponsesIgnoresReasoning(t *testing.T) {
	input := `{"output":[{"type":"reasoning","content":[{"type":"reasoning_text","text":"hidden"}]},{"type":"message","content":[{"type":"output_text","text":"{\"ok\":true}"}]}]}`
	text, err := decodeText(strings.NewReader(input), "responses")
	if err != nil || text != `{"ok":true}` {
		t.Fatalf("text=%q err=%v", text, err)
	}
}

func TestStripFence(t *testing.T) {
	if got := stripFence("```json\n{\"ok\":true}\n```"); got != `{"ok":true}` {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeEscapedJSON(t *testing.T) {
	if got := normalizeJSONText(`{\"claims\":[]}`); got != `{"claims":[]}` {
		t.Fatalf("got %q", got)
	}
	if got := normalizeJSONText(`"{\"claims\":[]}"`); got != `{"claims":[]}` {
		t.Fatalf("got %q", got)
	}
}

func TestJSONStripsLeadingThinkBlocks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"<think>reasoning with {\\\"draft\\\":true}</think>\\n{\\\"ok\\\":true}"}}]}`))
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL, Model: "test", WireAPI: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		OK bool `json:"ok"`
	}
	if err := client.JSON(context.Background(), "return JSON", &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatal("expected parsed JSON after think block")
	}
}

func TestChatRequestIncludesReasoningEffortAndJSONMode(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL, Model: "test", WireAPI: "chat", ReasoningEffort: "none"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Complete(context.Background(), "return JSON"); err != nil {
		t.Fatal(err)
	}
	if request["reasoning_effort"] != "none" {
		t.Fatalf("reasoning_effort=%v", request["reasoning_effort"])
	}
	format, _ := request["response_format"].(map[string]any)
	if format["type"] != "json_object" {
		t.Fatalf("response_format=%v", request["response_format"])
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, time.August, 5, 0, 0, 0, 0, time.UTC)
	tests := map[string]time.Duration{
		"2":      2 * time.Second,
		"2.5":    2500 * time.Millisecond,
		"1500ms": 1500 * time.Millisecond,
		now.Add(3 * time.Second).Format(http.TimeFormat): 3 * time.Second,
	}
	for value, want := range tests {
		if got := parseRetryAfter(value, now); got != want {
			t.Fatalf("parseRetryAfter(%q)=%s, want %s", value, got, want)
		}
	}
}

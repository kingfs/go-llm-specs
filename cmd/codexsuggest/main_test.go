package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseCutoff(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	got, err := parseCutoff("180d", now)
	if err != nil {
		t.Fatal(err)
	}
	want := now.Add(-180 * 24 * time.Hour)
	if !got.Equal(want) {
		t.Fatalf("got %s, want %s", got, want)
	}
	got, err = parseCutoff("2026-01-29", now)
	if err != nil || got.Format("2006-01-02") != "2026-01-29" {
		t.Fatalf("date cutoff: %s, %v", got, err)
	}
	if _, err := parseCutoff("six-months", now); err == nil {
		t.Fatal("expected invalid cutoff error")
	}
}

func TestAllowlistCreatesPendingSuggestionAndReportsSkips(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "models", "acme", "chat.yaml"), `schema_version: 2
id: acme/chat
name: Chat
provider: Acme
context_length: 32000
features: [CapChat, CapFunctionCall, ModalityTextIn, ModalityTextOut]
`)
	write(t, filepath.Join(root, "models", "acme", "embed.yaml"), `schema_version: 2
id: acme/embed
name: Embed
provider: Acme
context_length: 8192
features: [CapEmbedding, ModalityTextIn]
`)
	allowlist := filepath.Join(root, "allowlist.yaml")
	write(t, allowlist, `models:
  - id: acme/chat
    slugs: [served-chat]
  - id: acme/embed
    slugs: [served-embed]
  - id: acme/missing
    slugs: [missing]
`)
	reportPath := filepath.Join(root, "report.json")
	err := run(config{modelsDir: filepath.Join(root, "models"), allowlist: allowlist, outputDir: filepath.Join(root, "suggestions"), report: reportPath, now: time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "suggestions", "acme", "chat.codex.json"))
	if err != nil {
		t.Fatal(err)
	}
	var suggestion map[string]any
	if err := json.Unmarshal(data, &suggestion); err != nil {
		t.Fatal(err)
	}
	if suggestion["status"] != "pending" {
		t.Fatalf("status = %v", suggestion["status"])
	}
	data, err = os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var result report
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Included) != 1 || len(result.Skipped) != 2 {
		t.Fatalf("included=%d skipped=%d", len(result.Included), len(result.Skipped))
	}
}

func TestRecentSelectionRequiresExplicitServingNamespace(t *testing.T) {
	root := t.TempDir()
	cache := filepath.Join(root, "models.json")
	write(t, cache, `{"data":[{"id":"acme/new","created":1785283200},{"id":"acme/old","created":1700000000},{"id":"acme/unknown-date"}]}`)
	cfg := config{since: "2026-01-29", upstreamCache: cache, now: time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)}
	selected, _, err := selectModels(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 1 || len(selected[0].Slugs) != 0 {
		t.Fatalf("selection = %#v", selected)
	}
	cfg.servingProvider = "openrouter"
	selected, _, err = selectModels(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 1 || selected[0].Slugs[0] != "acme/new" {
		t.Fatalf("selection = %#v", selected)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

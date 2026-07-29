package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckLoadsCatalogThroughPinnedCodex(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "models.json")
	if err := os.WriteFile(catalog, []byte(`{"models":[{"slug":"qwen"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "codex")
	script := "#!/bin/sh\nif [ \"$1\" = --version ]; then echo '" + pinnedCodexVersion + "'; else echo '{\"models\":[{\"slug\":\"qwen\"}]}'; fi\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := check(binary, catalog); err != nil {
		t.Fatal(err)
	}
}

func TestCheckRejectsVersionDrift(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "codex")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\necho 'codex-cli 9.9.9'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := check(binary, "unused"); err == nil {
		t.Fatal("expected version mismatch")
	}
}

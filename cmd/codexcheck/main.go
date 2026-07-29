package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const pinnedCodexVersion = "codex-cli 0.146.0"

type modelsResponse struct {
	Models []struct {
		Slug string `json:"slug"`
	} `json:"models"`
}

func main() {
	var binary, catalog string
	flag.StringVar(&binary, "codex-bin", "codex", "Codex CLI binary")
	flag.StringVar(&catalog, "catalog", "dist/codex/models.json", "generated model catalog")
	flag.Parse()
	if err := check(binary, catalog); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func check(binary, catalog string) error {
	versionOutput, err := exec.Command(binary, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("run Codex version check: %w", err)
	}
	if strings.TrimSpace(string(versionOutput)) != pinnedCodexVersion {
		return fmt.Errorf("Codex version mismatch: got %q, want %q", strings.TrimSpace(string(versionOutput)), pinnedCodexVersion)
	}
	want, err := loadSlugs(catalog)
	if err != nil {
		return err
	}
	argument := `model_catalog_json="` + catalog + `"`
	command := exec.Command(binary, "debug", "models", "-c", argument)
	var stdout, stderr strings.Builder
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("Codex rejected catalog: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if strings.Contains(strings.ToLower(stderr.String()), "fallback metadata") {
		return errors.New("Codex loaded catalog with fallback metadata warning: " + strings.TrimSpace(stderr.String()))
	}
	var loaded modelsResponse
	if err := json.Unmarshal([]byte(stdout.String()), &loaded); err != nil {
		return fmt.Errorf("decode Codex debug output: %w", err)
	}
	got := make(map[string]bool, len(loaded.Models))
	for _, model := range loaded.Models {
		got[strings.ToLower(model.Slug)] = true
	}
	for slug := range want {
		if !got[slug] {
			return fmt.Errorf("Codex debug output omitted generated slug %q", slug)
		}
	}
	fmt.Printf("Codex %s loaded %d generated model entries\n", strings.TrimPrefix(pinnedCodexVersion, "codex-cli "), len(want))
	return nil
}

func loadSlugs(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var catalog modelsResponse
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, err
	}
	if len(catalog.Models) == 0 {
		return nil, errors.New("catalog contains no models")
	}
	result := make(map[string]bool, len(catalog.Models))
	for _, model := range catalog.Models {
		if model.Slug == "" {
			return nil, errors.New("catalog entry has empty slug")
		}
		result[strings.ToLower(model.Slug)] = true
	}
	return result, nil
}

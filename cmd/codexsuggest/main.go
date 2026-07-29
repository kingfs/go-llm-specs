package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kingfs/go-llm-specs/internal/registry"
	"github.com/kingfs/go-llm-specs/internal/suggestion"
)

func main() {
	var root, id, output string
	flag.StringVar(&root, "models-dir", "models", "model registry directory")
	flag.StringVar(&id, "model", "", "registry model ID")
	flag.StringVar(&output, "output", "", "suggestion output path")
	flag.Parse()
	if err := run(root, id, output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(root, id, output string) error {
	models, err := registry.Scan(root)
	if err != nil {
		return err
	}
	for _, model := range models {
		if !strings.EqualFold(model.ID, id) {
			continue
		}
		if !has(model.Features, "CapChat") || !has(model.Features, "CapFunctionCall") {
			return fmt.Errorf("%s is not an explicit chat/tool model", model.ID)
		}
		slug := modelSuffix(model.ID)
		modalities := []string{"text"}
		if has(model.Features, "ModalityImageIn") {
			modalities = append(modalities, "image")
		}
		claims := []suggestion.Claim{
			claim("codex.enabled", true, "Derived from explicit CapChat and CapFunctionCall registry capabilities"),
			claim("codex.slugs", []string{slug}, "Candidate serving slug derived from registry model ID; operator must confirm"),
			claim("codex.shell_type", "unified_exec", "Conservative third-party Codex shell default"),
			claim("codex.apply_patch_tool_type", "freeform", "Conservative third-party Codex patch-tool default"),
			claim("codex.supports_parallel_tool_calls", false, "Parallel tool support is not inferred from static metadata"),
			claim("codex.input_modalities", modalities, "Derived from explicit registry modality capabilities"),
		}
		doc := suggestion.Document{SchemaVersion: 1, Kind: "codex_policy", ModelID: model.ID, Status: "pending", CreatedAt: time.Now().UTC(), Source: suggestion.Source{URL: model.FilePath, SHA256: "registry-derived"}, Generator: suggestion.Generator{Model: "deterministic/codexsuggest", WireAPI: "none"}, Claims: claims}
		if output == "" {
			output = filepath.Join("data", "suggestions", filepath.FromSlash(strings.ReplaceAll(model.ID, ":", "_"))+".codex.json")
		}
		if err := suggestion.Save(output, doc); err != nil {
			return err
		}
		fmt.Printf("wrote conservative Codex policy suggestion to %s\n", output)
		return nil
	}
	return fmt.Errorf("model %q not found", id)
}

func claim(field string, value any, evidence string) suggestion.Claim {
	data, _ := json.Marshal(value)
	return suggestion.Claim{Field: field, Value: data, Evidence: evidence, Confidence: "high"}
}
func modelSuffix(id string) string {
	if _, suffix, ok := strings.Cut(id, "/"); ok {
		return suffix
	}
	return id
}
func has(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

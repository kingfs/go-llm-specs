package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kingfs/go-llm-specs/internal/registry"
	"github.com/kingfs/go-llm-specs/internal/suggestion"
)

func main() {
	var root, modelsDir, fields string
	flag.StringVar(&root, "suggestions-dir", "data/suggestions", "suggestion directory")
	flag.StringVar(&modelsDir, "models-dir", "models", "model registry directory")
	flag.StringVar(&fields, "fields", "", "comma-separated fields to apply")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		fatal("command required: list, show, apply, reject")
	}
	var err error
	switch args[0] {
	case "list":
		err = list(root)
	case "show":
		err = requireFile(args, func(path string) error { return show(path) })
	case "reject":
		err = requireFile(args, func(path string) error { return setStatus(path, "rejected") })
	case "apply":
		err = requireFile(args, func(path string) error { return apply(path, modelsDir, fields) })
	default:
		err = fmt.Errorf("unknown command %q", args[0])
	}
	if err != nil {
		fatal(err.Error())
	}
}

func fatal(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }

func requireFile(args []string, action func(string) error) error {
	if len(args) != 2 {
		return fmt.Errorf("command requires one suggestion file")
	}
	return action(args[1])
}

func list(root string) error {
	var rows []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".json") {
			return err
		}
		doc, loadErr := suggestion.Load(path)
		if loadErr != nil {
			return loadErr
		}
		rows = append(rows, fmt.Sprintf("%s\t%s\t%s\t%d\t%s", doc.Status, doc.Kind, doc.ModelID, len(doc.Claims), path))
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(rows)
	for _, row := range rows {
		fmt.Println(row)
	}
	return nil
}

func show(path string) error {
	doc, err := suggestion.Load(path)
	if err != nil {
		return err
	}
	data, _ := json.MarshalIndent(doc, "", "  ")
	fmt.Println(string(data))
	return nil
}

func setStatus(path, status string) error {
	doc, err := suggestion.Load(path)
	if err != nil {
		return err
	}
	doc.Status = status
	return suggestion.Save(path, doc)
}

func apply(path, modelsDir, fields string) error {
	if strings.TrimSpace(fields) == "" {
		return fmt.Errorf("-fields is required; applying all AI claims is not allowed")
	}
	doc, err := suggestion.Load(path)
	if err != nil {
		return err
	}
	wanted := map[string]bool{}
	for _, field := range strings.Split(fields, ",") {
		wanted[strings.TrimSpace(field)] = true
	}
	models, err := registry.Scan(modelsDir)
	if err != nil {
		return err
	}
	for _, model := range models {
		if !strings.EqualFold(model.ID, doc.ModelID) {
			continue
		}
		for _, claim := range doc.Claims {
			if !wanted[claim.Field] {
				continue
			}
			if err := applyClaim(&model, claim); err != nil {
				return err
			}
		}
		if err := registry.Save(model.FilePath, model); err != nil {
			return err
		}
		doc.Status = "accepted"
		return suggestion.Save(path, doc)
	}
	return fmt.Errorf("model %q not found", doc.ModelID)
}

func applyClaim(model *registry.Model, claim suggestion.Claim) error {
	if strings.HasPrefix(claim.Field, "codex.") && model.Codex == nil {
		model.Codex = &registry.CodexMetadata{}
	}
	switch claim.Field {
	case "description":
		return json.Unmarshal(claim.Value, &model.Description)
	case "context_length":
		return json.Unmarshal(claim.Value, &model.ContextLen)
	case "max_output":
		return json.Unmarshal(claim.Value, &model.MaxOutput)
	case "features":
		return json.Unmarshal(claim.Value, &model.Features)
	case "reasoning.supported":
		if model.Reasoning == nil {
			model.Reasoning = &registry.ReasoningMetadata{}
		}
		return json.Unmarshal(claim.Value, &model.Reasoning.Supported)
	case "reasoning.parser":
		if model.Reasoning == nil {
			model.Reasoning = &registry.ReasoningMetadata{}
		}
		return json.Unmarshal(claim.Value, &model.Reasoning.Parser)
	case "codex.enabled":
		return json.Unmarshal(claim.Value, &model.Codex.Enabled)
	case "codex.slugs":
		return json.Unmarshal(claim.Value, &model.Codex.Slugs)
	case "codex.shell_type":
		return json.Unmarshal(claim.Value, &model.Codex.ShellType)
	case "codex.apply_patch_tool_type":
		return json.Unmarshal(claim.Value, &model.Codex.ApplyPatchToolType)
	case "codex.supports_parallel_tool_calls":
		return json.Unmarshal(claim.Value, &model.Codex.SupportsParallelToolCalls)
	case "codex.input_modalities":
		return json.Unmarshal(claim.Value, &model.Codex.InputModalities)
	default:
		return fmt.Errorf("field %q cannot be applied automatically", claim.Field)
	}
}

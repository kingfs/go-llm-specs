package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kingfs/go-llm-specs/internal/registry"
	"github.com/kingfs/go-llm-specs/internal/suggestion"
	"gopkg.in/yaml.v3"
)

type config struct {
	modelsDir, model, slug, allowlist, since, servingProvider, upstreamCache, output, outputDir, report string
	now                                                                                                 time.Time
}

type allowlistDocument struct {
	Models []allowlistModel `yaml:"models"`
}
type allowlistModel struct {
	ID    string   `yaml:"id"`
	Slugs []string `yaml:"slugs"`
}
type upstreamResponse struct {
	Data []upstreamModel `json:"data"`
}
type upstreamModel struct {
	ID      string `json:"id"`
	Created int64  `json:"created"`
}
type selection struct {
	ID     string
	Slugs  []string
	Source string
}
type report struct {
	GeneratedAt time.Time    `json:"generated_at"`
	Since       *time.Time   `json:"since,omitempty"`
	Included    []reportItem `json:"included"`
	Skipped     []reportItem `json:"skipped"`
}
type reportItem struct {
	ID         string   `json:"id"`
	Slugs      []string `json:"slugs,omitempty"`
	Reason     string   `json:"reason,omitempty"`
	Suggestion string   `json:"suggestion,omitempty"`
}

func main() {
	var cfg config
	flag.StringVar(&cfg.modelsDir, "models-dir", "models", "model registry directory")
	flag.StringVar(&cfg.model, "model", "", "single registry model ID")
	flag.StringVar(&cfg.slug, "slug", "", "serving slug for -model (defaults to the model ID suffix for compatibility)")
	flag.StringVar(&cfg.allowlist, "allowlist", "", "YAML file containing registry IDs and explicit serving slugs")
	flag.StringVar(&cfg.since, "since", "", "select candidates created since an RFC3339/date cutoff or duration such as 180d")
	flag.StringVar(&cfg.servingProvider, "serving-provider", "", "serving namespace for recent models; 'openrouter' explicitly uses registry IDs as slugs")
	flag.StringVar(&cfg.upstreamCache, "upstream-cache", "data/models.json", "OpenRouter cache containing created timestamps")
	flag.StringVar(&cfg.output, "output", "", "single-model suggestion output path")
	flag.StringVar(&cfg.outputDir, "output-dir", "data/suggestions", "batch suggestion output directory")
	flag.StringVar(&cfg.report, "report", "", "optional JSON selection report path")
	flag.Parse()
	cfg.now = time.Now().UTC()
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cfg config) error {
	modes := 0
	for _, set := range []bool{cfg.model != "", cfg.allowlist != "", cfg.since != ""} {
		if set {
			modes++
		}
	}
	if modes != 1 {
		return fmt.Errorf("exactly one of -model, -allowlist, or -since is required")
	}
	if cfg.output != "" && cfg.model == "" {
		return fmt.Errorf("-output is only valid with -model; use -output-dir for batch selection")
	}
	if cfg.servingProvider != "" && !strings.EqualFold(cfg.servingProvider, "openrouter") {
		return fmt.Errorf("unsupported -serving-provider %q", cfg.servingProvider)
	}
	models, err := registry.Scan(cfg.modelsDir)
	if err != nil {
		return err
	}
	byID := make(map[string]registry.Model, len(models))
	for _, model := range models {
		byID[strings.ToLower(model.ID)] = model
	}
	selections, cutoff, err := selectModels(cfg)
	if err != nil {
		return err
	}
	result := report{GeneratedAt: cfg.now, Since: cutoff, Included: []reportItem{}, Skipped: []reportItem{}}
	seen := map[string]bool{}
	for _, selected := range selections {
		key := strings.ToLower(selected.ID)
		if seen[key] {
			result.Skipped = append(result.Skipped, reportItem{ID: selected.ID, Reason: "duplicate selection"})
			continue
		}
		seen[key] = true
		model, ok := byID[key]
		if !ok {
			result.Skipped = append(result.Skipped, reportItem{ID: selected.ID, Reason: "model not found in registry"})
			continue
		}
		if reason := ineligibleReason(model); reason != "" {
			result.Skipped = append(result.Skipped, reportItem{ID: model.ID, Reason: reason})
			continue
		}
		if len(selected.Slugs) == 0 {
			result.Skipped = append(result.Skipped, reportItem{ID: model.ID, Reason: "no serving slug; add an explicit slug to an allowlist"})
			continue
		}
		path := cfg.output
		if path == "" {
			path, err = suggestionPath(cfg.outputDir, model.ID)
			if err != nil {
				return err
			}
		}
		if err := writeSuggestion(path, model, selected.Slugs, selected.Source, cfg.now); err != nil {
			return err
		}
		result.Included = append(result.Included, reportItem{ID: model.ID, Slugs: selected.Slugs, Suggestion: path})
		fmt.Printf("wrote conservative Codex policy suggestion to %s\n", path)
	}
	sort.Slice(result.Included, func(i, j int) bool { return result.Included[i].ID < result.Included[j].ID })
	sort.Slice(result.Skipped, func(i, j int) bool { return result.Skipped[i].ID < result.Skipped[j].ID })
	if cfg.report != "" {
		if err := saveJSON(cfg.report, result); err != nil {
			return err
		}
	}
	if len(result.Included) == 0 {
		return fmt.Errorf("no eligible Codex candidates selected (%d skipped)", len(result.Skipped))
	}
	return nil
}

func selectModels(cfg config) ([]selection, *time.Time, error) {
	if cfg.model != "" {
		slug := cfg.slug
		if slug == "" {
			slug = modelSuffix(cfg.model)
		}
		return []selection{{ID: cfg.model, Slugs: []string{slug}, Source: "explicit single-model selection"}}, nil, nil
	}
	if cfg.allowlist != "" {
		data, err := os.ReadFile(cfg.allowlist)
		if err != nil {
			return nil, nil, err
		}
		var doc allowlistDocument
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, nil, fmt.Errorf("parse allowlist: %w", err)
		}
		var selected []selection
		for _, item := range doc.Models {
			selected = append(selected, selection{ID: strings.TrimSpace(item.ID), Slugs: cleanStrings(item.Slugs), Source: cfg.allowlist})
		}
		return selected, nil, nil
	}
	cutoff, err := parseCutoff(cfg.since, cfg.now)
	if err != nil {
		return nil, nil, err
	}
	data, err := os.ReadFile(cfg.upstreamCache)
	if err != nil {
		return nil, nil, err
	}
	var response upstreamResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, nil, fmt.Errorf("parse upstream cache: %w", err)
	}
	var selected []selection
	for _, item := range response.Data {
		if item.Created <= 0 || time.Unix(item.Created, 0).Before(cutoff) {
			continue
		}
		var slugs []string
		if strings.EqualFold(cfg.servingProvider, "openrouter") {
			slugs = []string{item.ID}
		}
		selected = append(selected, selection{ID: item.ID, Slugs: slugs, Source: cfg.upstreamCache})
	}
	return selected, &cutoff, nil
}

func parseCutoff(value string, now time.Time) (time.Time, error) {
	if strings.HasSuffix(value, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(value, "d"))
		if err != nil || days < 0 {
			return time.Time{}, fmt.Errorf("invalid -since %q", value)
		}
		return now.Add(-time.Duration(days) * 24 * time.Hour), nil
	}
	if t, err := time.Parse("2006-01-02", value); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("invalid -since %q: use YYYY-MM-DD, RFC3339, or Nd", value)
}

func ineligibleReason(model registry.Model) string {
	if !model.IsV2() {
		return "schema_version 2 is required"
	}
	if model.ContextLen <= 0 {
		return "positive context_length is required"
	}
	for _, feature := range []string{"CapChat", "CapFunctionCall", "ModalityTextIn", "ModalityTextOut"} {
		if !has(model.Features, feature) {
			return "missing required capability " + feature
		}
	}
	return ""
}

func writeSuggestion(path string, model registry.Model, slugs []string, source string, now time.Time) error {
	modalities := []string{"text"}
	if has(model.Features, "ModalityImageIn") {
		modalities = append(modalities, "image")
	}
	claims := []suggestion.Claim{
		claim("codex.enabled", true, "Candidate passed static Codex eligibility checks; operator approval is still required"),
		claim("codex.slugs", slugs, "Explicit serving slug supplied by operator; must match the API model name"),
		claim("codex.shell_type", "unified_exec", "Conservative third-party Codex shell default"),
		claim("codex.apply_patch_tool_type", "freeform", "Conservative third-party Codex patch-tool default"),
		claim("codex.supports_parallel_tool_calls", false, "Parallel tool support is not inferred from static metadata"),
		claim("codex.input_modalities", modalities, "Derived from explicit registry modality capabilities"),
	}
	doc := suggestion.Document{SchemaVersion: 1, Kind: "codex_policy", ModelID: model.ID, Status: "pending", CreatedAt: now, Source: suggestion.Source{URL: source, SHA256: "registry-derived"}, Generator: suggestion.Generator{Model: "deterministic/codexsuggest", WireAPI: "none"}, Claims: claims}
	return suggestion.Save(path, doc)
}

func saveJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func suggestionPath(root, id string) (string, error) {
	parts := strings.Split(strings.ReplaceAll(id, ":", "_"), "/")
	if len(parts) == 0 {
		return "", fmt.Errorf("invalid empty model ID")
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || filepath.Base(part) != part {
			return "", fmt.Errorf("model ID %q cannot be used as a suggestion path", id)
		}
	}
	parts[len(parts)-1] += ".codex.json"
	return filepath.Join(append([]string{root}, parts...)...), nil
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
func cleanStrings(values []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

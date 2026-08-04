package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"log"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

type snapshotModel struct {
	ID            string
	Name          string
	Provider      string
	Developer     string
	OfficialURL   string
	ModelCardURL  string
	Description   string
	DescriptionCN string
	ContextLen    int
	MaxOutput     int
	Features      []string
	Aliases       []string
}

type diffEntry struct {
	ID            string   `json:"id"`
	ChangedFields []string `json:"changed_fields,omitempty"`
	IgnoredFields []string `json:"ignored_fields,omitempty"`
}

type report struct {
	BaseTag             string      `json:"base_tag"`
	NextTag             string      `json:"next_tag"`
	ReleaseNeeded       bool        `json:"release_needed"`
	HasAnyContentChange bool        `json:"has_any_content_change"`
	AddedModels         []string    `json:"added_models,omitempty"`
	RemovedModels       []string    `json:"removed_models,omitempty"`
	UpdatedModels       []diffEntry `json:"updated_models,omitempty"`
	IgnoredOnlyModels   []diffEntry `json:"ignored_only_models,omitempty"`
	ReleaseReason       string      `json:"release_reason"`
	ReleaseSummary      string      `json:"release_summary"`
	ReleaseBody         string      `json:"release_body"`
}

func main() {
	cfg := struct {
		RepoPath     string
		BaseRef      string
		BaseFile     string
		CurrentFile  string
		Format       string
		GitHubOutput string
	}{
		RepoPath:    ".",
		BaseFile:    "models_gen.go",
		CurrentFile: "models_gen.go",
		Format:      "text",
	}

	flag.StringVar(&cfg.RepoPath, "repo", cfg.RepoPath, "repository root used for git lookups")
	flag.StringVar(&cfg.BaseRef, "base-ref", "", "git ref to compare against; defaults to latest semver tag")
	flag.StringVar(&cfg.BaseFile, "base-file", cfg.BaseFile, "path loaded from base ref")
	flag.StringVar(&cfg.CurrentFile, "current-file", cfg.CurrentFile, "current generated registry file path")
	flag.StringVar(&cfg.Format, "format", cfg.Format, "output format: text or json")
	flag.StringVar(&cfg.GitHubOutput, "github-output", "", "optional GitHub output file path")
	flag.Parse()

	if err := run(cfg); err != nil {
		log.Fatal(err)
	}
}

func run(cfg struct {
	RepoPath     string
	BaseRef      string
	BaseFile     string
	CurrentFile  string
	Format       string
	GitHubOutput string
}) error {
	baseRef := cfg.BaseRef
	if baseRef == "" {
		var err error
		baseRef, err = latestSemverTag(cfg.RepoPath)
		if err != nil {
			return err
		}
	}

	currentModels, err := loadSnapshotFromFile(cfg.CurrentFile)
	if err != nil {
		return fmt.Errorf("load current snapshot from %s: %w", cfg.CurrentFile, err)
	}

	baseModels := map[string]snapshotModel{}
	if baseRef != "" {
		baseContent, err := gitShowFile(cfg.RepoPath, baseRef, cfg.BaseFile)
		if err != nil {
			return fmt.Errorf("load base snapshot from %s:%s: %w", baseRef, cfg.BaseFile, err)
		}
		baseModels, err = parseSnapshot(baseContent)
		if err != nil {
			return fmt.Errorf("parse base snapshot from %s:%s: %w", baseRef, cfg.BaseFile, err)
		}
	}

	rep := compareSnapshots(baseRef, currentModels, baseModels)

	if cfg.GitHubOutput != "" {
		if err := writeGitHubOutput(cfg.GitHubOutput, rep); err != nil {
			return err
		}
	}

	switch cfg.Format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	case "text":
		fmt.Print(rep.ReleaseBody)
		return nil
	default:
		return fmt.Errorf("unsupported format %q", cfg.Format)
	}
}

func latestSemverTag(repo string) (string, error) {
	cmd := exec.Command("git", "tag", "--sort=-v:refname")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	lines := strings.Fields(string(out))
	if len(lines) == 0 {
		return "", nil
	}
	return lines[0], nil
}

func gitShowFile(repo, ref, path string) ([]byte, error) {
	cmd := exec.Command("git", "show", fmt.Sprintf("%s:%s", ref, path))
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("%s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, err
	}
	return out, nil
}

func loadSnapshotFromFile(path string) (map[string]snapshotModel, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseSnapshot(content)
}

func parseSnapshot(content []byte) (map[string]snapshotModel, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "models_gen.go", content, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}

	models := make(map[string]snapshotModel)
	found := false

	ast.Inspect(file, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}

		ident, ok := assign.Lhs[0].(*ast.Ident)
		if !ok || ident.Name != "staticRegistry" {
			return true
		}

		lit, ok := assign.Rhs[0].(*ast.CompositeLit)
		if !ok {
			return true
		}

		parsed, err := parseRegistryComposite(fset, lit)
		if err == nil {
			models = parsed
			found = true
		}
		return true
	})

	if !found {
		return nil, fmt.Errorf("staticRegistry assignment not found")
	}

	return models, nil
}

func parseRegistryComposite(fset *token.FileSet, lit *ast.CompositeLit) (map[string]snapshotModel, error) {
	models := make(map[string]snapshotModel, len(lit.Elts))
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		id, err := parseStringExpr(kv.Key)
		if err != nil {
			return nil, err
		}

		modelLit, ok := kv.Value.(*ast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf("registry value for %s is not a composite literal", id)
		}

		model, err := parseModelComposite(fset, modelLit)
		if err != nil {
			return nil, err
		}
		if model.ID == "" {
			model.ID = id
		}
		model.Features = normalizeList(model.Features)
		model.Aliases = normalizeList(model.Aliases)
		models[model.ID] = model
	}
	return models, nil
}

func parseModelComposite(fset *token.FileSet, lit *ast.CompositeLit) (snapshotModel, error) {
	var model snapshotModel
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}

		switch key.Name {
		case "IDVal":
			v, err := parseStringExpr(kv.Value)
			if err != nil {
				return model, err
			}
			model.ID = v
		case "NameVal":
			v, err := parseStringExpr(kv.Value)
			if err != nil {
				return model, err
			}
			model.Name = v
		case "ProviderVal":
			v, err := parseStringExpr(kv.Value)
			if err != nil {
				return model, err
			}
			model.Provider = v
		case "DeveloperVal":
			v, err := parseStringExpr(kv.Value)
			if err != nil {
				return model, err
			}
			model.Developer = v
		case "OfficialURLVal":
			v, err := parseStringExpr(kv.Value)
			if err != nil {
				return model, err
			}
			model.OfficialURL = v
		case "ModelCardURLVal":
			v, err := parseStringExpr(kv.Value)
			if err != nil {
				return model, err
			}
			model.ModelCardURL = v
		case "DescVal":
			v, err := parseStringExpr(kv.Value)
			if err != nil {
				return model, err
			}
			model.Description = v
		case "DescCNVal":
			v, err := parseStringExpr(kv.Value)
			if err != nil {
				return model, err
			}
			model.DescriptionCN = v
		case "ContextLenVal":
			v, err := parseIntExpr(kv.Value)
			if err != nil {
				return model, err
			}
			model.ContextLen = v
		case "MaxOutputVal":
			v, err := parseIntExpr(kv.Value)
			if err != nil {
				return model, err
			}
			model.MaxOutput = v
		case "FeaturesVal":
			v, err := parseFeatureExpr(fset, kv.Value)
			if err != nil {
				return model, err
			}
			model.Features = v
		case "AliasList":
			v, err := parseStringSliceExpr(kv.Value)
			if err != nil {
				return model, err
			}
			model.Aliases = v
		}
	}
	return model, nil
}

func parseStringExpr(expr ast.Expr) (string, error) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", fmt.Errorf("expected string literal")
	}
	return strconv.Unquote(lit.Value)
}

func parseIntExpr(expr ast.Expr) (int, error) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return 0, fmt.Errorf("expected int literal")
	}
	v, err := strconv.Atoi(lit.Value)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func parseStringSliceExpr(expr ast.Expr) ([]string, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("expected string slice composite literal")
	}

	values := make([]string, 0, len(lit.Elts))
	for _, elt := range lit.Elts {
		value, err := parseStringExpr(elt)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func parseFeatureExpr(fset *token.FileSet, expr ast.Expr) ([]string, error) {
	if ident, ok := expr.(*ast.Ident); ok && ident.Name == "0" {
		return nil, nil
	}

	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, expr); err != nil {
		return nil, err
	}

	raw := strings.TrimSpace(buf.String())
	if raw == "" || raw == "0" {
		return nil, nil
	}

	parts := strings.Split(raw, "|")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "0" {
			continue
		}
		values = append(values, part)
	}
	return values, nil
}

func compareSnapshots(baseTag string, current, base map[string]snapshotModel) report {
	rep := report{
		BaseTag: baseTag,
		NextTag: nextPatchTag(baseTag),
	}

	allIDs := make([]string, 0, len(current)+len(base))
	seen := make(map[string]struct{}, len(current)+len(base))
	for id := range current {
		seen[id] = struct{}{}
		allIDs = append(allIDs, id)
	}
	for id := range base {
		if _, ok := seen[id]; !ok {
			allIDs = append(allIDs, id)
		}
	}
	sort.Strings(allIDs)

	for _, id := range allIDs {
		cur, curOK := current[id]
		old, oldOK := base[id]
		switch {
		case curOK && !oldOK:
			rep.AddedModels = append(rep.AddedModels, id)
		case !curOK && oldOK:
			rep.RemovedModels = append(rep.RemovedModels, id)
		default:
			significant, ignored := diffModel(cur, old)
			if len(significant) > 0 {
				rep.UpdatedModels = append(rep.UpdatedModels, diffEntry{
					ID:            id,
					ChangedFields: significant,
					IgnoredFields: ignored,
				})
			} else if len(ignored) > 0 {
				rep.IgnoredOnlyModels = append(rep.IgnoredOnlyModels, diffEntry{
					ID:            id,
					IgnoredFields: ignored,
				})
			}
		}
	}

	rep.HasAnyContentChange = len(rep.AddedModels) > 0 || len(rep.RemovedModels) > 0 || len(rep.UpdatedModels) > 0 || len(rep.IgnoredOnlyModels) > 0
	rep.ReleaseNeeded = len(rep.AddedModels) > 0 || len(rep.RemovedModels) > 0 || len(rep.UpdatedModels) > 0
	rep.ReleaseReason = buildReleaseReason(rep)
	rep.ReleaseSummary = buildReleaseSummary(rep)
	rep.ReleaseBody = buildReleaseBody(rep)
	return rep
}

func diffModel(cur, old snapshotModel) (significant, ignored []string) {
	if cur.Name != old.Name {
		significant = append(significant, "name")
	}
	if cur.Provider != old.Provider {
		significant = append(significant, "provider")
	}
	if cur.Developer != old.Developer {
		significant = append(significant, "developer")
	}
	if cur.OfficialURL != old.OfficialURL {
		significant = append(significant, "official_url")
	}
	if cur.ModelCardURL != old.ModelCardURL {
		significant = append(significant, "model_card_url")
	}
	if cur.Description != old.Description {
		significant = append(significant, "description")
	}
	if cur.DescriptionCN != old.DescriptionCN {
		significant = append(significant, "description_cn")
	}
	if !equalStringSlice(cur.Features, old.Features) {
		significant = append(significant, "features")
	}
	if !equalStringSlice(cur.Aliases, old.Aliases) {
		significant = append(significant, "aliases")
	}
	if cur.ContextLen != old.ContextLen {
		significant = append(significant, "context_length")
	}
	if cur.MaxOutput != old.MaxOutput {
		significant = append(significant, "max_output")
	}
	return significant, ignored
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func normalizeList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; !ok {
			seen[key] = value
		}
	}
	result := make([]string, 0, len(seen))
	for _, value := range seen {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i]) < strings.ToLower(result[j])
	})
	return result
}

func nextPatchTag(baseTag string) string {
	if baseTag == "" {
		return "v0.1.0"
	}

	version := strings.TrimPrefix(baseTag, "v")
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return baseTag + ".1"
	}

	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return baseTag + ".1"
	}
	parts[2] = strconv.Itoa(patch + 1)
	return "v" + strings.Join(parts, ".")
}

func buildReleaseReason(rep report) string {
	if rep.ReleaseNeeded {
		var parts []string
		if len(rep.AddedModels) > 0 {
			parts = append(parts, fmt.Sprintf("%d new model(s)", len(rep.AddedModels)))
		}
		if len(rep.RemovedModels) > 0 {
			parts = append(parts, fmt.Sprintf("%d removed model(s)", len(rep.RemovedModels)))
		}
		if len(rep.UpdatedModels) > 0 {
			parts = append(parts, fmt.Sprintf("%d significant model update(s)", len(rep.UpdatedModels)))
		}
		return strings.Join(parts, ", ")
	}
	if rep.HasAnyContentChange {
		return "only ignored fields changed"
	}
	return "no model metadata changes"
}

func buildReleaseSummary(rep report) string {
	if rep.ReleaseNeeded {
		return fmt.Sprintf("Release %s from %s: %s.", rep.NextTag, safeTag(rep.BaseTag), rep.ReleaseReason)
	}
	return fmt.Sprintf("No release needed from %s: %s.", safeTag(rep.BaseTag), rep.ReleaseReason)
}

func buildReleaseBody(rep report) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("Base tag: %s", safeTag(rep.BaseTag)))
	lines = append(lines, fmt.Sprintf("Next tag: %s", rep.NextTag))
	lines = append(lines, fmt.Sprintf("Release needed: %t", rep.ReleaseNeeded))
	lines = append(lines, fmt.Sprintf("Reason: %s", rep.ReleaseReason))

	if len(rep.AddedModels) > 0 {
		lines = append(lines, "", "New models:")
		for _, id := range rep.AddedModels {
			lines = append(lines, "- "+id)
		}
	}

	if len(rep.RemovedModels) > 0 {
		lines = append(lines, "", "Removed models:")
		for _, id := range rep.RemovedModels {
			lines = append(lines, "- "+id)
		}
	}

	if len(rep.UpdatedModels) > 0 {
		lines = append(lines, "", "Significant updates:")
		for _, entry := range rep.UpdatedModels {
			lines = append(lines, fmt.Sprintf("- %s (%s)", entry.ID, strings.Join(entry.ChangedFields, ", ")))
		}
	}

	if len(rep.IgnoredOnlyModels) > 0 {
		lines = append(lines, "", "Ignored-only updates:")
		for _, entry := range rep.IgnoredOnlyModels {
			lines = append(lines, fmt.Sprintf("- %s (%s)", entry.ID, strings.Join(entry.IgnoredFields, ", ")))
		}
	}

	return strings.Join(lines, "\n") + "\n"
}

func safeTag(tag string) string {
	if tag == "" {
		return "(none)"
	}
	return tag
}

func writeGitHubOutput(path string, rep report) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	writeMultiline := func(key, value string) error {
		if _, err := fmt.Fprintf(f, "%s<<EOF\n%s\nEOF\n", key, value); err != nil {
			return err
		}
		return nil
	}

	if _, err := fmt.Fprintf(f, "base_tag=%s\n", rep.BaseTag); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(f, "next_tag=%s\n", rep.NextTag); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(f, "release_needed=%t\n", rep.ReleaseNeeded); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(f, "has_any_content_change=%t\n", rep.HasAnyContentChange); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(f, "added_models_count=%d\n", len(rep.AddedModels)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(f, "removed_models_count=%d\n", len(rep.RemovedModels)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(f, "updated_models_count=%d\n", len(rep.UpdatedModels)); err != nil {
		return err
	}
	if err := writeMultiline("release_reason", rep.ReleaseReason); err != nil {
		return err
	}
	if err := writeMultiline("release_summary", rep.ReleaseSummary); err != nil {
		return err
	}
	if err := writeMultiline("release_body", rep.ReleaseBody); err != nil {
		return err
	}

	addedJSON, _ := json.Marshal(rep.AddedModels)
	if _, err := fmt.Fprintf(f, "added_models_json=%s\n", string(addedJSON)); err != nil {
		return err
	}
	return nil
}

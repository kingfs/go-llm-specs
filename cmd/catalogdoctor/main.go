package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/kingfs/go-llm-specs/internal/provider"
	"github.com/kingfs/go-llm-specs/internal/registry"
)

// catalogDoctorSchemaVersion is bumped whenever the report shape changes.
const catalogDoctorSchemaVersion = 1

// quantizationSuffix flags records that repackage an existing checkpoint at a
// different precision. They are reported for review, not excluded.
var quantizationSuffix = regexp.MustCompile(`(?i)-(bf16|fp8|fp4|nvfp4|int4|int8|awq|gptq|qad|gguf|mlx)$`)

// baseSuffix flags pretrained checkpoints that may shadow an instruct sibling.
var baseSuffix = regexp.MustCompile(`(?i)-base$`)

type doctorReport struct {
	SchemaVersion int                 `json:"schema_version"`
	Summary       doctorSummary       `json:"summary"`
	ByKind        map[string]int      `json:"by_kind"`
	RoutingAlias  []string            `json:"routing_aliases"`
	DraftHeads    []string            `json:"draft_heads"`
	Quantization  []string            `json:"quantization_candidates"`
	BaseVariants  []string            `json:"base_variants"`
	IdentityGaps  []doctorIdentityGap `json:"identity_gaps"`
	Aggregators   []string            `json:"aggregator_providers"`
}

type doctorSummary struct {
	Models       int `json:"models"`
	Compiled     int `json:"compiled"`
	Candidates   int `json:"candidates"`
	Serving      int `json:"serving_variants"`
	Quantization int `json:"quantization_candidates"`
	IdentityGaps int `json:"identity_gaps"`
}

type doctorIdentityGap struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
}

func main() {
	providersDir := flag.String("providers-dir", "providers", "publisher catalog directory")
	modelsDir := flag.String("models-dir", "models", "model registry directory")
	output := flag.String("output", "data/catalog-doctor.json", "report output path")
	check := flag.Bool("check", false, "verify the committed report is current instead of writing it")
	flag.Parse()
	if err := run(*providersDir, *modelsDir, *output, *check); err != nil {
		log.Fatal(err)
	}
}

func run(providersDir, modelsDir, output string, check bool) error {
	providers, err := provider.Scan(providersDir)
	if err != nil {
		return err
	}
	models, err := registry.Scan(modelsDir)
	if err != nil {
		return err
	}
	report := buildReport(providers, models)
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if check {
		existing, err := os.ReadFile(output)
		if err != nil {
			return err
		}
		if !bytes.Equal(existing, data) {
			return fmt.Errorf("%s is stale; run task catalog-doctor", output)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(output, data, 0o644); err != nil {
		return err
	}
	log.Printf("catalog doctor: models=%d serving=%d quantization=%d identity_gaps=%d", report.Summary.Models, report.Summary.Serving, report.Summary.Quantization, report.Summary.IdentityGaps)
	return nil
}

func buildReport(providers []provider.Provider, models []registry.Model) doctorReport {
	byID := make(map[string]provider.Provider, len(providers))
	for _, p := range providers {
		byID[p.ID] = p
	}
	report := doctorReport{
		SchemaVersion: catalogDoctorSchemaVersion,
		ByKind:        map[string]int{},
	}
	for _, p := range providers {
		if !p.HasAuthoritativeSource() {
			report.Aggregators = append(report.Aggregators, p.ID)
		}
	}
	for _, m := range models {
		report.Summary.Models++
		kind := registry.ClassifyKind(m)
		report.ByKind[kind]++
		switch {
		case registry.IsRoutingAlias(m.ID):
			report.RoutingAlias = append(report.RoutingAlias, m.ID)
		case registry.IsDraftHead(m):
			report.DraftHeads = append(report.DraftHeads, m.ID)
		}
		if registry.IsServingKind(kind) {
			report.Summary.Serving++
		} else if m.Lifecycle == "" || m.Lifecycle == "active" {
			report.Summary.Compiled++
		} else {
			report.Summary.Candidates++
		}
		if quantizationSuffix.MatchString(m.ID) {
			report.Quantization = append(report.Quantization, m.ID)
			report.Summary.Quantization++
		}
		if baseSuffix.MatchString(m.ID) {
			report.BaseVariants = append(report.BaseVariants, m.ID)
		}
		p, ok := providerForModel(m, byID)
		if !ok || len(p.Organizations.HuggingFace) == 0 {
			continue
		}
		if !hasHuggingFaceIdentity(m) {
			report.IdentityGaps = append(report.IdentityGaps, doctorIdentityGap{ID: m.ID, Provider: p.ID})
		}
	}
	report.Summary.IdentityGaps = len(report.IdentityGaps)
	sort.Strings(report.RoutingAlias)
	sort.Strings(report.DraftHeads)
	sort.Strings(report.Quantization)
	sort.Strings(report.BaseVariants)
	sort.Strings(report.Aggregators)
	sort.Slice(report.IdentityGaps, func(i, j int) bool { return report.IdentityGaps[i].ID < report.IdentityGaps[j].ID })
	return report
}

func hasHuggingFaceIdentity(m registry.Model) bool {
	if len(m.Identifiers.HuggingFace) > 0 {
		return true
	}
	return m.Upstream.HuggingFace != nil
}

// providerForModel resolves the publisher record a model belongs to. Model IDs
// do not always match the provider directory, so developer, directory and ID
// prefix are tried in order.
func providerForModel(m registry.Model, byID map[string]provider.Provider) (provider.Provider, bool) {
	candidates := []string{
		normalizeKey(m.Developer),
		normalizeKey(filepath.Base(filepath.Dir(m.FilePath))),
		normalizeKey(strings.SplitN(m.ID, "/", 2)[0]),
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		for id, p := range byID {
			if normalizeKey(id) == candidate || normalizeKey(p.Name) == candidate {
				return p, true
			}
		}
	}
	return provider.Provider{}, false
}

func normalizeKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(" ", "", "-", "", "_", "", ".", "").Replace(value)
	return value
}

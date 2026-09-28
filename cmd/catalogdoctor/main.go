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

	"github.com/kingfs/go-llm-specs/internal/identity"
	"github.com/kingfs/go-llm-specs/internal/provider"
	"github.com/kingfs/go-llm-specs/internal/registry"
)

// catalogDoctorSchemaVersion is bumped whenever the report shape changes.
const catalogDoctorSchemaVersion = 3

// baseSuffix flags pretrained checkpoints that may shadow an instruct sibling.
var baseSuffix = regexp.MustCompile(`(?i)-base$`)

type doctorReport struct {
	SchemaVersion int                   `json:"schema_version"`
	Summary       doctorSummary         `json:"summary"`
	ByKind        map[string]int        `json:"by_kind"`
	Identity      doctorIdentitySummary `json:"identity"`
	RoutingAlias  []string              `json:"routing_aliases"`
	DraftHeads    []string              `json:"draft_heads"`
	OutOfScope    []doctorScopeRef      `json:"out_of_scope"`
	Quantization  []doctorScopeRef      `json:"quantization_variants"`
	BaseVariants  []string              `json:"base_variants"`
	IdentityGaps  []doctorIdentityGap   `json:"identity_gaps"`
	Aggregators   []string              `json:"aggregator_providers"`
}

type doctorSummary struct {
	Models       int `json:"models"`
	Compiled     int `json:"compiled"`
	Candidates   int `json:"candidates"`
	Serving      int `json:"serving_variants"`
	OutOfScope   int `json:"out_of_scope"`
	Quantization int `json:"quantization_variants"`
	IdentityGaps int `json:"identity_gaps"`
	Unverified   int `json:"unverified_identities"`
}

// doctorScopeRef records a record the catalog does not collect, with the reason
// the scope classifier reached that decision.
type doctorScopeRef struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

type doctorIdentitySummary struct {
	BySource   map[string]int      `json:"by_source"`
	Unverified []doctorIdentityRef `json:"unverified"`
}

type doctorIdentityRef struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Source   string `json:"source"`
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
	log.Printf("catalog doctor: models=%d serving=%d quantization=%d out_of_scope=%d identity_gaps=%d", report.Summary.Models, report.Summary.Serving, report.Summary.Quantization, report.Summary.OutOfScope, report.Summary.IdentityGaps)
	return nil
}

func buildReport(providers []provider.Provider, models []registry.Model) doctorReport {
	report := doctorReport{
		SchemaVersion: catalogDoctorSchemaVersion,
		ByKind:        map[string]int{},
		Identity:      doctorIdentitySummary{BySource: map[string]int{}},
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
		switch {
		case registry.IsServingKind(kind):
			report.Summary.Serving++
		case kind == registry.KindQuantization:
			// A quantization variant shares the model card of the checkpoint it
			// was derived from, so it is neither compiled nor counted as a model
			// of its own.
			report.Summary.Quantization++
			report.Quantization = append(report.Quantization, doctorScopeRef{ID: m.ID, Reason: registry.QuantizationFormat(m)})
		case kind == registry.KindOutOfScope:
			// Out-of-scope records stay in models/ for provenance but are never
			// compiled or published, so they are counted separately from both
			// candidates and compiled models.
			report.Summary.OutOfScope++
			report.OutOfScope = append(report.OutOfScope, doctorScopeRef{ID: m.ID, Reason: registry.ScopeReason(m)})
		case m.Lifecycle == "" || m.Lifecycle == "active":
			report.Summary.Compiled++
		default:
			report.Summary.Candidates++
		}
		if baseSuffix.MatchString(m.ID) {
			report.BaseVariants = append(report.BaseVariants, m.ID)
		}
		p, ok := identity.ProviderFor(m, providers)
		resolved := identity.Resolved{Source: identity.SourceOpenRouter, Verified: true}
		if ok {
			resolved = identity.Resolve(m, p)
		}
		report.Identity.BySource[resolved.Source]++
		if !resolved.Verified {
			providerID := ""
			if ok {
				providerID = p.ID
			}
			report.Identity.Unverified = append(report.Identity.Unverified, doctorIdentityRef{
				ID: m.ID, Provider: providerID, Source: resolved.Source,
			})
			report.Summary.Unverified++
		}
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
	sort.Slice(report.Quantization, func(i, j int) bool { return report.Quantization[i].ID < report.Quantization[j].ID })
	sort.Strings(report.BaseVariants)
	sort.Strings(report.Aggregators)
	sort.Slice(report.OutOfScope, func(i, j int) bool { return report.OutOfScope[i].ID < report.OutOfScope[j].ID })
	sort.Slice(report.IdentityGaps, func(i, j int) bool { return report.IdentityGaps[i].ID < report.IdentityGaps[j].ID })
	sort.Slice(report.Identity.Unverified, func(i, j int) bool {
		return report.Identity.Unverified[i].ID < report.Identity.Unverified[j].ID
	})
	return report
}

func hasHuggingFaceIdentity(m registry.Model) bool {
	if len(m.Identifiers.HuggingFace) > 0 {
		return true
	}
	return m.Upstream.HuggingFace != nil
}

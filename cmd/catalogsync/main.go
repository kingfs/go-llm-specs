// catalogsync audits publisher attribution and discovers new repositories from
// explicitly subscribed official Hugging Face organizations. It never removes
// model records: this repository is an append-only historical catalog.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kingfs/go-llm-specs/internal/provider"
	"github.com/kingfs/go-llm-specs/internal/registry"
)

type config struct {
	ProvidersDir   string
	ModelsDir      string
	HuggingFaceAPI string
	Output         string
	DiscoverHF     bool
	ApplyMatches   bool
	Materialize    bool
	PromoteReady   bool
	Limit          int
	Check          bool
	Since          time.Duration
	Timeout        time.Duration
}

type report struct {
	Providers               int               `json:"providers"`
	Models                  int               `json:"models"`
	ModelsWithOfficialLinks int               `json:"models_with_official_links"`
	UncatalogedPublishers   map[string]int    `json:"uncataloged_publishers,omitempty"`
	MissingDeveloper        []string          `json:"missing_developer,omitempty"`
	HuggingFaceCandidates   []hfCandidate     `json:"huggingface_candidates,omitempty"`
	DiscoveryErrors         map[string]string `json:"discovery_errors,omitempty"`
}

type hfCandidate struct {
	ProviderID   string    `json:"provider_id"`
	Organization string    `json:"organization"`
	RepositoryID string    `json:"repository_id"`
	RegistryID   string    `json:"registry_id,omitempty"`
	Status       string    `json:"status"`
	URL          string    `json:"url"`
	LastModified time.Time `json:"last_modified,omitempty"`
	PipelineTag  string    `json:"pipeline_tag,omitempty"`
	Tags         []string  `json:"tags,omitempty"`
}

type hfModel struct {
	ID           string    `json:"id"`
	ModelID      string    `json:"modelId"`
	LastModified time.Time `json:"lastModified"`
	PipelineTag  string    `json:"pipeline_tag"`
	Tags         []string  `json:"tags"`
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.ProvidersDir, "providers-dir", "providers", "publisher catalog directory")
	flag.StringVar(&cfg.ModelsDir, "models-dir", "models", "model registry directory")
	flag.StringVar(&cfg.HuggingFaceAPI, "huggingface-api", "https://huggingface.co/api", "Hugging Face API base URL")
	flag.StringVar(&cfg.Output, "output", "data/catalog-report.json", "audit and discovery report")
	flag.BoolVar(&cfg.DiscoverHF, "discover-huggingface", false, "query subscribed official Hugging Face organizations")
	flag.BoolVar(&cfg.ApplyMatches, "apply-identity-matches", false, "write exact unique official Hugging Face identity matches to existing model YAML")
	flag.BoolVar(&cfg.Materialize, "materialize-candidates", false, "create bounded lifecycle=candidate YAML records from eligible official repositories")
	flag.BoolVar(&cfg.PromoteReady, "promote-ready", false, "activate candidate records that now contain required model facts")
	flag.IntVar(&cfg.Limit, "limit", 5, "maximum new candidate YAML records to materialize")
	flag.BoolVar(&cfg.Check, "check", false, "verify the existing report matches current local catalog state")
	flag.DurationVar(&cfg.Since, "since", 30*24*time.Hour, "only report repositories modified within this window; zero scans the full API result")
	flag.DurationVar(&cfg.Timeout, "timeout", 30*time.Second, "HTTP timeout")
	flag.Parse()
	if err := run(context.Background(), cfg); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, cfg config) error {
	providers, err := provider.Scan(cfg.ProvidersDir)
	if err != nil {
		return err
	}
	models, err := registry.Scan(cfg.ModelsDir)
	if err != nil {
		return err
	}
	if cfg.PromoteReady {
		for i := range models {
			if models[i].Lifecycle == "candidate" && readyForPromotion(models[i]) {
				models[i].Lifecycle = "active"
				if err := registry.Save(models[i].FilePath, models[i]); err != nil {
					return err
				}
			}
		}
	}

	byName := make(map[string]string)
	for _, p := range providers {
		byName[normalize(p.ID)] = p.ID
		byName[normalize(p.Name)] = p.ID
		for _, alias := range p.Aliases {
			byName[normalize(alias)] = p.ID
		}
	}

	r := report{
		Providers: len(providers), Models: len(models),
		UncatalogedPublishers: make(map[string]int), DiscoveryErrors: make(map[string]string),
	}
	previousCandidates := make(map[string]hfCandidate)
	if cfg.DiscoverHF {
		if previous, loadErr := loadReport(cfg.Output); loadErr == nil {
			for _, candidate := range previous.HuggingFaceCandidates {
				previousCandidates[candidateKey(candidate.Organization, candidate.RepositoryID)] = candidate
			}
		} else if !os.IsNotExist(loadErr) {
			return fmt.Errorf("load previous discovery state: %w", loadErr)
		}
	}
	knownHF := make(map[string]bool)
	modelMatches := make(map[string][]int)
	for i, m := range models {
		if !m.Links.IsZero() {
			r.ModelsWithOfficialLinks++
		}
		developer := m.Developer
		if developer == "" {
			developer = m.Provider
			r.MissingDeveloper = append(r.MissingDeveloper, m.ID)
		}
		providerID, cataloged := byName[normalize(developer)]
		if !cataloged {
			r.UncatalogedPublishers[developer]++
		}
		if cataloged {
			modelMatches[providerID+":"+normalize(modelSuffix(m.ID))] = append(modelMatches[providerID+":"+normalize(modelSuffix(m.ID))], i)
			if cfg.ApplyMatches && models[i].Developer != providerID {
				models[i].Developer = providerID
				if err := registry.Save(models[i].FilePath, models[i]); err != nil {
					return err
				}
			}
		} else if cfg.ApplyMatches && models[i].Developer == "" {
			models[i].Developer = strings.TrimPrefix(strings.Split(models[i].ID, "/")[0], "~")
			if err := registry.Save(models[i].FilePath, models[i]); err != nil {
				return err
			}
		}
		for _, id := range m.Identifiers.HuggingFace {
			knownHF[strings.ToLower(id)] = true
		}
		if m.Upstream.HuggingFace != nil {
			knownHF[strings.ToLower(m.Upstream.HuggingFace.ID)] = true
			if cfg.ApplyMatches && m.Upstream.HuggingFace.ID != "" {
				model := &models[i]
				model.Identifiers.HuggingFace = appendUnique(model.Identifiers.HuggingFace, m.Upstream.HuggingFace.ID)
				if model.Links.ModelCard == "" {
					model.Links.ModelCard = "https://huggingface.co/" + m.Upstream.HuggingFace.ID
				}
				if err := registry.Save(model.FilePath, *model); err != nil {
					return err
				}
			}
		}
	}

	if cfg.DiscoverHF {
		client := &http.Client{Timeout: cfg.Timeout}
		attempted, succeeded := 0, 0
		cutoff := time.Time{}
		if cfg.Since > 0 {
			cutoff = time.Now().UTC().Add(-cfg.Since)
		}
		for _, p := range providers {
			if !p.Discovery.HuggingFace {
				continue
			}
			for _, org := range p.Organizations.HuggingFace {
				attempted++
				items, err := discoverHF(ctx, client, cfg.HuggingFaceAPI, org, cutoff)
				if err != nil {
					r.DiscoveryErrors[p.ID+":"+org] = err.Error()
					continue
				}
				succeeded++
				for _, item := range items {
					id := item.ID
					if id == "" {
						id = item.ModelID
					}
					if id == "" || knownHF[strings.ToLower(id)] || (!cutoff.IsZero() && item.LastModified.Before(cutoff)) {
						continue
					}
					matches := modelMatches[p.ID+":"+normalize(modelSuffix(id))]
					status := "new"
					registryID := ""
					if len(matches) == 1 {
						status = "identity_match"
						registryID = models[matches[0]].ID
						if cfg.ApplyMatches {
							model := &models[matches[0]]
							model.Identifiers.HuggingFace = appendUnique(model.Identifiers.HuggingFace, id)
							if model.Links.ModelCard == "" {
								model.Links.ModelCard = "https://huggingface.co/" + id
							}
							if err := registry.Save(model.FilePath, *model); err != nil {
								return err
							}
							knownHF[strings.ToLower(id)] = true
							status = "identity_applied"
						}
					}
					candidate := hfCandidate{
						ProviderID: p.ID, Organization: org, RepositoryID: id,
						RegistryID: registryID, Status: status,
						URL: "https://huggingface.co/" + id, LastModified: item.LastModified,
						PipelineTag: item.PipelineTag, Tags: item.Tags,
					}
					key := candidateKey(org, id)
					if previous, ok := previousCandidates[key]; ok && previous.Status != "new" && previous.Status != "identity_match" {
						candidate.Status, candidate.RegistryID = previous.Status, previous.RegistryID
					}
					previousCandidates[key] = candidate
				}
			}
		}
		if attempted > 0 && succeeded == 0 {
			return fmt.Errorf("all %d Hugging Face organization queries failed", attempted)
		}
		for key, candidate := range previousCandidates {
			if knownHF[strings.ToLower(candidate.RepositoryID)] && candidate.Status == "new" {
				candidate.Status = "registered"
				previousCandidates[key] = candidate
			}
		}
		for _, candidate := range previousCandidates {
			r.HuggingFaceCandidates = append(r.HuggingFaceCandidates, candidate)
		}
		if cfg.Materialize {
			if err := materializeCandidates(&r, providers, models, cfg); err != nil {
				return err
			}
		}
	}

	sort.Strings(r.MissingDeveloper)
	sort.Slice(r.HuggingFaceCandidates, func(i, j int) bool {
		return r.HuggingFaceCandidates[i].RepositoryID < r.HuggingFaceCandidates[j].RepositoryID
	})
	if len(r.UncatalogedPublishers) == 0 {
		r.UncatalogedPublishers = nil
	}
	if len(r.DiscoveryErrors) == 0 {
		r.DiscoveryErrors = nil
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if cfg.Check {
		existing, err := os.ReadFile(cfg.Output)
		if err != nil {
			return err
		}
		if !bytes.Equal(existing, data) {
			return fmt.Errorf("%s is stale; run task catalog-audit", cfg.Output)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Output), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(cfg.Output, data, 0o644); err != nil {
		return err
	}
	log.Printf("catalog report: providers=%d models=%d uncataloged=%d hf_candidates=%d", r.Providers, r.Models, len(r.UncatalogedPublishers), len(r.HuggingFaceCandidates))
	return nil
}

func materializeCandidates(r *report, providers []provider.Provider, models []registry.Model, cfg config) error {
	providerByID := make(map[string]provider.Provider, len(providers))
	known := make(map[string]bool, len(models))
	for _, p := range providers {
		providerByID[p.ID] = p
	}
	for _, model := range models {
		known[strings.ToLower(model.ID)] = true
	}
	sort.SliceStable(r.HuggingFaceCandidates, func(i, j int) bool {
		return r.HuggingFaceCandidates[i].LastModified.After(r.HuggingFaceCandidates[j].LastModified)
	})
	written := 0
	for i := range r.HuggingFaceCandidates {
		candidate := &r.HuggingFaceCandidates[i]
		if candidate.Status != "new" || !eligiblePipeline(candidate.PipelineTag, candidate.Tags) {
			continue
		}
		p, ok := providerByID[candidate.ProviderID]
		if !ok {
			continue
		}
		id := p.ID + "/" + strings.ToLower(modelSuffix(candidate.RepositoryID))
		if known[strings.ToLower(id)] {
			candidate.Status = "registered"
			continue
		}
		model := registry.Model{
			SchemaVersion: registry.CurrentSchemaVersion, ID: id, Name: modelSuffix(candidate.RepositoryID),
			Provider: p.Name, Developer: p.ID, Lifecycle: "candidate",
			Features:    featuresForPipeline(candidate.PipelineTag),
			Links:       registry.ModelLinks{ModelCard: candidate.URL},
			Identifiers: registry.ModelIdentifiers{HuggingFace: []string{candidate.RepositoryID}},
			Provenance: map[string]registry.Provenance{
				"id":       {Source: "official_huggingface", URL: candidate.URL},
				"name":     {Source: "official_huggingface", URL: candidate.URL},
				"features": {Source: "official_huggingface_pipeline", URL: candidate.URL},
			},
		}
		path := filepath.Join(cfg.ModelsDir, p.ID, safeFilename(modelSuffix(candidate.RepositoryID))+".yaml")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := registry.Save(path, model); err != nil {
			return err
		}
		candidate.Status, candidate.RegistryID = "materialized", id
		known[strings.ToLower(id)] = true
		written++
		if cfg.Limit > 0 && written >= cfg.Limit {
			break
		}
	}
	sort.Slice(r.HuggingFaceCandidates, func(i, j int) bool {
		return r.HuggingFaceCandidates[i].RepositoryID < r.HuggingFaceCandidates[j].RepositoryID
	})
	return nil
}

func eligiblePipeline(pipeline string, tags []string) bool {
	switch pipeline {
	case "text-generation", "text2text-generation", "image-text-to-text", "visual-question-answering", "feature-extraction":
	default:
		return false
	}
	for _, tag := range tags {
		tag = strings.ToLower(tag)
		if strings.Contains(tag, "gguf") || strings.Contains(tag, "adapter") || strings.Contains(tag, "merge") || strings.Contains(tag, "quantized") {
			return false
		}
	}
	return true
}

func featuresForPipeline(pipeline string) []string {
	switch pipeline {
	case "feature-extraction":
		return []string{"CapEmbedding", "ModalityTextIn"}
	case "image-text-to-text", "visual-question-answering":
		return []string{"ModalityImageIn", "ModalityTextIn", "ModalityTextOut"}
	default:
		return []string{"ModalityTextIn", "ModalityTextOut"}
	}
}

func readyForPromotion(model registry.Model) bool {
	return model.ContextLen > 0 && strings.TrimSpace(model.Description) != "" && len(model.Features) > 0 && model.Upstream.HuggingFace != nil
}

func safeFilename(value string) string {
	return strings.NewReplacer(":", "_", "/", "_").Replace(value)
}

func discoverHF(ctx context.Context, client *http.Client, baseURL, organization string, cutoff time.Time) ([]hfModel, error) {
	values := url.Values{"author": {organization}, "sort": {"lastModified"}, "direction": {"-1"}, "limit": {"100"}, "full": {"true"}}
	next := strings.TrimRight(baseURL, "/") + "/models?" + values.Encode()
	var models []hfModel
	for page := 0; next != ""; page++ {
		if page >= 100 {
			return nil, fmt.Errorf("Hugging Face pagination exceeded 100 pages")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("Hugging Face returned %s", resp.Status)
		}
		var pageModels []hfModel
		err = json.NewDecoder(resp.Body).Decode(&pageModels)
		link := resp.Header.Get("Link")
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		models = append(models, pageModels...)
		next = nextLink(link)
		if !cutoff.IsZero() && len(pageModels) > 0 && pageModels[len(pageModels)-1].LastModified.Before(cutoff) {
			next = ""
		}
	}
	return models, nil
}

func nextLink(header string) string {
	for _, part := range strings.Split(header, ",") {
		pieces := strings.Split(strings.TrimSpace(part), ";")
		if len(pieces) < 2 || !strings.Contains(strings.Join(pieces[1:], ";"), `rel="next"`) {
			continue
		}
		return strings.Trim(strings.TrimSpace(pieces[0]), "<>")
	}
	return ""
}

func loadReport(path string) (report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return report{}, err
	}
	var r report
	if err := json.Unmarshal(data, &r); err != nil {
		return report{}, err
	}
	return r, nil
}

func candidateKey(organization, repositoryID string) string {
	return strings.ToLower(organization + "/" + repositoryID)
}

func normalize(value string) string {
	value = strings.ToLower(value)
	return strings.NewReplacer(" ", "", "-", "", "_", "", ".", "").Replace(value)
}

func modelSuffix(id string) string {
	parts := strings.Split(id, "/")
	return parts[len(parts)-1]
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if strings.EqualFold(existing, value) {
			return values
		}
	}
	return append(values, value)
}

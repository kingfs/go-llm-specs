// officialsync corroborates model identity from first-party model-list APIs.
//
// It is deliberately narrow: it records the official identifier (and an
// official link when the publisher configures a template) and nothing else.
// Attribute enrichment stays with the existing sources. Providers whose API key
// is not configured are reported as skipped rather than failing, so the command
// is safe to run without credentials.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kingfs/go-llm-specs/internal/identity"
	"github.com/kingfs/go-llm-specs/internal/provider"
	"github.com/kingfs/go-llm-specs/internal/registry"
)

const officialsyncSchemaVersion = 1

// minPrefixMatch is the shortest normalized key allowed to match a longer
// official identifier, so short fragments cannot claim unrelated models.
const minPrefixMatch = 6

type config struct {
	ProvidersDir string
	ModelsDir    string
	Output       string
	Timeout      time.Duration
	Apply        bool
}

type report struct {
	SchemaVersion int              `json:"schema_version"`
	Providers     []providerReport `json:"providers"`
}

type providerReport struct {
	Provider          string   `json:"provider"`
	Status            string   `json:"status"`
	Error             string   `json:"error,omitempty"`
	OfficialModels    int      `json:"official_models"`
	Matched           int      `json:"matched"`
	Applied           int      `json:"applied"`
	Ambiguous         []string `json:"ambiguous_official,omitempty"`
	UnmatchedOfficial []string `json:"unmatched_official,omitempty"`
	UnmatchedRegistry []string `json:"unmatched_registry,omitempty"`
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.ProvidersDir, "providers-dir", "providers", "publisher catalog directory")
	flag.StringVar(&cfg.ModelsDir, "models-dir", "models", "model registry directory")
	flag.StringVar(&cfg.Output, "output", "data/official-identity.json", "identity report output path")
	flag.DurationVar(&cfg.Timeout, "timeout", 30*time.Second, "HTTP timeout")
	flag.BoolVar(&cfg.Apply, "apply", false, "write matched official identifiers to model YAML")
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
	client := &http.Client{Timeout: cfg.Timeout}
	r := report{SchemaVersion: officialsyncSchemaVersion}
	for _, p := range providers {
		api := p.Identity.OfficialAPI
		if api == nil {
			continue
		}
		pr := providerReport{Provider: p.ID}
		if api.Env != "" && strings.TrimSpace(os.Getenv(api.Env)) == "" {
			pr.Status = "skipped_no_credentials"
			r.Providers = append(r.Providers, pr)
			continue
		}
		officialIDs, err := fetchOfficialIDs(ctx, client, *api)
		if err != nil {
			pr.Status = "error"
			pr.Error = err.Error()
			r.Providers = append(r.Providers, pr)
			continue
		}
		pr.Status = "ok"
		pr.OfficialModels = len(officialIDs)

		index := buildIndex(models, providers, p)
		matched := make(map[int]bool)
		for _, officialID := range officialIDs {
			position, ambiguous := index.lookup(officialID)
			if ambiguous {
				pr.Ambiguous = append(pr.Ambiguous, officialID)
				continue
			}
			if position < 0 {
				pr.UnmatchedOfficial = append(pr.UnmatchedOfficial, officialID)
				continue
			}
			matched[position] = true
			pr.Matched++
			if !cfg.Apply {
				continue
			}
			if applyIdentity(&models[position], officialID, *api) {
				if err := registry.Save(models[position].FilePath, models[position]); err != nil {
					return fmt.Errorf("save %s: %w", models[position].ID, err)
				}
				pr.Applied++
			}
		}
		for position := range models {
			publisher, ok := identity.ProviderFor(models[position], providers)
			if !ok || publisher.ID != p.ID || matched[position] {
				continue
			}
			pr.UnmatchedRegistry = append(pr.UnmatchedRegistry, models[position].ID)
		}
		sort.Strings(pr.Ambiguous)
		sort.Strings(pr.UnmatchedOfficial)
		sort.Strings(pr.UnmatchedRegistry)
		r.Providers = append(r.Providers, pr)
	}
	sort.Slice(r.Providers, func(i, j int) bool { return r.Providers[i].Provider < r.Providers[j].Provider })

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(cfg.Output), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(cfg.Output, data, 0o644); err != nil {
		return err
	}
	applied, matched := 0, 0
	for _, pr := range r.Providers {
		applied += pr.Applied
		matched += pr.Matched
	}
	log.Printf("official identity: providers=%d matched=%d applied=%d", len(r.Providers), matched, applied)
	return nil
}

// applyIdentity records the official identifier and, when the publisher
// provides a link template, the official link. It reports whether anything
// changed so unchanged records are not rewritten.
func applyIdentity(model *registry.Model, officialID string, api provider.OfficialAPI) bool {
	changed := false
	if !containsFold(model.Identifiers.Official, officialID) {
		model.Identifiers.Official = append(model.Identifiers.Official, officialID)
		sort.Strings(model.Identifiers.Official)
		changed = true
	}
	if model.Links.Official == "" && strings.TrimSpace(api.LinkTemplate) != "" {
		model.Links.Official = strings.ReplaceAll(api.LinkTemplate, "{id}", officialID)
		changed = true
	}
	return changed
}

func fetchOfficialIDs(ctx context.Context, client *http.Client, api provider.OfficialAPI) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api.URL, nil)
	if err != nil {
		return nil, err
	}
	switch api.EffectiveAuth() {
	case provider.OfficialAPIAuthBearer:
		req.Header.Set("Authorization", "Bearer "+os.Getenv(api.Env))
	case provider.OfficialAPIAuthHeader:
		req.Header.Set("x-api-key", os.Getenv(api.Env))
	case provider.OfficialAPIAuthQuery:
		query := req.URL.Query()
		query.Set(api.QueryParam, os.Getenv(api.Env))
		req.URL.RawQuery = query.Encode()
	}
	for name, value := range api.Headers {
		req.Header.Set(name, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("official API returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	return collectOfficialIDs(body, api)
}

// collectOfficialIDs extracts and normalizes model identifiers from a
// first-party payload.
func collectOfficialIDs(body []byte, api provider.OfficialAPI) ([]string, error) {
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode official API payload: %w", err)
	}
	items, err := selectItems(payload, api.ItemsPath)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(items))
	ids := make([]string, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		value, ok := object[api.EffectiveIDField()].(string)
		if !ok {
			continue
		}
		value = strings.TrimPrefix(strings.TrimSpace(value), api.IDPrefix)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		ids = append(ids, value)
	}
	sort.Strings(ids)
	return ids, nil
}

func selectItems(payload any, path string) ([]any, error) {
	if path == "" {
		if items, ok := payload.([]any); ok {
			return items, nil
		}
		return nil, fmt.Errorf("official API payload has no items array; set items_path")
	}
	current := payload
	for _, segment := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("official API payload has no %q array", path)
		}
		current, ok = object[segment]
		if !ok {
			return nil, fmt.Errorf("official API payload has no %q array", path)
		}
	}
	items, ok := current.([]any)
	if !ok {
		return nil, fmt.Errorf("official API payload %q is not an array", path)
	}
	return items, nil
}

// modelIndex maps normalized identifier keys to registry positions.
type modelIndex struct {
	keys map[string][]int
}

func buildIndex(models []registry.Model, providers []provider.Provider, target provider.Provider) modelIndex {
	index := modelIndex{keys: make(map[string][]int)}
	for position := range models {
		publisher, ok := identity.ProviderFor(models[position], providers)
		if !ok || publisher.ID != target.ID {
			continue
		}
		for _, key := range candidateKeys(models[position]) {
			index.keys[key] = append(index.keys[key], position)
		}
	}
	return index
}

// lookup returns the unique registry position for an official identifier. An
// exact normalized match wins; otherwise a single local key that is a prefix of
// the official identifier matches, which covers dated vendor identifiers such
// as "claude-sonnet-4-5-20250929". Anything ambiguous is reported, never
// guessed.
func (i modelIndex) lookup(officialID string) (int, bool) {
	key := normalizeIdentifier(officialID)
	if key == "" {
		return -1, false
	}
	if positions := uniquePositions(i.keys[key]); len(positions) == 1 {
		return positions[0], false
	} else if len(positions) > 1 {
		return -1, true
	}
	candidates := make(map[int]bool)
	for localKey, positions := range i.keys {
		if len(localKey) < minPrefixMatch || !strings.HasPrefix(key, localKey) {
			continue
		}
		for _, position := range positions {
			candidates[position] = true
		}
	}
	positions := make([]int, 0, len(candidates))
	for position := range candidates {
		positions = append(positions, position)
	}
	sort.Ints(positions)
	switch len(positions) {
	case 0:
		return -1, false
	case 1:
		return positions[0], false
	default:
		return -1, true
	}
}

func candidateKeys(model registry.Model) []string {
	values := []string{identifierSuffix(model.ID)}
	values = append(values, suffixes(model.Identifiers.OpenRouter)...)
	values = append(values, suffixes(model.Aliases)...)
	keys := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		key := normalizeIdentifier(value)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	return keys
}

func suffixes(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if registry.IsRoutingAlias(value) {
			continue
		}
		out = append(out, identifierSuffix(value))
	}
	return out
}

// identifierSuffix drops the vendor namespace from an upstream or registry
// identifier so "google/gemini-2.5-pro" compares as "gemini-2.5-pro".
func identifierSuffix(value string) string {
	value = strings.TrimSpace(value)
	if _, suffix, ok := strings.Cut(value, "/"); ok {
		return suffix
	}
	return value
}

func normalizeIdentifier(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(" ", "", "-", "", "_", "", ".", "", ":", "").Replace(value)
	return value
}

func uniquePositions(positions []int) []int {
	if len(positions) == 0 {
		return nil
	}
	seen := make(map[int]bool, len(positions))
	unique := make([]int, 0, len(positions))
	for _, position := range positions {
		if seen[position] {
			continue
		}
		seen[position] = true
		unique = append(unique, position)
	}
	sort.Ints(unique)
	return unique
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

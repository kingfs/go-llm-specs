package llmspecs

import (
	"sort"
	"strings"
)

// staticRegistry stores all static model data.
// This will be populated in models_gen.go.
var staticRegistry = map[string]*modelData{}

// aliasIndex maps aliases to their primary model IDs.
// This will be populated in models_gen.go.
var aliasIndex = map[string]string{}

// Total number of models in the registry.
func Total() int {
	return len(staticRegistry)
}

// Get retrieves a model by its ID or alias.
func Get(name string) (Model, bool) {
	// 1. Try exact ID
	if m, ok := staticRegistry[name]; ok {
		return m, true
	}

	// 2. Try alias (normalized to lowercase for case-insensitive lookup)
	if id, ok := aliasIndex[strings.ToLower(name)]; ok {
		if m, ok := staticRegistry[id]; ok {
			return m, true
		}
	}

	return nil, false
}

// GetMany retrieves multiple models by their IDs or aliases.
// It returns a slice containing the found models. Names that do not match any model are skipped.
func GetMany(names []string) []Model {
	results := make([]Model, 0, len(names))
	for _, name := range names {
		if m, ok := Get(name); ok {
			results = append(results, m)
		}
	}
	return results
}

// QueryBuilder provides a chainable API for filtering models.
type QueryBuilder struct {
	provider   string
	capability Capability
}

// Query starts a new query builder.
func Query() *QueryBuilder {
	return &QueryBuilder{}
}

// Provider filters models by provider name.
func (q *QueryBuilder) Provider(p string) *QueryBuilder {
	q.provider = p
	return q
}

// Has filters models by capability.
func (q *QueryBuilder) Has(cap Capability) *QueryBuilder {
	q.capability |= cap
	return q
}

// List returns a slice of models matching the query criteria.
func (q *QueryBuilder) List() []Model {
	var results []Model
	for _, m := range staticRegistry {
		// Filter by provider
		if q.provider != "" && !strings.EqualFold(m.ProviderVal, q.provider) {
			continue
		}
		// Filter by capabilities
		if q.capability != 0 && (m.FeaturesVal&q.capability) != q.capability {
			continue
		}
		results = append(results, m)
	}
	return results
}

// Search performs a fuzzy search across model IDs, names, and aliases.
// It returns a ranked list of models based on relevance.
func Search(query string, limit int) []Model {
	if query == "" {
		return nil
	}

	query = strings.ToLower(query)
	type searchResult struct {
		m     Model
		score int
	}
	var results []searchResult

	for _, m := range staticRegistry {
		score := 0
		id := strings.ToLower(m.ID())
		name := strings.ToLower(m.Name())

		// 1. Exact matches (Highest priority)
		if id == query {
			score += 100
		} else if name == query {
			score += 90
		}

		// 2. Prefix matches
		if strings.HasPrefix(id, query) {
			score += 50
		} else if strings.HasPrefix(name, query) {
			score += 40
		}

		// 3. Substring matches
		if strings.Contains(id, query) {
			score += 20
		} else if strings.Contains(name, query) {
			score += 10
		}

		// 4. Alias matches
		for _, alias := range m.Aliases() {
			a := strings.ToLower(alias)
			if a == query {
				score += 80
			} else if strings.Contains(a, query) {
				score += 15
			}
		}

		if score > 0 {
			results = append(results, searchResult{m, score})
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		if results[i].score == results[j].score {
			return results[i].m.ID() < results[j].m.ID()
		}
		return results[i].score > results[j].score
	})

	// Apply limit
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	final := make([]Model, len(results))
	for i, r := range results {
		final[i] = r.m
	}
	return final
}

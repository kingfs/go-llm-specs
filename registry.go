package llmspecs

import "strings"

// staticRegistry stores all static model data.
// This will be populated in models_gen.go.
var staticRegistry = map[string]*modelData{}

// aliasIndex maps aliases to their primary model IDs.
// This will be populated in models_gen.go.
var aliasIndex = map[string]string{}

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
		if q.capability != 0 && (m.Features&q.capability) != q.capability {
			continue
		}
		results = append(results, m)
	}
	return results
}

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
	family     string
	tags       []string
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

// Family filters models by structured family name.
func (q *QueryBuilder) Family(f string) *QueryBuilder {
	q.family = f
	return q
}

// Tag filters models by structured tag.
func (q *QueryBuilder) Tag(tag string) *QueryBuilder {
	tag = NormalizeTag(tag)
	if tag != "" {
		q.tags = append(q.tags, tag)
	}
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
		if q.family != "" && !strings.EqualFold(m.FamilyVal, q.family) {
			continue
		}
		if len(q.tags) > 0 && !modelHasAllTags(m, q.tags) {
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
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}

	query = strings.ToLower(query)
	queryTokens := tokenizeSearchText(query)
	type searchResult struct {
		m     Model
		score int
	}
	var results []searchResult

	for _, m := range staticRegistry {
		score := scoreModelSearch(m, query, queryTokens)
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

func modelHasAllTags(m *modelData, tags []string) bool {
	if len(tags) == 0 {
		return true
	}
	if len(m.TagList) == 0 {
		return false
	}

	have := make(map[string]struct{}, len(m.TagList))
	for _, tag := range m.TagList {
		have[NormalizeTag(tag)] = struct{}{}
	}
	for _, tag := range tags {
		if _, ok := have[NormalizeTag(tag)]; !ok {
			return false
		}
	}
	return true
}

func scoreModelSearch(m *modelData, query string, queryTokens []string) int {
	if strings.EqualFold(m.IDVal, query) {
		return 1_000_000
	}
	for _, alias := range m.AliasList {
		if strings.EqualFold(alias, query) {
			return 900_000
		}
	}
	score := 0

	score += scoreTextField(strings.ToLower(m.IDVal), query, 120, 60, 25)
	score += scoreTextField(strings.ToLower(m.NameVal), query, 110, 55, 20)
	score += scoreTextField(strings.ToLower(m.FamilyVal), query, 105, 50, 20)
	score += scoreTextField(strings.ToLower(m.SeriesVal), query, 100, 50, 20)
	score += scoreTextField(strings.ToLower(m.SummaryVal), query, 30, 0, 12)

	for _, alias := range m.AliasList {
		score += scoreTextField(strings.ToLower(alias), query, 115, 50, 20)
	}
	for _, tag := range m.TagList {
		score += scoreTextField(strings.ToLower(tag), query, 80, 35, 15)
	}

	if len(queryTokens) > 0 {
		textTokens := map[string]struct{}{}
		addTokens := func(value string) {
			for _, token := range tokenizeSearchText(value) {
				textTokens[token] = struct{}{}
			}
		}
		addTokens(m.IDVal)
		addTokens(m.NameVal)
		addTokens(m.FamilyVal)
		addTokens(m.SeriesVal)
		addTokens(m.SummaryVal)
		for _, alias := range m.AliasList {
			addTokens(alias)
		}
		for _, tag := range m.TagList {
			addTokens(tag)
		}

		matched := 0
		for _, token := range queryTokens {
			if _, ok := textTokens[token]; ok {
				matched++
			}
		}
		if matched > 0 {
			score += matched * 18
			if matched == len(queryTokens) {
				score += 25
			}
		}
	}

	return score
}

func scoreTextField(field string, query string, exact, prefix, contains int) int {
	if field == "" || query == "" {
		return 0
	}
	switch {
	case field == query:
		return exact
	case strings.HasPrefix(field, query):
		return prefix
	case strings.Contains(field, query):
		return contains
	default:
		return 0
	}
}

func tokenizeSearchText(s string) []string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return nil
	}

	replacer := strings.NewReplacer(
		"/", " ",
		":", " ",
		"-", " ",
		"_", " ",
		".", " ",
		"(", " ",
		")", " ",
		",", " ",
	)
	s = replacer.Replace(s)
	raw := strings.Fields(s)
	if len(raw) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(raw))
	tokens := make([]string, 0, len(raw))
	for _, token := range raw {
		if token == "" {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token)
	}
	return tokens
}

package llmspecs

// Model is an interface for reading model metadata.
type Model interface {
	ID() string
	Name() string
	Provider() string
	Description() string
	DescriptionCN() string
	Family() string
	Series() string
	Summary() string
	Tags() []string

	ContextLength() int
	MaxOutput() int

	HasCapability(c Capability) bool
	Features() Capability
	Aliases() []string
	HasTag(tag string) bool
	Card() ModelCard
}

// ModelCard contains the minimum structured metadata needed to render a model card quickly.
type ModelCard struct {
	ID            string
	Name          string
	Provider      string
	Family        string
	Series        string
	Summary       string
	Tags          []string
	ContextLength int
	MaxOutput     int
	Features      Capability
	Aliases       []string
}

// modelData is the internal implementation of the Model interface.
type modelData struct {
	IDVal         string
	NameVal       string
	ProviderVal   string
	DescVal       string
	DescCNVal     string
	FamilyVal     string
	SeriesVal     string
	SummaryVal    string
	TagList       []string
	ContextLenVal int
	MaxOutputVal  int
	FeaturesVal   Capability
	AliasList     []string
}

func (m *modelData) ID() string                      { return m.IDVal }
func (m *modelData) Name() string                    { return m.NameVal }
func (m *modelData) Provider() string                { return m.ProviderVal }
func (m *modelData) Description() string             { return m.DescVal }
func (m *modelData) DescriptionCN() string           { return m.DescCNVal }
func (m *modelData) Family() string                  { return m.FamilyVal }
func (m *modelData) Series() string                  { return m.SeriesVal }
func (m *modelData) Summary() string                 { return m.SummaryVal }
func (m *modelData) Tags() []string                  { return m.TagList }
func (m *modelData) ContextLength() int              { return m.ContextLenVal }
func (m *modelData) MaxOutput() int                  { return m.MaxOutputVal }
func (m *modelData) HasCapability(c Capability) bool { return m.FeaturesVal&c != 0 }
func (m *modelData) Features() Capability            { return m.FeaturesVal }
func (m *modelData) Aliases() []string               { return m.AliasList }
func (m *modelData) HasTag(tag string) bool {
	tag = NormalizeTag(tag)
	if tag == "" {
		return false
	}
	for _, existing := range m.TagList {
		if NormalizeTag(existing) == tag {
			return true
		}
	}
	return false
}
func (m *modelData) Card() ModelCard {
	return ModelCard{
		ID:            m.IDVal,
		Name:          m.NameVal,
		Provider:      m.ProviderVal,
		Family:        m.FamilyVal,
		Series:        m.SeriesVal,
		Summary:       m.SummaryVal,
		Tags:          m.TagList,
		ContextLength: m.ContextLenVal,
		MaxOutput:     m.MaxOutputVal,
		Features:      m.FeaturesVal,
		Aliases:       m.AliasList,
	}
}

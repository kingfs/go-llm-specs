package llmspecs

// Model is an interface for reading model metadata.
type Model interface {
	ID() string
	Name() string
	Provider() string
	Description() string
	DescriptionCN() string

	ContextLength() int
	MaxOutput() int

	HasCapability(c Capability) bool
	Features() Capability
	Aliases() []string
}

// modelData is the internal implementation of the Model interface.
type modelData struct {
	IDVal         string
	NameVal       string
	ProviderVal   string
	DescVal       string
	DescCNVal     string
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
func (m *modelData) ContextLength() int              { return m.ContextLenVal }
func (m *modelData) MaxOutput() int                  { return m.MaxOutputVal }
func (m *modelData) HasCapability(c Capability) bool { return m.FeaturesVal&c != 0 }
func (m *modelData) Features() Capability            { return m.FeaturesVal }
func (m *modelData) Aliases() []string               { return m.AliasList }

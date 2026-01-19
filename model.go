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

	PriceInput() float64
	PriceOutput() float64

	HasCapability(c Capability) bool
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
	PriceInVal    float64
	PriceOutVal   float64
	Features      Capability
	AliasList     []string
}

func (m *modelData) ID() string                      { return m.IDVal }
func (m *modelData) Name() string                    { return m.NameVal }
func (m *modelData) Provider() string                { return m.ProviderVal }
func (m *modelData) Description() string             { return m.DescVal }
func (m *modelData) DescriptionCN() string           { return m.DescCNVal }
func (m *modelData) ContextLength() int              { return m.ContextLenVal }
func (m *modelData) MaxOutput() int                  { return m.MaxOutputVal }
func (m *modelData) PriceInput() float64             { return m.PriceInVal }
func (m *modelData) PriceOutput() float64            { return m.PriceOutVal }
func (m *modelData) HasCapability(c Capability) bool { return m.Features&c != 0 }
func (m *modelData) Aliases() []string               { return m.AliasList }

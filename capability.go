package llmspecs

// Capability represents a model's features or modalities using bitmasks.
type Capability uint64

const (
	// Modalities (0-15 bit)
	ModalityTextIn Capability = 1 << iota
	ModalityTextOut
	ModalityImageIn
	ModalityAudioIn
	ModalityAudioOut
	ModalityVideoIn
	ModalityFileIn
	ModalityImageOut
)

const (
	// Features (16-31 bit)
	CapFunctionCall Capability = 1 << (16 + iota)
	CapJsonMode
	CapSystemPrompt
)

// Has checks if the capability set contains the given capability.
func (c Capability) Has(other Capability) bool {
	return c&other != 0
}

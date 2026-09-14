package registry

import (
	"regexp"
	"strings"
)

// Some upstream catalogs list provider or inference packaging of a model as if
// it were its own model. Those records are not model identities and must not be
// compiled into models_gen.go or the public catalog. Two classes are
// recognised:
//
//   - routing aliases: IDs in OpenRouter's "~" namespace. They resolve to
//     whichever checkpoint is current, so they have no stable identity of their
//     own. A publisher-named model such as "openai/gpt-chat-latest" is a real
//     model and is not matched.
//   - draft heads: speculative-decoding modules (DSpark, DFlash, EAGLE-3, MTP)
//     attached to another model's checkpoint. They are not standalone language
//     models.
//
// Independently published models that merely share a naming pattern (for
// example "o3-pro" or a "-preview" checkpoint) are intentionally not matched.

// routingNamespacePrefix marks upstream routing pointers that do not name a
// model. OpenRouter uses "~" for aliases that always resolve to the current
// checkpoint of a family.
const routingNamespacePrefix = "~"

// draftHeadSuffix matches speculative-decoding draft heads by ID suffix.
var draftHeadSuffix = regexp.MustCompile(`(?i)-(?:dspark|dflash|eagle3(?:-v[0-9]+)?|mtpv?[0-9]*)$`)

// draftHeadEvidence lists publisher wording that marks a record as a
// speculative-decoding draft head even when the ID does not carry a known
// suffix.
var draftHeadEvidence = []string{
	"draft head",
	"not a standalone language model",
	"does not contain the target model",
}

// IsRoutingAlias reports whether id is an upstream routing pointer rather than
// a model identity.
func IsRoutingAlias(id string) bool {
	return strings.HasPrefix(strings.TrimSpace(id), routingNamespacePrefix)
}

// IsDraftHead reports whether m describes a speculative-decoding draft head
// that is attached to another model rather than being a standalone model.
func IsDraftHead(m Model) bool {
	if draftHeadSuffix.MatchString(strings.TrimSpace(m.ID)) || draftHeadSuffix.MatchString(strings.TrimSpace(m.Name)) {
		return true
	}
	text := strings.ToLower(m.ID + " " + m.Name + " " + m.Description)
	for _, evidence := range draftHeadEvidence {
		if strings.Contains(text, evidence) {
			return true
		}
	}
	return false
}

// IsServingVariant reports whether m is provider or inference packaging of
// another model rather than an independently published model identity.
func IsServingVariant(m Model) bool {
	return IsRoutingAlias(m.ID) || IsDraftHead(m)
}

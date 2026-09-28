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
//   - quantization variants: a serialization of an existing checkpoint at a
//     different precision (BF16, FP8, NVFP4, INT4, GPTQ, AWQ, MLX, GGUF). They
//     share the model card of the checkpoint they were derived from, so they are
//     the same model and are not collected as a second record. Publishers that
//     only ship one precision are unaffected: the model record is the one that
//     cites that repository, whatever the repository is called.
//
// Independently published models that merely share a naming pattern (for
// example "o3-pro" or a "-preview" checkpoint) are intentionally not matched.

// routingNamespacePrefix marks upstream routing pointers that do not name a
// model. OpenRouter uses "~" for aliases that always resolve to the current
// checkpoint of a family.
const routingNamespacePrefix = "~"

// draftHeadSuffix matches speculative-decoding draft heads by ID suffix.
var draftHeadSuffix = regexp.MustCompile(`(?i)-(?:dspark|dflash|eagle3(?:-v[0-9]+)?|mtpv?[0-9]*)$`)

// quantizationSuffix matches the precision or compression marker a publisher
// appends to a serialization of an existing checkpoint.
var quantizationSuffix = regexp.MustCompile(`(?i)-(bf16|fp16|fp8|fp4|nvfp4|nvfp4[-_]qad|qad|int4|int8|awq|gptq|gguf|mlx|w4a4|w8a8|a8w8|[48]bit)$`)

// QuantizationFormat returns the serialization marker that makes m a precision
// or compression variant of another model, or "" when m names a model.
//
// Only the record id is inspected. The repository a model record cites may
// legitimately carry the marker, because publishers such as NVIDIA ship the
// model itself as "-BF16" and its quantizations next to it; that repository
// belongs to the model record and must not exclude it.
func QuantizationFormat(m Model) string {
	match := quantizationSuffix.FindStringSubmatch(strings.TrimSpace(m.ID))
	if match == nil {
		return ""
	}
	return strings.ToLower(strings.ReplaceAll(match[1], "_", "-"))
}

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
	return IsServingKind(ClassifyKind(m))
}

// IsCompiledKind reports whether kind may appear in a compiled artifact.
// Serving kinds and out-of-scope records are excluded.
func IsCompiledKind(kind string) bool {
	return !IsExcludedKind(kind)
}

// ClassifyKind returns the effective kind of a record. An explicit Kind field
// always wins so a human can override a false positive; otherwise the ID and
// description patterns classify routing aliases, draft heads, quantization
// variants and out-of-scope domain or non-language models. Unmatched records are
// ordinary models.
func ClassifyKind(m Model) string {
	if kind := strings.TrimSpace(m.Kind); kind != "" {
		return kind
	}
	if IsRoutingAlias(m.ID) {
		return KindServingArtifact
	}
	if IsDraftHead(m) {
		return KindDraftHead
	}
	if QuantizationFormat(m) != "" {
		return KindQuantization
	}
	if IsOutOfScope(m) {
		return KindOutOfScope
	}
	return KindModel
}

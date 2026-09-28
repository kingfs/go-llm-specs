package registry

import (
	"regexp"
	"strings"
)

// KindOutOfScope marks a record this catalog intentionally does not collect.
//
// Publishers sometimes expose a domain-specific research model (protein
// sequence generation, medical imaging reports, ...) or a non-language
// backbone (vision classifier, speech encoder, 3D generator, robot policy)
// through a language-model pipeline tag. A pipeline tag therefore cannot define
// catalog scope on its own.
//
// A record with this kind stays in models/ for provenance but never reaches
// models_gen.go, the public catalog or a Codex export. An explicit `kind` field
// in YAML always wins over these patterns, so a human review can pull a false
// positive back into scope.
const KindOutOfScope = "out-of-scope"

// ExcludedKinds are record kinds that never appear in a compiled artifact.
var ExcludedKinds = []string{KindServingArtifact, KindDraftHead, KindAdapter, KindOutOfScope}

// IsExcludedKind reports whether kind is excluded from every compiled artifact.
func IsExcludedKind(kind string) bool {
	for _, excluded := range ExcludedKinds {
		if kind == excluded {
			return true
		}
	}
	return false
}

// inScopePipelines lists the Hugging Face pipeline tags whose repositories may
// enter the catalog. Everything else describes a task this registry does not
// model, so an unexpected tag is treated as out of scope rather than silently
// accepted. Tags are the publisher's own declaration, which makes this a
// conservative default.
var inScopePipelines = map[string]bool{
	"text-generation":              true,
	"text2text-generation":         true,
	"image-text-to-text":           true,
	"visual-question-answering":    true,
	"video-text-to-text":           true,
	"feature-extraction":           true,
	"text-ranking":                 true,
	"sentence-similarity":          true,
	"translation":                  true,
	"any-to-any":                   true,
	"audio-text-to-text":           true,
	"automatic-speech-recognition": true,
	"text-to-speech":               true,
	"audio-to-audio":               true,
}

// IsInScopePipeline reports whether a Hugging Face pipeline tag describes a task
// this registry collects. An empty tag means the record was not discovered from
// a repository, so it is not judged here.
func IsInScopePipeline(tag string) bool {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return true
	}
	return inScopePipelines[tag]
}

type scopePattern struct {
	subject string
	re      *regexp.Regexp
}

// domainSubjects are publisher-authored identity signals that mark a record as
// belonging to a domain-science field rather than general language modelling.
// They are matched against identity metadata (ID, name, developer, official and
// repository identifiers, repository tags and architecture). Prose fields use
// the narrower list below so marketing copy cannot exclude a general model.
var domainSubjects = []scopePattern{
	{"protein", regexp.MustCompile(`(?i)\b(protein|proteins|peptide|peptides|proteome|proteomics|antibod\w*|amino[- ]acid|protein[- ]fold\w*|esm2?|progen2?)\b`)},
	{"nucleic-acid", regexp.MustCompile(`(?i)\b(dna|rna|genome|genomes|genomic|genomics|nucleotide|nucleotides|metagenom\w*|microbiom\w*|transcriptom\w*|dnabert|evo2?)\b`)},
	{"chemistry", regexp.MustCompile(`(?i)\b(molecul\w*|cheminformat\w*|materials?[- ](science|discovery|generation)|mattergen|drug[- ]discovery|crystal[- ]structure)\b`)},
	{"biomedicine", regexp.MustCompile(`(?i)\b(bio\w*|med(ical|icine|gemma|iphi|llama|clip|vlm)\w*|clinical|clinician|radiolog\w*|pathology|histolog\w*|dermatolog\w*|ophthalmolog\w*|chest[- ]x[- ]ray|healthcare|rad[- ]dino|skala)\b`)},
	{"earth-science", regexp.MustCompile(`(?i)\b(climate|weather|meteorolog\w*|atmospheric|earth[- ]observation|remote[- ]sensing)\b`)},
}

// domainProseSubject is the narrow subset safe to match in free-text
// descriptions. Generic words such as "medical", "biology" or "chemistry"
// commonly appear in a general model's benchmark prose, so only unambiguous
// biological-sequence and molecule wording is used here.
const domainProseRE = `(?i)\b(protein|proteins|peptide|peptides|proteome|proteomics|antibod\w*|amino[- ]acid|` +
	`dna|rna|genome|genomes|genomic|nucleotide|nucleotides|metagenom\w*|transcriptom\w*|` +
	`molecul\w*|cheminformat\w*|protein[- ]fold\w*|protein[- ]sequence\w*)\b`

var domainProse = regexp.MustCompile(domainProseRE)

// nonLLMModelTypes are architecture names that never describe a language,
// multimodal-chat, embedding or speech model. Matching is exact against the
// repository's declared architecture, which keeps vision-language models such
// as llava, molmo, qwen-vl or phi4-siglip in scope.
var nonLLMModelTypes = map[string]bool{
	// Vision classification, detection and segmentation backbones.
	"resnet": true, "resnext": true, "swin": true, "swinv2": true, "beit": true,
	"focalnet": true, "convnext": true, "convnextv2": true, "efficientnet": true,
	"mobilenet_v2": true, "mobilenetv2": true, "vgg": true, "densenet": true,
	"regnet": true, "vit": true, "vit_mae": true, "vit_msn": true, "deit": true,
	"cvt": true, "poolformer": true, "segformer": true, "maskformer": true,
	"mask2former": true, "oneformer": true, "upernet": true, "detr": true,
	"conditional_detr": true, "deformable_detr": true, "rt_detr": true,
	"yolos": true, "dpt": true, "glpn": true, "zoedepth": true, "sam": true,
	"sam2": true, "sam_hq": true, "vitmatte": true, "imagegpt": true,
	"videomae": true, "vivit": true, "timesformer": true, "x_clip": true,
	"owlvit": true, "owlv2": true, "altclip": true, "chinese_clip": true,
	"latent_zoning_networks": true,
	// Speech, audio and codec backbones. Speech synthesis and recognition
	// models stay in scope; these are the encoders behind them.
	"wavlm": true, "hubert": true, "unispeech": true, "unispeech_sat": true,
	"wav2vec2": true, "wav2vec2_conformer": true, "wav2vec2_bert": true,
	"data2vec_audio": true, "sew": true, "sew_d": true, "clap": true,
	"encodec": true, "musicgen": true,
	// Vision and 3D generative or geometric backbones.
	"graphormer": true, "dit": true, "vq_diffusion": true,
}

// backboneNameRE catches non-language backbone families whose repository name
// carries the evidence even when the declared architecture is generic.
var backboneNameRE = regexp.MustCompile(`(?i)\b(resnet|resnext|swin|swinv2|beit|focalnet|convnext|densenet|` +
	`efficientnet|mobilenet|regnet|segformer|mask2former|upernet|deit|vit_mae|videomae|dinov2?|` +
	`wavlm|hubert|unispeech|wav2vec2?|data2vec|encodec|musicgen|graphormer)\b`)

// ScopeReason returns why m is outside the catalog scope, or "" when it is in
// scope. The reason is stable so it can be written into reports and candidate
// queues.
func ScopeReason(m Model) string {
	// An explicit kind is a human decision and always wins.
	if strings.TrimSpace(m.Kind) != "" {
		return ""
	}
	hf := m.Upstream.HuggingFace
	if hf != nil {
		if tag := strings.ToLower(strings.TrimSpace(hf.PipelineTag)); tag != "" && !inScopePipelines[tag] {
			return "out-of-scope-pipeline:" + tag
		}
	}
	identity := m.identityText()
	for _, pattern := range domainSubjects {
		if pattern.re.MatchString(identity) {
			return "out-of-scope-domain:" + pattern.subject
		}
	}
	prose := strings.Join([]string{m.Description, m.DescriptionCN}, " ")
	if domainProse.MatchString(prose) {
		return "out-of-scope-domain:prose"
	}
	if hf != nil {
		if modelType := strings.ToLower(strings.TrimSpace(hf.ModelType)); nonLLMModelTypes[modelType] {
			return "out-of-scope-backbone:" + modelType
		}
	}
	if match := backboneNameRE.FindString(m.ID + " " + m.Name); match != "" {
		return "out-of-scope-backbone:" + strings.ToLower(match)
	}
	return ""
}

// IsOutOfScope reports whether m is a domain-specific or non-language model the
// catalog does not collect.
func IsOutOfScope(m Model) bool {
	return ScopeReason(m) != ""
}

// identityText joins the publisher-authored fields that describe what a record
// is. Free-text descriptions are deliberately excluded; see ScopeReason.
func (m Model) identityText() string {
	parts := []string{m.ID, m.Name, m.Developer, m.Provider}
	parts = append(parts, m.Identifiers.Official...)
	parts = append(parts, m.Identifiers.HuggingFace...)
	if hf := m.Upstream.HuggingFace; hf != nil {
		parts = append(parts, hf.ID, hf.ModelType)
		parts = append(parts, hf.Tags...)
	}
	return strings.Join(parts, " ")
}

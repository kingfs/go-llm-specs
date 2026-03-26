package llmspecs

import "strings"

// Tag is a stable canonical label that can be used by downstream projects to render model badges.
type Tag string

const (
	TagChat             Tag = "chat"
	TagEmbedding        Tag = "embedding"
	TagRerank           Tag = "rerank"
	TagTTS              Tag = "tts"
	TagASR              Tag = "asr"
	TagToolUse          Tag = "tool-use"
	TagStructuredOutput Tag = "structured-output"
	TagMultimodal       Tag = "multimodal"
	TagVision           Tag = "vision"
	TagImageGeneration  Tag = "image-generation"
	TagAudioInput       Tag = "audio-input"
	TagAudioOutput      Tag = "audio-output"
	TagVideoInput       Tag = "video-input"
	TagFileInput        Tag = "file-input"
	TagCoding           Tag = "coding"
	TagReasoning        Tag = "reasoning"
	TagAgent            Tag = "agent"
	TagSearch           Tag = "search"
	TagPreview          Tag = "preview"
	TagExperimental     Tag = "experimental"
	TagFast             Tag = "fast"
	TagMini             Tag = "mini"
	TagNano             Tag = "nano"
	TagTurbo            Tag = "turbo"
	TagPro              Tag = "pro"
	TagFree             Tag = "free"
	TagThinking         Tag = "thinking"
)

type TagDescriptor struct {
	Name     Tag
	Category string
	Label    string
}

var knownTagDescriptors = []TagDescriptor{
	{Name: TagChat, Category: "task", Label: "Chat"},
	{Name: TagEmbedding, Category: "task", Label: "Embedding"},
	{Name: TagRerank, Category: "task", Label: "Rerank"},
	{Name: TagTTS, Category: "task", Label: "TTS"},
	{Name: TagASR, Category: "task", Label: "ASR"},
	{Name: TagCoding, Category: "task", Label: "Coding"},
	{Name: TagReasoning, Category: "task", Label: "Reasoning"},
	{Name: TagAgent, Category: "task", Label: "Agent"},
	{Name: TagSearch, Category: "task", Label: "Search"},
	{Name: TagToolUse, Category: "feature", Label: "Tool Use"},
	{Name: TagStructuredOutput, Category: "feature", Label: "Structured Output"},
	{Name: TagMultimodal, Category: "modality", Label: "Multimodal"},
	{Name: TagVision, Category: "modality", Label: "Vision"},
	{Name: TagImageGeneration, Category: "modality", Label: "Image Generation"},
	{Name: TagAudioInput, Category: "modality", Label: "Audio Input"},
	{Name: TagAudioOutput, Category: "modality", Label: "Audio Output"},
	{Name: TagVideoInput, Category: "modality", Label: "Video Input"},
	{Name: TagFileInput, Category: "modality", Label: "File Input"},
	{Name: TagPreview, Category: "release", Label: "Preview"},
	{Name: TagExperimental, Category: "release", Label: "Experimental"},
	{Name: TagFast, Category: "variant", Label: "Fast"},
	{Name: TagMini, Category: "variant", Label: "Mini"},
	{Name: TagNano, Category: "variant", Label: "Nano"},
	{Name: TagTurbo, Category: "variant", Label: "Turbo"},
	{Name: TagPro, Category: "variant", Label: "Pro"},
	{Name: TagFree, Category: "variant", Label: "Free"},
	{Name: TagThinking, Category: "variant", Label: "Thinking"},
}

var tagAliases = map[string]Tag{
	"agentic":            TagAgent,
	"agents":             TagAgent,
	"asr":                TagASR,
	"audio-in":           TagAudioInput,
	"audio-input":        TagAudioInput,
	"audio-out":          TagAudioOutput,
	"audio-output":       TagAudioOutput,
	"beta":               TagPreview,
	"chat":               TagChat,
	"code":               TagCoding,
	"coder":              TagCoding,
	"coding":             TagCoding,
	"embedding":          TagEmbedding,
	"embeddings":         TagEmbedding,
	"experimental":       TagExperimental,
	"fast":               TagFast,
	"file-input":         TagFileInput,
	"free":               TagFree,
	"image-generation":   TagImageGeneration,
	"image-input":        TagVision,
	"mini":               TagMini,
	"multimodal":         TagMultimodal,
	"nano":               TagNano,
	"preview":            TagPreview,
	"pro":                TagPro,
	"reasoning":          TagReasoning,
	"rerank":             TagRerank,
	"reranker":           TagRerank,
	"research":           TagSearch,
	"search":             TagSearch,
	"structured-output":  TagStructuredOutput,
	"structured_outputs": TagStructuredOutput,
	"thinking":           TagThinking,
	"tool-use":           TagToolUse,
	"tool_use":           TagToolUse,
	"tools":              TagToolUse,
	"transcription":      TagASR,
	"tts":                TagTTS,
	"turbo":              TagTurbo,
	"video-input":        TagVideoInput,
	"vision":             TagVision,
}

// KnownTags returns the stable tag catalog that downstream projects can use for rendering and grouping.
func KnownTags() []TagDescriptor {
	out := make([]TagDescriptor, len(knownTagDescriptors))
	copy(out, knownTagDescriptors)
	return out
}

// NormalizeTag maps an arbitrary tag-like string to a canonical taxonomy tag.
func NormalizeTag(tag string) string {
	tag = strings.TrimSpace(strings.ToLower(tag))
	if tag == "" {
		return ""
	}
	if canonical, ok := tagAliases[tag]; ok {
		return string(canonical)
	}
	return tag
}

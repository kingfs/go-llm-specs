package main

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	llmspecs "github.com/kingfs/go-llm-specs"
)

type structuredMetadata struct {
	Family  string
	Series  string
	Summary string
	Tags    []string
}

var tokenSplitRe = regexp.MustCompile(`[^a-z0-9]+`)

func deriveStructuredMetadata(m ModelRegistry) structuredMetadata {
	series := deriveSeries(m)
	family := deriveFamily(m, series)
	summary := deriveSummary(m)
	tags := deriveTags(m, family, series)

	return structuredMetadata{
		Family:  family,
		Series:  series,
		Summary: summary,
		Tags:    tags,
	}
}

func deriveSeries(m ModelRegistry) string {
	name := strings.TrimSpace(m.Name)
	if idx := strings.Index(name, ":"); idx >= 0 {
		name = strings.TrimSpace(name[idx+1:])
	}
	if name != "" {
		return name
	}

	parts := strings.Split(m.ID, "/")
	if len(parts) == 0 {
		return ""
	}
	suffix := parts[len(parts)-1]
	suffix = strings.ReplaceAll(suffix, ":", " ")
	suffix = strings.ReplaceAll(suffix, "-", " ")
	suffix = strings.ReplaceAll(suffix, "_", " ")
	return strings.TrimSpace(strings.Join(strings.Fields(suffix), " "))
}

func deriveFamily(m ModelRegistry, series string) string {
	id := strings.ToLower(m.ID)
	seriesLower := strings.ToLower(series)
	switch {
	case strings.Contains(id, "chatgpt") || strings.Contains(id, "gpt") || strings.Contains(seriesLower, "gpt"):
		return "GPT"
	case strings.Contains(id, "claude") || strings.Contains(seriesLower, "claude"):
		return "Claude"
	case strings.Contains(id, "gemini") || strings.Contains(seriesLower, "gemini"):
		return "Gemini"
	case strings.Contains(id, "gemma") || strings.Contains(seriesLower, "gemma"):
		return "Gemma"
	case strings.Contains(id, "qwen") || strings.Contains(id, "qwq") || strings.Contains(seriesLower, "qwen"):
		return "Qwen"
	case strings.Contains(id, "llama") || strings.Contains(seriesLower, "llama"):
		return "Llama"
	case strings.Contains(id, "mistral") || strings.Contains(seriesLower, "mistral"):
		return "Mistral"
	case strings.Contains(id, "mixtral") || strings.Contains(seriesLower, "mixtral"):
		return "Mixtral"
	case strings.Contains(id, "codestral") || strings.Contains(seriesLower, "codestral"):
		return "Codestral"
	case strings.Contains(id, "pixtral") || strings.Contains(seriesLower, "pixtral"):
		return "Pixtral"
	case strings.Contains(id, "ministral") || strings.Contains(seriesLower, "ministral"):
		return "Ministral"
	case strings.Contains(id, "grok") || strings.Contains(seriesLower, "grok"):
		return "Grok"
	case strings.Contains(id, "deepseek") || strings.Contains(seriesLower, "deepseek"):
		return "DeepSeek"
	case strings.Contains(id, "kimi") || strings.Contains(seriesLower, "kimi"):
		return "Kimi"
	case strings.Contains(id, "glm") || strings.Contains(seriesLower, "glm"):
		return "GLM"
	case strings.Contains(id, "nova") || strings.Contains(seriesLower, "nova"):
		return "Nova"
	case strings.Contains(id, "sonar") || strings.Contains(seriesLower, "sonar"):
		return "Sonar"
	case strings.Contains(id, "command") || strings.Contains(seriesLower, "command"):
		return "Command"
	case strings.Contains(id, "olmo") || strings.Contains(seriesLower, "olmo"):
		return "OLMo"
	case strings.Contains(id, "jamba") || strings.Contains(seriesLower, "jamba"):
		return "Jamba"
	}

	token := firstAlphaNumToken(series)
	if token == "" {
		token = firstAlphaNumToken(m.ID)
	}
	return titleWord(token)
}

func deriveSummary(m ModelRegistry) string {
	raw := strings.TrimSpace(m.DescriptionCN)
	if raw == "" {
		raw = strings.TrimSpace(m.Description)
	}
	if raw == "" {
		return ""
	}

	paragraph := strings.Split(raw, "\n\n")[0]
	paragraph = strings.TrimSpace(paragraph)
	if paragraph == "" {
		return ""
	}

	var builder strings.Builder
	for _, r := range paragraph {
		builder.WriteRune(r)
		switch r {
		case '.', '!', '?', '。', '！', '？':
			summary := strings.TrimSpace(builder.String())
			if summary != "" {
				return summary
			}
		}
	}
	return strings.TrimSpace(paragraph)
}

func deriveTags(m ModelRegistry, family, series string) []string {
	var tags []string
	content := strings.ToLower(strings.Join([]string{m.ID, m.Name, m.Description, m.DescriptionCN, family, series}, " "))

	addTag := func(tag string) {
		tags = append(tags, tag)
	}

	for _, feature := range m.Features {
		switch feature {
		case "CapChat":
			addTag(string(llmspecs.TagChat))
		case "CapEmbedding":
			addTag(string(llmspecs.TagEmbedding))
		case "CapRerank":
			addTag(string(llmspecs.TagRerank))
		case "CapTTS":
			addTag(string(llmspecs.TagTTS))
		case "CapASR":
			addTag(string(llmspecs.TagASR))
		case "CapFunctionCall":
			addTag(string(llmspecs.TagToolUse))
		case "CapJsonMode":
			addTag(string(llmspecs.TagStructuredOutput))
		case "CapMultimodal":
			addTag(string(llmspecs.TagMultimodal))
		case "ModalityImageIn":
			addTag(string(llmspecs.TagVision))
		case "ModalityImageOut":
			addTag(string(llmspecs.TagImageGeneration))
		case "ModalityAudioIn":
			addTag(string(llmspecs.TagAudioInput))
		case "ModalityAudioOut":
			addTag(string(llmspecs.TagAudioOutput))
		case "ModalityVideoIn":
			addTag(string(llmspecs.TagVideoInput))
		case "ModalityFileIn":
			addTag(string(llmspecs.TagFileInput))
		}
	}

	for _, rule := range keywordTagRules {
		if rule.pattern.MatchString(content) {
			addTag(rule.tag)
		}
	}

	if family != "" {
		addTag(llmspecs.NormalizeTag(family))
	}

	return normalizeTagList(tags)
}

// tagRule maps publisher wording to a canonical tag. A rule matches when one of
// its markers appears in the model metadata as a whole word or phrase. Matching
// is bounded by alphanumeric characters so a short marker such as "pro" is not
// triggered by "protein" and "ide" is not triggered by "provides".
type tagRule struct {
	tag     string
	pattern *regexp.Regexp
}

func newTagRule(tag string, markers ...string) tagRule {
	alternatives := make([]string, 0, len(markers))
	for _, marker := range markers {
		marker = strings.ToLower(strings.TrimSpace(marker))
		quoted := regexp.QuoteMeta(marker)
		// Only the alphanumeric edges of a marker need a word boundary. A
		// marker that already starts or ends with punctuation (":free",
		// "(thinking)") is anchored by that punctuation itself.
		if isAlnumByte(marker[0]) {
			quoted = `(?:^|[^a-z0-9])` + quoted
		}
		if isAlnumByte(marker[len(marker)-1]) {
			quoted += `(?:[^a-z0-9]|$)`
		}
		alternatives = append(alternatives, quoted)
	}
	return tagRule{
		tag:     tag,
		pattern: regexp.MustCompile(`(?i)(?:` + strings.Join(alternatives, "|") + `)`),
	}
}

func isAlnumByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}

var keywordTagRules = []tagRule{
	newTagRule(string(llmspecs.TagCoding), "coder", "coding", "codex", "software engineering", "swe-bench", "cli", "ide"),
	newTagRule(string(llmspecs.TagReasoning), "reasoning", "reasoner", "deep reasoning", "think", "thinking"),
	newTagRule(string(llmspecs.TagAgent), "agent", "agents", "agentic", "tool orchestration", "autonomous"),
	newTagRule(string(llmspecs.TagSearch), "search", "retrieval", "research", "deepresearch"),
	newTagRule(string(llmspecs.TagPreview), "preview", "beta"),
	newTagRule(string(llmspecs.TagExperimental), "experimental", "alpha"),
	newTagRule(string(llmspecs.TagFast), "fast", "low latency"),
	newTagRule(string(llmspecs.TagMini), "mini"),
	newTagRule(string(llmspecs.TagNano), "nano"),
	newTagRule(string(llmspecs.TagPro), "pro"),
	newTagRule(string(llmspecs.TagTurbo), "turbo"),
	newTagRule(string(llmspecs.TagFree), ":free", "(free)"),
	newTagRule(string(llmspecs.TagThinking), ":thinking", "(thinking)", "thinking"),
}

func normalizeTagList(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := make(map[string]string, len(tags))
	for _, tag := range tags {
		tag = llmspecs.NormalizeTag(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; !ok {
			seen[tag] = tag
		}
	}
	result := make([]string, 0, len(seen))
	for _, tag := range seen {
		result = append(result, tag)
	}
	sort.Strings(result)
	return result
}

func firstAlphaNumToken(s string) string {
	for _, token := range tokenSplitRe.Split(strings.ToLower(s), -1) {
		if token != "" {
			return token
		}
	}
	return ""
}

func titleWord(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(strings.ToLower(s))
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

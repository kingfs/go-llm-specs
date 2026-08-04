package registry

import "time"

const CurrentSchemaVersion = 2

// Model is the shared on-disk representation used by every registry command.
// Extra preserves forward-compatible top-level fields during load/save cycles.
type Model struct {
	SchemaVersion int                   `yaml:"schema_version,omitempty" json:"schema_version,omitempty"`
	ID            string                `yaml:"id" json:"id"`
	Name          string                `yaml:"name" json:"name"`
	NameCN        string                `yaml:"name_cn,omitempty" json:"name_cn,omitempty"`
	Provider      string                `yaml:"provider" json:"provider"`
	Developer     string                `yaml:"developer,omitempty" json:"developer,omitempty"`
	Lifecycle     string                `yaml:"lifecycle,omitempty" json:"lifecycle,omitempty"`
	Description   string                `yaml:"description,omitempty" json:"description,omitempty"`
	DescriptionCN string                `yaml:"description_cn,omitempty" json:"description_cn,omitempty"`
	ContextLen    int                   `yaml:"context_length" json:"context_length"`
	MaxOutput     int                   `yaml:"max_output,omitempty" json:"max_output,omitempty"`
	Features      []string              `yaml:"features,omitempty" json:"features,omitempty"`
	Aliases       []string              `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	DiscoveredAt  *time.Time            `yaml:"discovered_at,omitempty" json:"discovered_at,omitempty"`
	Links         ModelLinks            `yaml:"links,omitempty" json:"links,omitempty"`
	Identifiers   ModelIdentifiers      `yaml:"identifiers,omitempty" json:"identifiers,omitempty"`
	Provenance    map[string]Provenance `yaml:"provenance,omitempty" json:"provenance,omitempty"`
	Upstream      UpstreamMetadata      `yaml:"upstream,omitempty" json:"upstream,omitempty"`
	Reasoning     *ReasoningMetadata    `yaml:"reasoning,omitempty" json:"reasoning,omitempty"`
	Codex         *CodexMetadata        `yaml:"codex,omitempty" json:"codex,omitempty"`
	Extra         map[string]any        `yaml:",inline" json:"-"`

	FilePath string `yaml:"-" json:"-"`
}

// ModelLinks contains publication links for the model itself. Official is the
// publisher's canonical model page or announcement; the remaining links may
// point to official organization accounts on third-party hosts.
type ModelLinks struct {
	Official      string   `yaml:"official,omitempty" json:"official,omitempty"`
	Announcement  string   `yaml:"announcement,omitempty" json:"announcement,omitempty"`
	Documentation string   `yaml:"documentation,omitempty" json:"documentation,omitempty"`
	ModelCard     string   `yaml:"model_card,omitempty" json:"model_card,omitempty"`
	Paper         string   `yaml:"paper,omitempty" json:"paper,omitempty"`
	Repository    string   `yaml:"repository,omitempty" json:"repository,omitempty"`
	License       string   `yaml:"license,omitempty" json:"license,omitempty"`
	Other         []string `yaml:"other,omitempty" json:"other,omitempty"`
}

func (l ModelLinks) IsZero() bool {
	return l.Official == "" && l.Announcement == "" && l.Documentation == "" &&
		l.ModelCard == "" && l.Paper == "" && l.Repository == "" &&
		l.License == "" && len(l.Other) == 0
}

// ModelIdentifiers maps the stable registry record to names assigned by the
// publisher and discovery catalogs. These are identities, not aliases exposed
// by a particular inference deployment.
type ModelIdentifiers struct {
	Official    []string `yaml:"official,omitempty" json:"official,omitempty"`
	HuggingFace []string `yaml:"huggingface,omitempty" json:"huggingface,omitempty"`
	ModelScope  []string `yaml:"modelscope,omitempty" json:"modelscope,omitempty"`
	OpenRouter  []string `yaml:"openrouter,omitempty" json:"openrouter,omitempty"`
}

func (i ModelIdentifiers) IsZero() bool {
	return len(i.Official) == 0 && len(i.HuggingFace) == 0 &&
		len(i.ModelScope) == 0 && len(i.OpenRouter) == 0
}

// Provenance records why a compiled top-level model fact was selected. It is
// intentionally field-addressed so conflicting sources can be audited without
// changing the convenient human-readable model fields.
type Provenance struct {
	Source      string     `yaml:"source" json:"source"`
	URL         string     `yaml:"url,omitempty" json:"url,omitempty"`
	RetrievedAt *time.Time `yaml:"retrieved_at,omitempty" json:"retrieved_at,omitempty"`
}

func (m Model) IsV2() bool { return m.SchemaVersion >= CurrentSchemaVersion }

type UpstreamMetadata struct {
	OpenRouter  *OpenRouterMetadata  `yaml:"openrouter,omitempty" json:"openrouter,omitempty"`
	HuggingFace *HuggingFaceMetadata `yaml:"huggingface,omitempty" json:"huggingface,omitempty"`
	Extra       map[string]any       `yaml:",inline" json:"-"`
}

func (u UpstreamMetadata) IsZero() bool {
	return u.OpenRouter == nil && u.HuggingFace == nil && len(u.Extra) == 0
}

type OpenRouterMetadata struct {
	CanonicalSlug       string         `yaml:"canonical_slug,omitempty" json:"canonical_slug,omitempty"`
	HuggingFaceID       string         `yaml:"huggingface_id,omitempty" json:"huggingface_id,omitempty"`
	SupportedParameters []string       `yaml:"supported_parameters,omitempty" json:"supported_parameters,omitempty"`
	InputModalities     []string       `yaml:"input_modalities,omitempty" json:"input_modalities,omitempty"`
	OutputModalities    []string       `yaml:"output_modalities,omitempty" json:"output_modalities,omitempty"`
	KnowledgeCutoff     string         `yaml:"knowledge_cutoff,omitempty" json:"knowledge_cutoff,omitempty"`
	FetchedAt           *time.Time     `yaml:"fetched_at,omitempty" json:"fetched_at,omitempty"`
	Extra               map[string]any `yaml:",inline" json:"-"`
}

type HuggingFaceMetadata struct {
	ID                      string         `yaml:"id" json:"id"`
	Revision                string         `yaml:"revision,omitempty" json:"revision,omitempty"`
	PipelineTag             string         `yaml:"pipeline_tag,omitempty" json:"pipeline_tag,omitempty"`
	ModelType               string         `yaml:"model_type,omitempty" json:"model_type,omitempty"`
	Architectures           []string       `yaml:"architectures,omitempty" json:"architectures,omitempty"`
	License                 string         `yaml:"license,omitempty" json:"license,omitempty"`
	Tags                    []string       `yaml:"tags,omitempty" json:"tags,omitempty"`
	ConfigContextLength     int            `yaml:"config_context_length,omitempty" json:"config_context_length,omitempty"`
	TokenizerModelMaxLength int            `yaml:"tokenizer_model_max_length,omitempty" json:"tokenizer_model_max_length,omitempty"`
	ProcessorClass          string         `yaml:"processor_class,omitempty" json:"processor_class,omitempty"`
	ChatTemplateSHA256      string         `yaml:"chat_template_sha256,omitempty" json:"chat_template_sha256,omitempty"`
	StructuredFiles         []string       `yaml:"structured_files,omitempty" json:"structured_files,omitempty"`
	FetchedAt               *time.Time     `yaml:"fetched_at,omitempty" json:"fetched_at,omitempty"`
	Extra                   map[string]any `yaml:",inline" json:"-"`
}

type ReasoningMetadata struct {
	Supported        bool           `yaml:"supported" json:"supported"`
	Mandatory        bool           `yaml:"mandatory,omitempty" json:"mandatory,omitempty"`
	DefaultEnabled   bool           `yaml:"default_enabled,omitempty" json:"default_enabled,omitempty"`
	Parser           string         `yaml:"parser,omitempty" json:"parser,omitempty"`
	DefaultEffort    string         `yaml:"default_effort,omitempty" json:"default_effort,omitempty"`
	SupportedEfforts []string       `yaml:"supported_efforts,omitempty" json:"supported_efforts,omitempty"`
	Extra            map[string]any `yaml:",inline" json:"-"`
}

type CodexMetadata struct {
	Enabled                           bool             `yaml:"enabled" json:"enabled"`
	Slugs                             []string         `yaml:"slugs,omitempty" json:"slugs,omitempty"`
	ShellType                         string           `yaml:"shell_type,omitempty" json:"shell_type,omitempty"`
	Visibility                        string           `yaml:"visibility,omitempty" json:"visibility,omitempty"`
	SupportedInAPI                    *bool            `yaml:"supported_in_api,omitempty" json:"supported_in_api,omitempty"`
	Priority                          int              `yaml:"priority,omitempty" json:"priority,omitempty"`
	BaseInstructions                  string           `yaml:"base_instructions,omitempty" json:"base_instructions,omitempty"`
	SupportsParallelToolCalls         bool             `yaml:"supports_parallel_tool_calls" json:"supports_parallel_tool_calls"`
	SupportsReasoningSummaryParameter bool             `yaml:"supports_reasoning_summary_parameter" json:"supports_reasoning_summary_parameter"`
	SupportVerbosity                  bool             `yaml:"support_verbosity" json:"support_verbosity"`
	DefaultVerbosity                  string           `yaml:"default_verbosity,omitempty" json:"default_verbosity,omitempty"`
	ApplyPatchToolType                string           `yaml:"apply_patch_tool_type,omitempty" json:"apply_patch_tool_type,omitempty"`
	WebSearchToolType                 string           `yaml:"web_search_tool_type,omitempty" json:"web_search_tool_type,omitempty"`
	TruncationPolicy                  TruncationPolicy `yaml:"truncation_policy,omitempty" json:"truncation_policy,omitempty"`
	EffectiveContextWindowPercent     int              `yaml:"effective_context_window_percent,omitempty" json:"effective_context_window_percent,omitempty"`
	InputModalities                   []string         `yaml:"input_modalities,omitempty" json:"input_modalities,omitempty"`
	Extra                             map[string]any   `yaml:",inline" json:"-"`
}

type TruncationPolicy struct {
	Mode  string `yaml:"mode,omitempty" json:"mode,omitempty"`
	Limit int    `yaml:"limit,omitempty" json:"limit,omitempty"`
}

func (t TruncationPolicy) IsZero() bool { return t.Mode == "" && t.Limit == 0 }

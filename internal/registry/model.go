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
	Description   string                `yaml:"description,omitempty" json:"description,omitempty"`
	DescriptionCN string                `yaml:"description_cn,omitempty" json:"description_cn,omitempty"`
	ContextLen    int                   `yaml:"context_length" json:"context_length"`
	MaxOutput     int                   `yaml:"max_output,omitempty" json:"max_output,omitempty"`
	Features      []string              `yaml:"features,omitempty" json:"features,omitempty"`
	Aliases       []string              `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	DiscoveredAt  *time.Time            `yaml:"discovered_at,omitempty" json:"discovered_at,omitempty"`
	Upstream      UpstreamMetadata      `yaml:"upstream,omitempty" json:"upstream,omitempty"`
	Reasoning     *ReasoningMetadata    `yaml:"reasoning,omitempty" json:"reasoning,omitempty"`
	Codex         *CodexMetadata        `yaml:"codex,omitempty" json:"codex,omitempty"`
	Deployments   map[string]Deployment `yaml:"deployments,omitempty" json:"deployments,omitempty"`
	Extra         map[string]any        `yaml:",inline" json:"-"`

	FilePath string `yaml:"-" json:"-"`
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
	ID            string         `yaml:"id" json:"id"`
	PipelineTag   string         `yaml:"pipeline_tag,omitempty" json:"pipeline_tag,omitempty"`
	ModelType     string         `yaml:"model_type,omitempty" json:"model_type,omitempty"`
	Architectures []string       `yaml:"architectures,omitempty" json:"architectures,omitempty"`
	License       string         `yaml:"license,omitempty" json:"license,omitempty"`
	Tags          []string       `yaml:"tags,omitempty" json:"tags,omitempty"`
	FetchedAt     *time.Time     `yaml:"fetched_at,omitempty" json:"fetched_at,omitempty"`
	Extra         map[string]any `yaml:",inline" json:"-"`
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
	SupportsParallelToolCalls         bool             `yaml:"supports_parallel_tool_calls,omitempty" json:"supports_parallel_tool_calls,omitempty"`
	SupportsReasoningSummaryParameter bool             `yaml:"supports_reasoning_summary_parameter,omitempty" json:"supports_reasoning_summary_parameter,omitempty"`
	SupportVerbosity                  bool             `yaml:"support_verbosity,omitempty" json:"support_verbosity,omitempty"`
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

type Deployment struct {
	ModelSlug            string          `yaml:"model_slug,omitempty" json:"model_slug,omitempty"`
	ServerVersion        string          `yaml:"server_version,omitempty" json:"server_version,omitempty"`
	WireAPIs             []string        `yaml:"wire_apis,omitempty" json:"wire_apis,omitempty"`
	ContextLength        int             `yaml:"context_length,omitempty" json:"context_length,omitempty"`
	VerifiedCapabilities map[string]bool `yaml:"verified_capabilities,omitempty" json:"verified_capabilities,omitempty"`
	TestedAt             *time.Time      `yaml:"tested_at,omitempty" json:"tested_at,omitempty"`
	Extra                map[string]any  `yaml:",inline" json:"-"`
}

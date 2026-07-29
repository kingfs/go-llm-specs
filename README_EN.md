# go-llm-specs

An LLM model metadata registry for Go applications. It compiles model IDs, providers, context windows, input/output modalities, tool-use support, JSON mode, aliases, tags, and English/Chinese descriptions into your binary.

[English](./README_EN.md) | [中文](./README.md)

[![Daily Model Sync](https://github.com/kingfs/go-llm-specs/actions/workflows/daily-update.yml/badge.svg)](https://github.com/kingfs/go-llm-specs/actions/workflows/daily-update.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/kingfs/go-llm-specs.svg)](https://pkg.go.dev/github.com/kingfs/go-llm-specs)

## Why This Exists

When a Go product supports multiple LLM providers, the same problems tend to show up quickly:

- Users type aliases such as `gpt4t`, `claude sonnet`, or `qwen3-32b`, and your app needs to resolve them to stable model IDs.
- Model pickers, admin consoles, billing rules, and routing policies need names, providers, context windows, and capability labels.
- Your code needs to know whether a model supports image input, function calling, structured output, embedding, reranking, TTS, or ASR.
- You do not want every service startup to depend on a remote model-list endpoint, and you do not want stale model constants scattered across business code.

`go-llm-specs` packages that metadata as a static, type-safe Go library. Runtime lookups are in-memory and require no network I/O, making it a good fit for API services, agent platforms, model gateways, dashboards, CLI tools, and internal operations systems.

## What You Get

- Static registry: hundreds of models, synchronized and generated into `models_gen.go`.
- Unified model cards: ID, name, provider, family, series, summary, tags, context length, max output, and capabilities.
- Alias resolution: lookup by model ID or alias, with case-insensitive alias matching.
- Capability filters: query Vision, Tool Use, JSON mode, Embedding, Rerank, and other model capabilities with Go constants.
- Fuzzy search: search across model IDs, names, series, tags, summaries, and aliases for model pickers.
- Zero runtime network dependency: the registry is compiled into your binary.
- English and Chinese descriptions: useful for localized product interfaces.

## Installation

```bash
go get github.com/kingfs/go-llm-specs
```

## Quick Start

```go
package main

import (
	"fmt"

	llmspecs "github.com/kingfs/go-llm-specs"
)

func main() {
	model, ok := llmspecs.Get("gpt4t")
	if !ok {
		return
	}

	fmt.Println(model.ID())                // openai/gpt-4-turbo
	fmt.Println(model.Provider())          // OpenAI
	fmt.Println(model.ContextLength())     // context window
	fmt.Println(model.Features().String()) // TextIn|TextOut|...
}
```

See [examples/basic/main.go](./examples/basic/main.go) for a runnable example.

## Common Patterns

### Build a model picker

```go
for _, model := range llmspecs.Search("claude sonnet", 10) {
	card := model.Card()
	fmt.Printf("%s: %s [%s]\n", card.Provider, card.Name, card.ID)
}
```

### Find models with image input and tool use

```go
models := llmspecs.Query().
	Has(llmspecs.ModalityImageIn).
	Has(llmspecs.CapFunctionCall).
	List()
```

### List models from one provider

```go
anthropicVisionModels := llmspecs.Query().
	Provider("Anthropic").
	Has(llmspecs.ModalityImageIn).
	List()
```

### Validate configured model names

```go
configured := []string{"gpt4t", "qwen3-32b", "not-exist"}
validModels := llmspecs.GetMany(configured)
```

### Group models by tags

```go
reasoningModels := llmspecs.Query().
	Tag(string(llmspecs.TagReasoning)).
	List()

for _, tag := range llmspecs.KnownTags() {
	fmt.Println(tag.Category, tag.Name, tag.Label)
}
```

## API Overview

| API | Description |
| --- | --- |
| `Total()` | Return the number of models in the registry |
| `Get(idOrAlias)` | Get a model by ID or alias |
| `GetMany(idsOrAliases)` | Batch lookup; missing entries are skipped |
| `Search(query, limit)` | Fuzzy search over IDs, names, series, summaries, tags, and aliases |
| `Query()` | Create a chainable model query |
| `KnownTags()` | Return the stable tag catalog for downstream rendering and grouping |
| `Model.Card()` | Return a compact structure suitable for UI model cards |

Core `Model` fields include:

- `ID()`, `Name()`, `Provider()`, `Family()`, `Series()`
- `Description()`, `DescriptionCN()`, `Summary()`
- `ContextLength()`, `MaxOutput()`
- `Features()`, `HasCapability()`, `Tags()`, `HasTag()`, `Aliases()`

Capability constants live in [capability.go](./capability.go), and tag constants live in [tag.go](./tag.go).

## Where It Fits

- Model gateways: route by user selection, capability requirements, or provider policy.
- Agent platforms: expose only models that support tool use, structured output, or multimodal input.
- SaaS dashboards: render model lists, tags, context windows, and localized descriptions.
- CLIs and SDKs: validate config files and provide search/autocomplete results.
- Internal platforms: replace hard-coded model tables spread across service code.

## Data Source And Updates

The project currently syncs model metadata primarily from OpenRouter. Human-maintained `models/**/*.yaml` files provide corrections, extra fields, aliases, and Chinese descriptions. The final registry is generated into `models_gen.go`, so downstream applications only need to import the Go package; they do not need to run sync jobs at runtime.

Maintainer-facing files:

```text
.
├── cmd/
│   ├── generator/      # sync upstream metadata and generate the static registry
│   ├── translator/     # incrementally fill Chinese descriptions
│   ├── enricher/       # collect rich metadata from structured sources
│   ├── codexgen/       # generate Codex models.json
│   └── modelprobe/     # probe vLLM/SGLang and compatible endpoints
├── data/
│   └── models.json     # cached upstream payload
├── models/             # human-maintained YAML model definitions
├── models_gen.go       # generated file, do not edit manually
└── Taskfile.yml        # go-task entry point
```

## Contributing

PRs are welcome for missing models, better aliases, corrected capabilities, and improved metadata. The common maintenance flow is:

```bash
task generator
task test
```

Useful commands:

```bash
task fmt
task lint
task test
task build
task generator
task translator
task cardextract -- -model qwen/qwen3.6-27b -ai-model <local-model>
task suggestion -- list
task codexsuggest -- -model qwen/qwen3.6-27b
task codexsuggest -- -allowlist codex-models.yaml -report .cache/codex-selection.json
task codexsuggest -- -since 180d -serving-provider openrouter -report .cache/codex-recent.json
task enrich
task codexgen
task modelprobe -- -base-url http://localhost:8000/v1 -model qwen3.6-27b -server vllm
task releasecheck
task sync
```

When changing model metadata, edit `models/**/*.yaml` and regenerate instead of hand-editing `models_gen.go`. See [docs/DEVELOPMENT.md](./docs/DEVELOPMENT.md) for maintainer details and [AGENTS.md](./AGENTS.md) for AI collaboration notes.

Newly discovered or explicitly selected models can use schema v2 to store source-attributed OpenRouter, Hugging Face, reasoning, and Codex metadata. Historical records without a schema version remain v1 and are not bulk-migrated. Run `task enrich -- -model <provider/model>` for explicit enrichment and `task codexgen` to export records with `codex.enabled: true` to the standalone `dist/codex/third-party-models.json`. See the [Codex metadata pipeline](./docs/CODEX_METADATA_PIPELINE.md) for the complete trust model and deployment-probe boundaries.

## Codex third-party model catalog

Codex can load `third-party-models.json`. When the configured model matches a catalog `slug`, Codex no longer uses fallback metadata and the corresponding warning is eliminated. The catalog has no mandatory location; `~/.codex/third-party-models.json` is recommended. Point the Codex user configuration at its absolute path:

```bash
mkdir -p ~/.codex
gh release download v0.6.1 \
  --pattern 'third-party-models.json' \
  --dir ~/.codex
```

```toml
# ~/.codex/config.toml
# Replace this with your absolute path; do not rely on ~ expansion.
model_catalog_json = "/home/your-user/.codex/third-party-models.json"
```

`model_catalog_json` replaces the bundled Codex catalog; it does not append to it. To retain bundled models, export them with the same Codex version and merge locally:

```bash
codex debug models --bundled > bundled-models.json
task codexgen -- -bundled-catalog bundled-models.json -output merged-models.json
```

For a reviewed deployment set, create an allowlist whose slugs exactly match the API model names:

```yaml
models:
  - id: qwen/qwen3.6-27b
    slugs: [qwen3.6-27b]
  - id: deepseek/deepseek-v3.2
    slugs: [deepseek-v3.2]
```

```bash
task codexsuggest -- -allowlist codex-models.yaml -report .cache/codex-selection.json
task suggestion -- list
task suggestion -- -fields codex.enabled,codex.slugs,codex.shell_type,codex.apply_patch_tool_type,codex.supports_parallel_tool_calls,codex.input_modalities apply data/suggestions/<provider>/<model>.codex.json
task codexgen
task codexcheck
```

OpenRouter users can select candidates by their actual upstream creation time:

```bash
task codexsuggest -- -since 180d -serving-provider openrouter \
  -report .cache/codex-recent.json
```

“Recent” only selects candidates. Records that are not schema v2 chat/tool models with text input/output and a positive context window are listed as skipped. For vLLM, SGLang, or another provider, do not assume the OpenRouter ID is the serving slug; convert the candidates into an explicit allowlist first. After review and apply, `task codexgen` packages every eligible enabled model into one catalog. Exporting the entire registry is unsafe because it also contains embedding, reranking, audio, and records whose serving names or tool policies are not confirmed.

The Release catalog also uses [`data/codex/default-open-models.yaml`](./data/codex/default-open-models.yaml) to include these open-weight families by default: Qwen 3.5+, DeepSeek V3/R1+, GLM-5+, and Kimi K2.7+. A model must have a Hugging Face source and pass the static capability checks above. API-only models, routing aliases, and non-agent models are not included by name alone. Default slugs are lowercase, vendor-free model names such as `deepseek-v3.1`. After daily sync discovers a new matching model, `task codexgen` adds it automatically; an explicit `codex.enabled: false` remains authoritative.

## License

Apache 2.0 License

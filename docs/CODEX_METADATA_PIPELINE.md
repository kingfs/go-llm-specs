# Codex Model Metadata Pipeline

## Status

This document defines the implementation plan for enriching newly discovered model records and exporting a Codex-compatible `models.json`. Historical model files remain valid and are not bulk-migrated.

## Goals

- Preserve every existing `models/**/*.yaml` record without requiring migration.
- Store richer, source-attributed metadata for newly discovered or explicitly selected models.
- Collect deterministic metadata from OpenRouter and Hugging Face.
- Record observed serving capabilities separately from model-intrinsic facts.
- Generate a deterministic Codex `ModelsResponse` catalog.
- Probe OpenAI-compatible deployments, including vLLM and SGLang, without requiring a GPU in the normal CI path.
- Keep AI-generated claims advisory until reviewed.

## Non-goals

- Bulk-enrich all historical records.
- Start large models on GitHub-hosted runners.
- Infer runtime compatibility solely from model-card prose.
- Copy OpenAI's private model behavior to unrelated third-party models.
- Treat OpenRouter routing metadata as proof of local vLLM or SGLang behavior.

## Data model

Legacy YAML files without `schema_version` are schema v1. They continue to use the existing fields and must round-trip without gaining new fields.

New or explicitly enriched records use schema v2:

```yaml
schema_version: 2
id: qwen/qwen3.6-27b
name: 'Qwen: Qwen3.6 27B'
provider: Qwen
description: ...
context_length: 262144
max_output: 65536
features:
  - CapChat
  - CapFunctionCall
  - CapJsonMode
aliases:
  - qwen3.6-27b
discovered_at: 2026-07-29T02:00:00Z

upstream:
  openrouter:
    canonical_slug: qwen/qwen3.6-27b-20260422
    supported_parameters: [reasoning, tools, tool_choice]
    fetched_at: 2026-07-29T02:00:00Z
  huggingface:
    id: Qwen/Qwen3.6-27B
    revision: <immutable repository SHA>
    pipeline_tag: image-text-to-text
    model_type: qwen3_5
    architectures: [Qwen3_5ForConditionalGeneration]
    license: apache-2.0
    config_context_length: 262144
    tokenizer_model_max_length: 262144
    processor_class: Qwen3_5Processor
    chat_template_sha256: <digest>
    structured_files: [config.json, preprocessor_config.json, tokenizer_config.json]
    fetched_at: 2026-07-29T02:00:00Z

reasoning:
  supported: true
  mandatory: false
  default_enabled: true
  parser: qwen3
  supported_efforts: []

codex:
  enabled: true
  slugs: [qwen3.6-27b]
  shell_type: unified_exec
  visibility: list
  supports_parallel_tool_calls: false
  supports_reasoning_summary_parameter: false
  support_verbosity: false
  apply_patch_tool_type: freeform
  truncation_policy:
    mode: tokens
    limit: 10000
  effective_context_window_percent: 90
```

Unknown YAML fields must survive load/save cycles. All commands share one registry package rather than defining private, lossy model structs.

## Source priority

1. Human-maintained YAML overrides.
2. A verified deployment probe for runtime behavior.
3. Official provider/model structured APIs.
4. Official Hugging Face or ModelScope structured files.
5. OpenRouter structured metadata.
6. AI-extracted suggestions with evidence; never automatically promoted to verified runtime facts.

Every enrichment result records its source and fetch time. Model facts and deployment observations are kept separate because the same weights may behave differently behind OpenRouter, vLLM, SGLang, or an official API.

## Commands

### Existing generator

`cmd/generator` remains responsible for OpenRouter discovery, YAML synchronization, and `models_gen.go`. Its write path uses the shared registry schema. Existing v1 records keep their current behavior; newly discovered records may be created as v2 when enrichment is requested.

### Enricher

`cmd/enricher` enriches new or explicitly selected records. Initial adapters are:

- OpenRouter structured fields already present in `data/models.json`.
- Hugging Face Hub API plus `config.json`, `tokenizer_config.json`, and
  `preprocessor_config.json` fetched at the API-reported immutable revision.

Selection flags include `-new-only`, `-model`, `-provider`, `-source`, and `-dry-run`. Ambiguous Hugging Face matches are reported rather than written.

### Codex generator

`cmd/codexgen` reads v2 records with `codex.enabled: true`, validates required fields, expands configured serving slugs, and writes a deterministic Codex `ModelsResponse` JSON document.

It supports:

- standalone third-party catalogs;
- optional merge with a catalog captured by `codex debug models --bundled`;
- duplicate-slug rejection;
- a validation-only mode;
- generation metadata in a separate manifest.

The Codex schema version is pinned in project code and documented in the manifest. `used_fallback_model_metadata` is never emitted.

### Codex candidate selection

`cmd/codexsuggest` has three mutually exclusive selection modes. Every mode writes reviewable `pending` suggestions; none directly enables a model:

```bash
# One model. -slug is recommended when the serving name differs from the ID suffix.
task codexsuggest -- -model qwen/qwen3.6-27b -slug qwen3.6-27b

# A deployment-specific list with explicit serving names.
task codexsuggest -- -allowlist codex-models.yaml \
  -report .cache/codex-selection.json

# Models created during the last 180 days, served through OpenRouter.
task codexsuggest -- -since 180d -serving-provider openrouter \
  -report .cache/codex-recent.json
```

The allowlist format is:

```yaml
models:
  - id: qwen/qwen3.6-27b
    slugs:
      - qwen3.6-27b
```

The time selector reads the Unix `created` field in `data/models.json`. It intentionally does not use `discovered_at`, because a historical migration may give many old records the same migration timestamp. `YYYY-MM-DD` and RFC3339 cutoffs are also accepted.

Recent OpenRouter IDs are used as slugs only with the explicit `-serving-provider openrouter` switch. Without it, eligible records are reported as missing a serving slug so an operator can map them for vLLM, SGLang, or a provider API. The report records included and skipped candidates with reasons. Static eligibility requires schema v2, positive context length, chat, function calling, and text input/output capabilities.

### Deployment probe

`cmd/modelprobe` runs non-destructive requests against an already-running OpenAI-compatible endpoint. It tests discovery, text generation, tool calls, parallel tool calls, JSON output, reasoning parameters, and optional image input. It writes an observation report; it does not directly mutate YAML.

Large-model startup is outside normal GitHub-hosted CI. vLLM/SGLang probes run locally or on a self-hosted GPU runner and commit reviewable reports.

## Automation workflow

The deterministic daily path is:

```text
discover new OpenRouter models
  -> create/update structured records
  -> enrich new v2 records from structured sources
  -> validate registry
  -> generate Go registry
  -> generate Codex catalog
  -> tests
  -> reviewable commit or pull request
```

AI-assisted translation and model-card extraction are retryable enrichment jobs, not prerequisites for deterministic generation. They must handle HTTP 429, honor `Retry-After`, checkpoint per batch, and expose partial failure. GitHub Models is suitable for a small daily set; historical backfills should use BYOK or a local model.

### Evidence-backed AI suggestions

Natural-language model cards are processed through a review layer rather than written directly to YAML:

```bash
LLM_BASE_URL=http://localhost:8000/v1 LLM_MODEL=<serving-slug> \
  task cardextract -- -model qwen/qwen3.6-27b -wire-api responses \
  -api-key-env LLM_API_KEY

task suggestion -- list
task suggestion -- show data/suggestions/qwen/qwen3.6-27b.model-card.json
task suggestion -- -fields context_length,description apply \
  data/suggestions/qwen/qwen3.6-27b.model-card.json

task codexsuggest -- -model qwen/qwen3.6-27b

# Or select a reviewed deployment list / recent OpenRouter candidates.
task codexsuggest -- -allowlist codex-models.yaml -report .cache/codex-selection.json
task codexsuggest -- -since 180d -serving-provider openrouter -report .cache/codex-recent.json

# Resume a local backfill for every HF-enriched record. Identical source revisions are reused.
LLM_REASONING_EFFORT=none CARD_EXTRACT_MAX_CHARS=8000 task cardextract-batch
```

`cardextractor` records the immutable Hugging Face revision, source-content SHA-256,
generator model, typed claims, exact evidence quotes, and confidence. Suggestions begin
as `pending`; applying all fields implicitly is forbidden. API keys are read only from the
environment and are never persisted in suggestion documents.

## Compatibility rules

- Missing `schema_version` means v1.
- No command bulk-upgrades v1 records by default.
- Local YAML values remain authoritative when explicitly set.
- Unknown YAML fields survive round trips.
- A Codex export requires explicit `codex.enabled`.
- Runtime capability defaults are conservative.
- `supports_parallel_tool_calls`, Responses-specific reasoning fields, and image-detail support require provider documentation or a probe.
- A catalog's `slug` must match the actual serving API model name. Registry aliases are not automatically Codex aliases.
- Generated output is stable for identical inputs.

## Delivery phases

1. Shared schema, lossless round-trip, and legacy protection.
2. Rich OpenRouter fields and Hugging Face enrichment.
3. Codex catalog generation and validation.
4. OpenAI-compatible deployment probing.
5. Task, CI, documentation, and end-to-end verification.

Each phase is independently tested, committed, and pushed.

## Implemented command examples

```bash
# Enrich only schema-v2/new records from structured sources.
task enrich -- -new-only

# Safely migrate a reviewed batch of historical records; failures remain resumable.
task enrich -- -new-only=false -upgrade-v1 -limit 20 -delay 1s \
  -allowlist models-to-migrate.txt -checkpoint .cache/enrich-checkpoint.json \
  -failure-report .cache/enrich-failures.json

# Explicitly promote and enrich one selected historical record.
task enrich -- -model qwen/qwen3.6-27b

# Generate the standalone third-party catalog and manifest.
task codexgen

# Merge with a catalog captured from a pinned Codex installation.
task codexgen -- -bundled-catalog data/codex/bundled-0.146.0.json

# Probe an already-running local deployment; no model is started by this command.
task modelprobe -- -base-url http://localhost:8000/v1 -model qwen3.6-27b -server vllm -output data/probes/qwen3.6-27b-vllm.json

# Import semantically verified results into one schema-v2 registry record.
task modelprobe -- -base-url http://localhost:8000/v1 -model qwen3.6-27b \
  -server vllm -import-model qwen/qwen3.6-27b
```

The committed `dist/codex/third-party-models.json` is intentionally named as a standalone third-party catalog. Using it directly replaces Codex's bundled catalog. Users who also need bundled entries should capture them with their pinned Codex CLI and use the merge flag; the generator rejects collisions rather than silently overriding either catalog.

For example:

```bash
gh release download v0.6.0 --pattern 'third-party-models.json*'

# Direct use: exact catalog slugs avoid fallback metadata warnings.
# ~/.codex/config.toml:
# model_catalog_json = "/absolute/path/to/third-party-models.json"

# Preserve the bundled entries by generating a version-local merged catalog.
codex debug models --bundled > bundled-models.json
task codexgen -- -bundled-catalog bundled-models.json -output merged-models.json
```

The v0.6.0 asset contains one entry because only `qwen3.6-27b` had an approved Codex policy at release time. A later catalog grows only when suggestions are explicitly reviewed and applied. The full model registry is not a valid Codex catalog: it includes non-chat modalities and records without verified serving slugs or Codex tool policy.

## Acceptance criteria

- Existing v1 YAML files are byte-stable during validation-only and enrichment runs unless explicitly selected for change.
- Rich v2 fields survive generator and translator round trips.
- Hugging Face enrichment can populate Qwen/Qwen3.6-27B from structured API data without AI.
- Codex generation emits a catalog accepted by the pinned schema and contains a configured `qwen3.6-27b` slug without fallback metadata.
- A mock OpenAI-compatible server can exercise every probe branch in unit tests.
- `task lint`, `task test`, `task fmt`, and `task build` pass.

# Model Catalog Architecture

This repository is an append-only historical catalog of facts claimed by model
publishers. It describes models, not individual deployments. A serving provider
may expose a smaller context window or a different capability set; those
deployment constraints are outside this registry.

## Trust model

Facts are selected in this order:

1. reviewed human-maintained YAML;
2. the publisher's structured API or official documentation;
3. a repository owned by the publisher's configured Hugging Face or ModelScope organization;
4. OpenRouter structured metadata;
5. evidence-backed AI suggestions, only after explicit review.

OpenRouter remains the broad default discovery feed. It is not the canonical
owner of model names or publisher specifications. Existing model YAML is never
removed merely because an upstream feed stops listing it.

OpenRouter serving variants are not model identities. Suffixes such as
`:batch`, `:free`, and `:thinking` are folded into the underlying model and
retained as runtime aliases plus `identifiers.openrouter` values. Named modes
such as `-pro` or `-fast` are folded only when the upstream description
explicitly says they use the same underlying model or have identical
capabilities; independently published models such as `o3-pro` remain separate.

Two further classes of upstream records are provider or inference packaging
rather than model identities, so they are never compiled into `models_gen.go`
or the public catalog:

- **Routing aliases** are OpenRouter's `~` namespace. They move between
  checkpoints and have no stable identity of their own. When the upstream feed
  names the model an alias currently resolves to, the alias is folded into that
  model as an alias and an `identifiers.openrouter` value, and the standalone
  record is removed; otherwise the record is dropped. A model the publisher
  itself names `-latest`, such as `openai/gpt-chat-latest`, is a real model and
  is recorded as-is.
- **Draft heads** are speculative-decoding modules attached to another model's
  checkpoint, such as DSpark, DFlash, EAGLE-3, and MTP heads. They are not
  standalone language models. Their records stay in `models/` for provenance,
  but they are excluded from every compiled artifact.

The classification lives in `internal/registry/variant.go`. A record can make
the decision explicit with the optional `kind` field:

```yaml
kind: model            # model | serving-artifact | draft-head | adapter | quantization
```

An explicit `kind` always wins over the ID and description patterns, so a
reviewed record can override a false positive. `serving-artifact`, `draft-head`
and `adapter` are excluded from every compiled artifact; an empty `kind` means
`model`. `quantization` is reported by `task catalog-doctor` for review but is
still compiled. The generator, the public catalog, Hugging Face candidate
materialization and the Codex exporter all use `IsCompiledKind`, so a serving
kind can never reach `models_gen.go`, `catalog.json` or a runnable Codex entry.

## Publisher catalog

`providers/*.yaml` defines canonical publisher names, official entry points and
organizations that may be queried deterministically. A Hugging Face repository
is treated as official only when its organization is explicitly configured in
the corresponding publisher file.

Each publisher may declare who owns its model identity:

```yaml
identity:
  strategy: publisher             # publisher | aggregator (default)
  canonical_prefix: deepseek      # optional stable ID prefix
  require_corroboration: true     # optional; holds new discoveries as candidates
```

`publisher` means the provider's official API or organization repositories are
authoritative and OpenRouter is only a discovery and fallback feed. It requires
at least one authoritative source (`organizations.huggingface`,
`organizations.modelscope`, or `official.api`). `aggregator` is the default and
means OpenRouter is authoritative because the provider has no first-party
machine-readable source.

`require_corroboration` is opt-in and only valid together with `publisher`. When
set, a brand-new record discovered through OpenRouter is written as
`lifecycle: candidate` until an official organization repository or official
link confirms it, and `task catalog-promote` will not activate it before then.
Candidate records still appear on the public catalog page, so a genuinely
published model is never hidden; only its compiled identity waits for a
first-party source. Publishers that also ship closed models leave the flag off
because they have no repository to corroborate against.

The catalog intentionally starts with major publishers. `task catalog-audit`
lists long-tail publisher strings that still need a reviewed provider record;
the tool never invents official URLs. `task catalog-doctor` writes a read-only
`data/catalog-doctor.json` listing every record by `kind` plus routing aliases,
draft heads, quantization candidates, pretrained `-base` variants, resolved
identity sources, records whose identity is not yet corroborated, and publisher
models that still lack an organization repository.

## Model records

`models/**/*.yaml` remains the human-readable source of truth. New optional
fields separate model identity and evidence from discovery metadata:

```yaml
developer: qwen
links:
  official: https://qwen.ai/...
  model_card: https://huggingface.co/Qwen/...
identifiers:
  official: [Qwen3-32B]
  huggingface: [Qwen/Qwen3-32B]
  openrouter: [qwen/qwen3-32b]
provenance:
  context_length:
    source: official_model_card
    url: https://huggingface.co/Qwen/...
```

Top-level fields remain convenient compiled values. `provenance` explains why
a value was selected without turning each value into a deeply nested object.
It is audit metadata, not permission to overwrite the value: once a top-level
field exists in YAML, every automatic source treats it as immutable.

A model may also pin where its identity came from. This is the highest-priority
override and is only needed when the automatic resolution is wrong or when a
closed model must be accepted without a first-party repository:

```yaml
identity:
  source: manual        # manual | official | official_huggingface | official_modelscope | openrouter
  verified: true
```

When the block is absent, `internal/identity` resolves the origin in this order:
an explicit block, then a repository inside one of the publisher's declared
official organizations, then an official identifier or link (`official_*` or
`official`), and finally OpenRouter. OpenRouter is authoritative for aggregator
publishers and is reported unverified for publisher-strategy publishers until a
first-party source corroborates it. Publishers that declare an official
organization use publisher strategy; the five without one (Anthropic, OpenAI,
OpenRouter, Perplexity, xAI) keep the aggregator default. Corroboration gating
is enabled for the publishers whose releases are open-weight by default.

## Incremental workflow

```text
OpenRouter discovery ─┐
                     ├─> compare with historical YAML -> enrich -> AI suggestions -> review
official HF orgs ────┘
                                                        -> generate artifacts
```

- `task generator` discovers through OpenRouter, merges missing fields, and
  preserves all local records and explicit overrides. For publishers that
  require corroboration it writes a new discovery as `lifecycle: candidate`
  instead of active until a first-party source confirms its identity.
- `task catalog-discover` paginates subscribed official Hugging Face organizations,
  preserves a durable candidate queue in `data/catalog-discovery.json`, applies
  exact identity matches, and materializes at most five eligible official
  repositories per run as `lifecycle: candidate` YAML records.
- Candidate records are excluded from `models_gen.go` until structured enrichment
  and evidence-backed extraction provide the required facts. `task catalog-promote`
  activates only ready records.
- `task enrich -- -new-only` means source metadata is actually missing; it does
  not rescan every schema-v2 record.
- model-card AI extraction is bounded to a small incremental batch and produces
  suggestions. `suggestion auto-apply` accepts only high-confidence claims from
  a pinned model card owned by a configured official organization, only for
  fields that are currently empty; existing facts are never overwritten.
- `task catalog-audit` writes deterministic coverage and attribution gaps to
  `data/catalog-audit.json`.
- `task catalog-doctor` writes a deterministic record-kind and identity-gap
  report to `data/catalog-doctor.json`; `task catalog-doctor-check` gates CI on
  that report being current.

The initial historical backfill uses the same commands with reviewed allowlists.
This makes the one-time work resumable and ensures subsequent GitHub Actions runs
exercise exactly the same path on a much smaller delta.

Generation is deterministic: network acquisition happens once, and final Go
code generation reads the immutable cache without another upstream request.
CI checks both `models_gen.go` and the local audit report for drift. Context and
maximum-output corrections are release-worthy model facts.

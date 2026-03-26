# go-llm-specs

A static, type-safe LLM model metadata registry for Go.

[English](./README_EN.md) | [中文](./README.md)

[![Daily Model Sync](https://github.com/kingfs/go-llm-specs/actions/workflows/daily-update.yml/badge.svg)](https://github.com/kingfs/go-llm-specs/actions/workflows/daily-update.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/kingfs/go-llm-specs.svg)](https://pkg.go.dev/github.com/kingfs/go-llm-specs)

## What This Project Does

`go-llm-specs` maintains a compiled-in registry of LLM metadata for Go applications.

- The primary upstream source is currently OpenRouter.
- Local `models/**/*.yaml` files hold overrides, additions, aliases, and Chinese descriptions.
- The final merged registry is emitted into `models_gen.go`.
- Runtime lookups are fully static and require no network I/O.
- `cmd/translator` incrementally fills missing `description_cn` fields under `models/`.

For AI-oriented repository guidance, see [AGENTS.md](./AGENTS.md). For maintainer workflow details, see [docs/DEVELOPMENT.md](./docs/DEVELOPMENT.md).

## Main Features

- `Get` / `GetMany`: exact lookup by model ID or alias
- `Query`: chainable capability filtering backed by bitmasks
- `Search`: fuzzy search across IDs, names, and aliases
- Local override support: explicitly set YAML fields win over upstream values
- Code generation: the merged registry can be embedded directly into downstream Go binaries

## Installation

```bash
go get github.com/kingfs/go-llm-specs
```

## Quick Example

```go
package main

import (
	"fmt"

	llmspecs "github.com/kingfs/go-llm-specs"
)

func main() {
	if m, ok := llmspecs.Get("gpt4t"); ok {
		fmt.Println(m.Name(), m.ContextLength())
	}
}
```

### Query Example

```go
models := llmspecs.Query().
	Provider("Anthropic").
	Has(llmspecs.ModalityImageIn).
	Has(llmspecs.CapFunctionCall).
	List()
```

### Search Example

```go
results := llmspecs.Search("claude", 5)
```

See [examples/basic/main.go](./examples/basic/main.go) for a runnable example.

## Repository Layout

```text
.
├── cmd/
│   ├── generator/      # fetch upstream data and generate the static registry
│   └── translator/     # incrementally translate model descriptions
├── data/
│   └── models.json     # cached upstream payload
├── docs/
├── models/             # human-maintained YAML model files
├── models_gen.go       # generated file, do not edit manually
└── Taskfile.yml        # go-task entry point
```

## Development Commands

The repository uses `go-task` as the main command entry point.

```bash
task fmt
task lint
task test
task build
task generator
task translator
task releasecheck
task sync
```

Pass extra CLI flags after `--`:

```bash
task generator -- -fetch-only
task translator -- -provider OpenAI -limit 20
task run -- go test ./...
```

## Generator

`cmd/generator` is responsible for:

1. Fetching the upstream model list and descriptions.
2. Merging local overrides from `models/`.
3. Writing updated YAML files back to `models/`.
4. Emitting `models_gen.go` for downstream Go projects.

Common commands:

```bash
task generator
task generator -- -fetch-only
task generator -- -sync-registry=false
```

Useful flags:

- `-source`: upstream source, currently `openrouter`
- `-api-url`: upstream models endpoint
- `-models-dir`: local YAML directory
- `-cache-path`: raw upstream JSON cache path
- `-output-go`: generated Go file path
- `-fetch-only`: refresh upstream cache only

## Translator

By default, `cmd/translator` only targets YAML files that:

- have `description`
- do not yet have `description_cn`

Common commands:

```bash
export LLM_API_KEY="sk-..."
task translator
task translator -- -provider OpenAI -limit 50
task translator -- -dry-run -id-prefix qwen/
```

Environment variables:

- `LLM_API_KEY`: required
- `LLM_BASE_URL`: optional, defaults to `https://api.openai.com/v1`
- `LLM_MODEL`: optional, defaults to `gpt-4o-mini`

## Release Check

`cmd/releasecheck` compares the latest tag with the current `models_gen.go` and decides whether a new release is warranted.

Current release policy:

- These changes trigger a release:
  - added models
  - removed models
  - changes to `name`, `provider`, `description`, `description_cn`, `features`, or `aliases`
- These changes do not trigger a release by themselves:
  - `context_length`
  - `max_output`

Common commands:

```bash
task releasecheck
task releasecheck -- -base-ref v0.3.44 -format json
```

## Local Override Example

```yaml
id: openai/text-embedding-3-large
name: "OpenAI: Text Embedding 3 Large"
provider: OpenAI
description_cn: "OpenAI's most powerful embedding model."
features:
  - CapEmbedding
  - ModalityTextIn
context_length: 8192
aliases:
  - text-embedding-3-large
```

See [capability.go](./capability.go) for supported capability constants.

## Recommended Workflow

1. Run `task generator` to refresh upstream data and local registry files.
2. Run `task translator` if Chinese descriptions need to be filled in.
3. Run `task generator` again so translated fields are baked into `models_gen.go`.
4. Run `task releasecheck` to see whether the change should publish a release.
5. Run `task ci` before submitting changes.

## License

Apache 2.0 License

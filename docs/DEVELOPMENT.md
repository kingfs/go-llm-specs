# Development Workflow

## Core Data Flow

1. `cmd/generator` fetches upstream model metadata from OpenRouter.
2. The generator merges upstream data with local YAML overrides in `models/`.
3. The merged registry is emitted into `models_gen.go`.
4. `cmd/translator` incrementally fills missing `description_cn` values inside `models/`.
5. After translation, rerun the generator so the new Chinese descriptions are embedded in `models_gen.go`.

## Local Commands

The repository uses `go-task` as the main developer entry point.

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

Pass additional flags to the Go CLIs after `--`:

```bash
task generator -- -fetch-only
task generator -- -models-dir ./models -output-go ./models_gen.go
task translator -- -provider OpenAI -limit 50
task translator -- -dry-run -id-prefix qwen/
```

## Generator Semantics

- Default upstream source: OpenRouter.
- Default behavior: fetch upstream data, update `models/`, then regenerate `models_gen.go`.
- Local YAML fields override upstream values when explicitly set.
- If upstream fetch fails, the generator can fall back to `data/models.json`.

## Translator Semantics

- Default behavior: translate only files with `description` present and missing `description_cn`.
- Selection can be narrowed by `-provider`, `-id-prefix`, and `-limit`.
- `-dry-run` shows which files would be translated without calling the LLM API.

## Generated Files

- `models_gen.go`: generated, do not edit manually.
- `data/models.json`: cached upstream payload, useful for debugging or offline fallback.

## Release Gating

- `cmd/releasecheck` compares the latest git tag with the current `models_gen.go`.
- It triggers a release for model additions/removals and significant metadata changes.
- It ignores `context_length` and `max_output` changes when they are the only differences.
- This is the source of truth used by GitHub Actions to decide whether a new tag should be published.

## Validation

Run this before sending a change:

```bash
task ci
```

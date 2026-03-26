# AGENTS.md

This repository is a static LLM model metadata registry for Go. AI agents should treat the repo as a data compiler:

- Upstream source: `cmd/generator` currently fetches model metadata from OpenRouter.
- Human-maintained source of truth: `models/**/*.yaml`.
- Generated artifact: `models_gen.go`.
- Cached upstream payload: `data/models.json`.

## What To Read First

1. `README.md` or `README_EN.md` for project goals and user-facing workflows.
2. `Taskfile.yml` for the supported development commands.
3. `cmd/generator/main.go` for upstream sync and code generation behavior.
4. `cmd/translator/main.go` for incremental translation behavior.
5. `cmd/releasecheck/main.go` for release gating semantics.
6. `capability.go`, `model.go`, and `registry.go` for runtime API and feature semantics.

## File Ownership Rules

- Edit `models/**/*.yaml` when adjusting model metadata, aliases, or translated descriptions.
- Edit `cmd/generator/main.go` when changing upstream fetch, merge, or codegen behavior.
- Edit `cmd/translator/main.go` when changing translation batching, selection, or persistence behavior.
- Do not hand-edit `models_gen.go`; regenerate it with `task generator`.
- `data/models.json` is cache/debug output and may be refreshed by the generator.

## Development Commands

- `task fmt`
- `task lint`
- `task test`
- `task build`
- `task generator`
- `task translator`
- `task sync`
- `task releasecheck`

Pass extra flags after `--`, for example:

- `task generator -- -fetch-only`
- `task translator -- -provider OpenAI -limit 20`

## Important Behavior Constraints

- Local YAML overrides are authoritative when a field is explicitly set.
- `description_cn` is translated incrementally; untranslated files are the default target set.
- Releases are gated by `cmd/releasecheck`, not by raw file diffs.
- Aliases are case-insensitive at runtime.
- Capability names used in YAML must match the Go constants defined in `capability.go`.
- Keep docs aligned with the actual CLI flags and task commands.

## Typical Agent Workflows

### Add or fix model metadata

1. Edit the relevant `models/**/*.yaml` file.
2. Run `task generator`.
3. Run `task test`.

### Refresh upstream registry

1. Run `task generator`.
2. Review changes in `models/`, `data/models.json`, and `models_gen.go`.
3. Run `task test`.

### Translate missing Chinese descriptions

1. Ensure `LLM_API_KEY` and optional `LLM_BASE_URL` / `LLM_MODEL` are set.
2. Run `task translator`.
3. Run `task generator` to bake translations into `models_gen.go`.
4. Run `task releasecheck` to confirm this change should produce a tag.

## Review Checklist

- Did you accidentally hand-edit generated files instead of updating source inputs?
- Do README and AGENTS still match the actual command surface?
- If generator behavior changed, are merge semantics and deterministic output still preserved?
- If translator behavior changed, does it remain incremental by default?

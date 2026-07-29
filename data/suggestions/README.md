# Model metadata suggestions

Files in this directory are evidence-backed, reviewable suggestions. They are not part of
the runtime registry and have not been applied to `models/**/*.yaml`. Use `task suggestion`
to inspect and explicitly accept selected fields.

## Initial local backfill

- Generated with `qwen3.6-27b` through an OpenAI-compatible Responses API.
- Input was limited to the first 8,000 characters of each immutable Hugging Face Model Card.
- 187 of 190 HF-enriched registry records produced at least one schema-valid claim.
- Identical Model Card revision/SHA pairs reused previously validated claims.
- Invalid individual claims were discarded rather than coerced.

No valid allowed-field claims were produced for:

- `meta-llama/llama-3.1-70b-instruct`
- `meta-llama/llama-3.1-8b-instruct`
- `meta-llama/llama-guard-4-12b`

These records can be retried later with section-based extraction. API credentials are not
stored in suggestion documents or repository files.

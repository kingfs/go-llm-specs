#!/usr/bin/env bash
set -u

limit="${CARD_EXTRACT_LIMIT:-0}"
delay="${CARD_EXTRACT_DELAY:-1}"
checkpoint="${CARD_EXTRACT_CHECKPOINT:-.cache/cardextract-completed.txt}"
failures="${CARD_EXTRACT_FAILURES:-.cache/cardextract-failures.txt}"
models_dir="${MODELS_DIR:-models}"
shard_index="${CARD_EXTRACT_SHARD_INDEX:-0}"
shard_count="${CARD_EXTRACT_SHARD_COUNT:-1}"

mkdir -p "$(dirname "$checkpoint")" "$(dirname "$failures")"
touch "$checkpoint" "$failures"
processed=0
ordinal=0
while IFS= read -r file; do
  current=$ordinal
  ordinal=$((ordinal + 1))
  if [ $((current % shard_count)) -ne "$shard_index" ]; then continue; fi
  model_id="$(awk '/^id:/{print $2; exit}' "$file")"
  [ -n "$model_id" ] || continue
  grep -Fqx "$model_id" "$checkpoint" && continue
  suggestion_path="data/suggestions/${model_id//:/_}.model-card.json"
  revision="$(awk '/^  huggingface:/{found=1; next} found && /^    revision:/{print $2; exit} found && /^  [a-z]/{exit}' "$file")"
  if [ -n "$revision" ] && [ -f "$suggestion_path" ] && \
    [ "$(jq -r '.source.revision // ""' "$suggestion_path")" = "$revision" ]; then
    printf '%s\n' "$model_id" >> "$checkpoint"
    continue
  fi
  if go run ./cmd/cardextractor -models-dir "$models_dir" -model "$model_id" \
    -api-base "${LLM_BASE_URL:?LLM_BASE_URL is required}" \
    -api-key-env "${LLM_API_KEY_ENV:-LLM_API_KEY}" -ai-model "${LLM_MODEL:?LLM_MODEL is required}" \
    -wire-api "${LLM_WIRE_API:-responses}" -reasoning-effort "${LLM_REASONING_EFFORT:-low}" \
    -max-chars "${CARD_EXTRACT_MAX_CHARS:-8000}" -skip-current; then
    printf '%s\n' "$model_id" >> "$checkpoint"
  else
    printf '%s\n' "$model_id" >> "$failures"
  fi
  processed=$((processed + 1))
  if [ "$limit" -gt 0 ] && [ "$processed" -ge "$limit" ]; then break; fi
  sleep "$delay"
done < <(find "$models_dir" -type f -name '*.yaml' -exec grep -l '^  huggingface:' {} + | sort)
echo "card extraction batch complete: processed=$processed checkpoint=$checkpoint failures=$failures"

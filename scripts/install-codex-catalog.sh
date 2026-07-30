#!/bin/sh
set -eu

repo="${GO_LLM_SPECS_REPO:-kingfs/go-llm-specs}"
codex_bin="${CODEX_BIN:-codex}"
codex_dir="${CODEX_HOME:-${HOME}/.codex}"
config_file="${CODEX_CONFIG_FILE:-${codex_dir}/config.toml}"
catalog_file="${CODEX_CATALOG_FILE:-${codex_dir}/models.json}"
release_url="${GO_LLM_SPECS_CATALOG_URL:-https://github.com/${repo}/releases/latest/download/third-party-models.json}"

usage() {
	printf '%s\n' "Usage: install-codex-catalog.sh [--config PATH] [--output PATH] [--catalog-url URL]"
}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--config) config_file=$2; shift 2 ;;
		--output) catalog_file=$2; shift 2 ;;
		--catalog-url) release_url=$2; shift 2 ;;
		-h|--help) usage; exit 0 ;;
		*) printf 'unknown argument: %s\n' "$1" >&2; usage >&2; exit 2 ;;
	esac
done

for command_name in curl python3 "$codex_bin"; do
	if ! command -v "$command_name" >/dev/null 2>&1; then
		printf 'required command not found: %s\n' "$command_name" >&2
		exit 1
	fi
done

temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/go-llm-specs-codex.XXXXXX")
trap 'rm -rf "$temp_dir"' EXIT HUP INT TERM
bundled_file="${temp_dir}/bundled-models.json"
third_party_file="${temp_dir}/third-party-models.json"
merged_file="${temp_dir}/models.json"

printf 'Exporting the bundled catalog from %s...\n' "$("$codex_bin" --version)"
"$codex_bin" debug models --bundled > "$bundled_file"
printf 'Downloading %s...\n' "$release_url"
curl -fL --retry 3 --connect-timeout 15 "$release_url" -o "$third_party_file"

# model_catalog_json overrides the bundled catalog, so retain both groups. Schema
# normalization is driven by the installed CLI's parser below.
python3 - "$bundled_file" "$third_party_file" "$merged_file" <<'PY'
import json
import sys

bundled_path, additions_path, output_path = sys.argv[1:]

def load(path):
    with open(path, encoding="utf-8") as handle:
        document = json.load(handle)
    models = document.get("models")
    if not isinstance(models, list) or not models:
        raise SystemExit(f"catalog has no models: {path}")
    return models

bundled = load(bundled_path)
additions = load(additions_path)
for model in bundled:
    if not isinstance(model, dict) or not model.get("slug"):
        raise SystemExit("bundled catalog contains an invalid model entry")

seen = {model["slug"].casefold() for model in bundled}
normalized = []
for model in additions:
    if not isinstance(model, dict) or not model.get("slug"):
        raise SystemExit("third-party catalog contains an invalid model entry")
    slug_key = model["slug"].casefold()
    if slug_key in seen:
        raise SystemExit(f'duplicate catalog slug while merging: {model["slug"]}')
    seen.add(slug_key)
    normalized.append(dict(model))

models = sorted([*bundled, *normalized], key=lambda item: item["slug"].casefold())
with open(output_path, "w", encoding="utf-8") as handle:
    json.dump({"models": models}, handle, ensure_ascii=False, indent=2)
    handle.write("\n")
print(f"Merged {len(bundled)} bundled and {len(normalized)} third-party models.")
PY

printf 'Validating the merged catalog with the installed Codex CLI...\n'
attempt=0
while :; do
	if validation_output=$("$codex_bin" debug models -c "model_catalog_json=\"${merged_file}\"" 2>&1); then
		break
	fi
	# The backticks in this parser expression are literal Codex error delimiters.
	# shellcheck disable=SC2016
	missing_field=$(printf '%s\n' "$validation_output" | sed -n 's/.*missing field `\([^`]*\)`.*/\1/p' | head -n 1)
	if [ -z "$missing_field" ] || [ "$attempt" -ge 20 ]; then
		printf '%s\n' "$validation_output" >&2
		exit 1
	fi
	attempt=$((attempt + 1))
	printf 'Adding required local schema field: %s\n' "$missing_field"
	python3 - "$bundled_file" "$merged_file" "$missing_field" <<'PY'
import json
import sys

bundled_path, merged_path, field = sys.argv[1:]
with open(bundled_path, encoding="utf-8") as handle:
    bundled = json.load(handle)["models"]
with open(merged_path, encoding="utf-8") as handle:
    merged = json.load(handle)

examples = [model[field] for model in bundled if field in model and model[field] is not None]
if not examples:
    raise SystemExit(f"local bundled catalog has no example for required field: {field}")
example = examples[0]
if isinstance(example, bool):
    default = False
elif isinstance(example, (int, float)):
    default = 0
elif isinstance(example, str):
    default = ""
elif isinstance(example, list):
    default = []
elif isinstance(example, dict):
    default = {}
else:
    default = None

for model in merged["models"]:
    model.setdefault(field, default)
with open(merged_path, "w", encoding="utf-8") as handle:
    json.dump(merged, handle, ensure_ascii=False, indent=2)
    handle.write("\n")
PY
done

mkdir -p "$(dirname "$catalog_file")" "$(dirname "$config_file")"
install -m 0644 "$merged_file" "${catalog_file}.tmp"
mv "${catalog_file}.tmp" "$catalog_file"

if [ -f "$config_file" ]; then
	cp -p "$config_file" "${config_file}.bak"
else
	: > "$config_file"
fi

python3 - "$config_file" "$catalog_file" <<'PY'
import json
import re
import sys

config_path, catalog_path = sys.argv[1:]
with open(config_path, encoding="utf-8") as handle:
    text = handle.read()

assignment = f"model_catalog_json = {json.dumps(catalog_path)}"
lines = text.splitlines(keepends=True)
top_level_end = next((i for i, line in enumerate(lines) if re.match(r"\s*\[", line)), len(lines))
matches = [i for i, line in enumerate(lines[:top_level_end]) if re.match(r"\s*model_catalog_json\s*=", line)]
if len(matches) > 1:
    raise SystemExit(f"multiple top-level model_catalog_json entries in {config_path}")
if matches:
    ending = "\n" if lines[matches[0]].endswith("\n") else ""
    lines[matches[0]] = assignment + ending
else:
    prefix = assignment + "\n"
    if lines and lines[0].strip():
        prefix += "\n"
    lines.insert(0, prefix)

with open(config_path, "w", encoding="utf-8") as handle:
    handle.write("".join(lines))
PY

printf 'Installed merged catalog: %s\n' "$catalog_file"
printf 'Updated Codex config: %s\n' "$config_file"
if [ -f "${config_file}.bak" ]; then
	printf 'Previous config backup: %s\n' "${config_file}.bak"
fi

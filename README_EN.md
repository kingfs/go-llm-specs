# go-llm-specs

The most comprehensive, fastest, and type-safe LLM static metadata registry in the Golang ecosystem.

[English](./README_EN.md) | [中文](./README.md)

[![Daily Model Sync](https://github.com/kingfs/go-llm-specs/actions/workflows/daily-update.yml/badge.svg)](https://github.com/kingfs/go-llm-specs/actions/workflows/daily-update.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/kingfs/go-llm-specs.svg)](https://pkg.go.dev/github.com/kingfs/go-llm-specs)

## 🌟 Project Vision

*   **Single Source of Truth**: Primary data sourced from OpenRouter, combined with local overrides and additions in the `models/` directory.
*   **Zero Runtime IO**: All data is compiled into the binary—zero network latency for queries.
*   **High Performance**: Utilizes Bitmasks and efficient indexing for nanosecond-level lookups (including new types like Embedding/Reranker).
*   **Self-Updating**: Powered by GitHub Actions for fully automated daily updates and releases.

## 🚀 Benchmarks

Tested on Apple M3 Pro. All operations are nanosecond-level with near-zero memory allocation:

| Operation | Performance | Allocation |
| :--- | :--- | :--- |
| `Get(ID)` (Exact Lookup) | **~6 ns/op** | 0 B/op |
| `Get(Alias)` (Alias Lookup) | **~24 ns/op** | 0 B/op |
| `GetMany([]string)` (Batch) | **~156 ns/op** | 80 B/op (1 alloc) |
| `Search(query, limit)` (Fuzzy) | **~35 µs/op** | ~11 KB/op |
| `Query().Has(...).List()` | **~2000 ns/op** | 0 B/op |

## 📦 Installation

```bash
go get github.com/kingfs/go-llm-specs
```

## 🛠 Usage Examples

### 1. Basic Get

Supports model retrieval by ID or alias:

```go
package main

import (
    "fmt"
    "github.com/kingfs/go-llm-specs"
)

func main() {
    // Get model by alias
    if m, ok := llmspecs.Get("gpt4t"); ok {
        fmt.Printf("Model: %s\n", m.Name())
        fmt.Printf("Context Length: %d\n", m.ContextLength())
    }
}
```

### 2. Batch Get (GetMany)

Efficiently retrieve multiple models, automatically skipping those that don't exist:

```go
names := []string{"gpt4t", "qwen3-32b", "non-existent"}
models := llmspecs.GetMany(names)
for _, m := range models {
    fmt.Printf("- Found: %s\n", m.Name())
}
```

### 3. Chainable Query

Fast bitmask-based filtering to find models matching specific criteria:

```go
package main

import (
    "fmt"
    "github.com/kingfs/go-llm-specs"
)

func main() {
    // Find Anthropic models that support Vision and Function Calling
    models := llmspecs.Query().
        Provider("Anthropic").
        Has(llmspecs.ModalityImageIn).
        Has(llmspecs.CapFunctionCall).
        List()

    for _, m := range models {
        fmt.Printf("- %s: %s\n", m.ID(), m.Description())
    }
}
```

### 3. Fuzzy Search

When you are unsure of the full model name, use the search feature to get results ranked by relevance. The search logic matches against IDs, Names, and Aliases with the following weights:

1.  **Exact Match** (ID: 100 pts, Name: 90 pts)
2.  **Exact Alias Match** (80 pts)
3.  **Prefix Match** (ID: 50 pts, Name: 40 pts)
4.  **Substring Match** (ID: 20 pts, Name: 10 pts)
5.  **Alias Substring Match** (15 pts)

```go
// Search models containing "claude"
results := llmspecs.Search("claude", 5)
for _, m := range results {
    fmt.Printf("Found: %s (%s)\n", m.Name(), m.ID())
}
```

### 4. Aliases

To simplify lookups, the project provides aliases via:
- **Manual Overrides**: Manually defined in the `models/` directory (highest priority).
- **Auto-Generation**: If a model ID suffix (e.g., `qwen3-32b` from `qwen/qwen3-32b`) is unique among all models, the generator automatically assigns it as an alias.

```go
// Lookup using an auto-generated unique suffix alias
m, ok := llmspecs.Get("qwen3-32b")
```

Check the [examples](examples) directory for more details.

## 📂 Custom Registry & Overrides

The project supports adding new models or overriding existing ones via the `models/` directory in the root. The generator recursively scans all `.yaml` files in his folder.

### 1. Directory Structure
It is recommended to organize files by provider:
```
models/
├── openai/
│   ├── gpt-4o.yaml
│   └── text-embedding-3.yaml
├── anthropic/
│   └── claude-3-opus.yaml
└── custom-provider.yaml
```

### 2. Registration Rules
- **New Models**: Create a YAML file with a unique `id` (e.g., `my-provider/my-model`).
- **Overrides**: Use the same `id` as in OpenRouter; fields in the YAML will override the API-sourced data.

### 3. YAML Format Example
```yaml
id: openai/text-embedding-3-large
name: "OpenAI: Text Embedding 3 Large"
provider: OpenAI
description_cn: "OpenAI's most powerful embedding model."
features:
  - CapEmbedding    # Support for new model types
  - ModalityTextIn
context_length: 8192
aliases:
  - text-embedding-3-large
```

For supported features, check `capability.go`.

## 🤖 How it Works

1.  **Generator (cmd/generator)**: Automatically fetches the full model list from OpenRouter and recursively loads all local definitions from `models/`, then merges them.
2.  **Translator (cmd/translator)**: Optionally uses LLMs to translate missing Chinese descriptions in `models/`.
3.  **Local Registry (models/)**: Stores manual corrections, aliases, translations, and models missing from the API (like Embedding/Reranker).
4.  **Code Gen**: Automatically generates `models_gen.go`, hard-coding all data into static maps.
5.  **Auto Update**: Uses GitHub Actions to sync daily and publish new versions using SemVer.

## 📝 Running Tools Locally

### Generator
```bash
go run cmd/generator/main.go
```

### Translator
Requires `LLM_API_KEY`:
```bash
export LLM_API_KEY="sk-..."
go run cmd/translator/main.go
```

## 📄 License

Apache 2.0 License

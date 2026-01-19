# go-llm-specs

The most comprehensive, fastest, and type-safe LLM static metadata registry in the Golang ecosystem.

[![Daily Model Sync](https://github.com/kingfs/go-llm-specs/actions/workflows/daily-update.yml/badge.svg)](https://github.com/kingfs/go-llm-specs/actions/workflows/daily-update.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/kingfs/go-llm-specs.svg)](https://pkg.go.dev/github.com/kingfs/go-llm-specs)

## 🌟 Project Vision

*   **Single Source of Truth**: Primary data sourced from OpenRouter, combined with community-maintained manual overrides.
*   **Zero Runtime IO**: All data is compiled into the binary—zero network latency for queries.
*   **High Performance**: Utilizes Bitmasks and efficient indexing for nanosecond-level lookups.
*   **Self-Updating**: Powered by GitHub Actions for fully automated daily updates and releases.

## 🚀 Benchmarks

Tested on Apple M3 Pro. All operations are nanosecond-level with near-zero memory allocation:

| Operation | Performance | Allocation |
| :--- | :--- | :--- |
| `Get(ID)` (Exact Lookup) | **~6 ns/op** | 0 B/op |
| `Get(Alias)` (Alias Lookup) | **~24 ns/op** | 0 B/op |
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
        fmt.Printf("Input Price: $%f / 1k tokens\n", m.PriceInput())
    }
}
```

### 2. Chainable Query

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

When you are unsure of the full model name, use the search feature to get ranked results:

```go
// Search models containing "claude"
results := llmspecs.Search("claude", 5)
for _, m := range results {
    fmt.Printf("Found: %s (%s)\n", m.Name(), m.ID())
}
```

Check the [examples](file:///Users/kingfs/go/src/github.com/kingfs/go-llm-specs/examples) directory for more details.

## 🤖 How it Works

1.  **Generator (cmd/generator)**: Automatically fetches the full model list from OpenRouter daily.
2.  **Overrides (data/overrides.yaml)**: Allows manual correction of aliases, addition of Chinese descriptions, and normalization of Provider names.
3.  **Code Gen**: Automatically generates `models_gen.go`, hard-coding all data into static maps.
4.  **Auto Update**: Uses GitHub Actions to sync daily and publish new versions using SemVer.

## 📝 Running the Generator Locally

If you wish to refresh data manually:

```bash
# Requires internet access
go run cmd/generator/main.go
```

## 📄 License

Apache 2.0 License

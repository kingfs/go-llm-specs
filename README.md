# go-llm-specs

构建 Golang 生态中最全、最快、类型安全的 LLM 静态元数据中心。

[![Daily Model Sync](https://github.com/kingfs/go-llm-specs/actions/workflows/daily-update.yml/badge.svg)](https://github.com/kingfs/go-llm-specs/actions/workflows/daily-update.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/kingfs/go-llm-specs.svg)](https://pkg.go.dev/github.com/kingfs/go-llm-specs)

## 🌟 项目愿景

*   **Single Source of Truth**: 以 OpenRouter 为主数据源，结合社区维护的修正文件。
*   **Zero Runtime IO**: 所有数据编译进二进制，查询零网络延迟。
*   **High Performance**: 利用 Bitmask（位掩码）处理模型能力，纳秒级查询。
*   **Self-Updating**: 利用 GitHub Actions 实现“无人值守”的自动更新与版本发布。

## 🚀 性能基准

在 Apple M3 Pro 上测试，所有操作均为纳秒级且几乎零内存分配：

| 操作 | 性能 | 内存分配 |
| :--- | :--- | :--- |
| `Get(ID)` (精确查找) | **~6 ns/op** | 0 B/op |
| `Get(Alias)` (别名查找) | **~24 ns/op** | 0 B/op |
| `Query().Has(...).List()` | **~2000 ns/op** | 0 B/op |

## 📦 安装

```bash
go get github.com/kingfs/go-llm-specs
```

## 🛠 使用示例

### 1. 基础获取 (Get)

支持通过 ID 或别名获取模型信息：

```go
package main

import (
    "fmt"
    "github.com/kingfs/go-llm-specs"
)

func main() {
    // 通过别名获取模型
    if m, ok := llmspecs.Get("gpt4t"); ok {
        fmt.Printf("Model: %s\n", m.Name())
        fmt.Printf("Context Length: %d\n", m.ContextLength())
        fmt.Printf("Input Price: $%f / 1k tokens\n", m.PriceInput())
    }
}
```

### 2. 链式查询 (Query)

强大的位掩码过滤，极速筛选符合要求的模型：

```go
package main

import (
    "fmt"
    "github.com/kingfs/go-llm-specs"
)

func main() {
    // 筛选 Anthropic 旗下支持图片输入和函数调用的模型
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

更多示例请参考 [examples](file:///Users/kingfs/go/src/github.com/kingfs/go-llm-specs/examples) 目录。

## 🤖 工作原理

1.  **Generator (cmd/generator)**: 每天自动从 OpenRouter 抓取全量模型数据。
2.  **Overrides (data/overrides.yaml)**: 允许人工修正别名、补全中文描述、纠正 Provider 名称。
3.  **Code Gen**: 自动生成 `models_gen.go`，将所有数据硬编码为静态 Map。
4.  **Auto Update**: 通过 GitHub Actions 每天更新并自动发布 SemVer 版本。

## 📝 手动运行生成器

如果你想使用最新的本地数据，可以手动运行：

```bash
# 需要有网络访问权限
go run cmd/generator/main.go
```

## 📄 开源协议

Apache 2.0 License

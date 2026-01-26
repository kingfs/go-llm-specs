# go-llm-specs

构建 Golang 生态中最全、最快、类型安全的 LLM 静态元数据中心。

[English](./README_EN.md) | [中文](./README.md)

[![Daily Model Sync](https://github.com/kingfs/go-llm-specs/actions/workflows/daily-update.yml/badge.svg)](https://github.com/kingfs/go-llm-specs/actions/workflows/daily-update.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/kingfs/go-llm-specs.svg)](https://pkg.go.dev/github.com/kingfs/go-llm-specs)

## 🌟 项目愿景

*   **Single Source of Truth**: 以 OpenRouter 为主数据源，结合本地 `models/` 目录下的修正与补充。
*   **Zero Runtime IO**: 所有数据编译进二进制，查询零网络延迟。
*   **High Performance**: 利用 Bitmask（位掩码）处理模型能力，纳秒级查询（包括新增的模型类型如 Embedding/Reranker）。
*   **Self-Updating**: 利用 GitHub Actions 实现“无人值守”的自动更新与版本发布。

## 🚀 性能基准

在 Apple M3 Pro 上测试，所有操作均为纳秒级且几乎零内存分配：

| 操作 | 性能 | 内存分配 |
| :--- | :--- | :--- |
| `Get(ID)` (精确查找) | **~6 ns/op** | 0 B/op |
| `Get(Alias)` (别名查找) | **~24 ns/op** | 0 B/op |
| `GetMany([]string)` (批量) | **~156 ns/op** | 80 B/op (1 alloc) |
| `Search(query, limit)` (模糊搜索) | **~35 µs/op** | ~11 KB/op |
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
    }
}
```

### 2. 批量获取 (GetMany)

高效取回多个模型，自动跳过不存在的模型：

```go
names := []string{"gpt4t", "qwen3-32b", "non-existent"}
models := llmspecs.GetMany(names)
for _, m := range models {
    fmt.Printf("- Found: %s\n", m.Name())
}
```

### 3. 链式查询 (Query)

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

### 3. 模糊搜索 (Search)

当你不确定模型全名时，可以使用搜索功能获取按相关度排序的结果。搜索逻辑支持对 ID、名称和别名进行加权匹配：

1.  **精确匹配** (ID: 100分, 名称: 90分)
2.  **别名精确匹配** (80分)
3.  **前缀匹配** (ID: 50分, 名称: 40分)
4.  **子串匹配** (ID: 20分, 名称: 10分)
5.  **别名子串匹配** (15分)

```go
// 搜索包含 "claude" 的模型
results := llmspecs.Search("claude", 5)
for _, m := range results {
    fmt.Printf("Found: %s (%s)\n", m.Name(), m.ID())
}
```

### 4. 别名机制 (Aliases)

为了简化查找，项目通过以下方式生成别名：
- **手动修正**: 在 `models/` 目录下人工定义的别名（具有最高优先级）。
- **自动生成**: 如果模型 ID 的后缀（如 `qwen/qwen3-32b` 中的 `qwen3-32b`）在全量模型中是唯一的，生成器会自动将其设为别名。

```go
// 使用自动生成的唯一后缀别名查找
m, ok := llmspecs.Get("qwen3-32b")
```

更多示例请参考 [examples](examples) 目录。

## 📂 自定义注册表与覆盖

项目支持通过根目录下的 `models/` 文件夹添加新模型或覆盖现有模型信息。生成器会递归扫描该目录下的所有 `.yaml` 文件。

### 1. 目录结构
建议按供应商组织文件：
```
models/
├── openai/
│   ├── gpt-4o.yaml
│   └── text-embedding-3.yaml
├── anthropic/
│   └── claude-3-opus.yaml
└── custom-provider.yaml
```

### 2. 添加/覆盖规则
- **添加新模型**: 创建 YAML 文件并指定唯一的 `id`（例如：`my-provider/my-model`）。
- **覆盖现有模型**: 使用与 OpenRouter 相同的 `id`，YAML 中的字段将覆盖 API 返回的数据。

### 3. YAML 格式示例
```yaml
id: openai/text-embedding-3-large
name: "OpenAI: Text Embedding 3 Large"
provider: OpenAI
description_cn: "OpenAI 最强大的嵌入模型。"
features:
  - CapEmbedding    # 新增的模型类型支持
  - ModalityTextIn
context_length: 8192
price_in: 0.00000013
aliases:
  - text-embedding-3-large
```

支持的 Feature 见 `capability.go`。

## 🤖 工作原理

1.  **Generator (cmd/generator)**: 每天自动从 OpenRouter 抓取数据，并递归加载 `models/` 目录下的所有本地定义，最后进行合并。
2.  **Translator (cmd/translator)**: 批量调用 LLM 将 `models/` 中缺失中文描述的模型进行翻译补偿（可选）。
3.  **Local Registry (models/)**: 存放人工修正、别名、中文描述以及 API 缺失的模型（如 Embedding/Reranker）。
4.  **Code Gen**: 自动生成 `models_gen.go`，将所有数据硬编码为静态 Map。
5.  **Auto Update**: 通过 GitHub Actions 每天更新并自动发布 SemVer 版本。

## 📝 手动运行工具

### 生成器 (Generator)
```bash
go run cmd/generator/main.go
```

### 翻译器 (Translator)
需要设置 `LLM_API_KEY` (OpenAI 格式):
```bash
export LLM_API_KEY="sk-..."
export LLM_MODEL="gpt-4o-mini" # 可选，默认值
go run cmd/translator/main.go
```

## 📄 开源协议

Apache 2.0 License

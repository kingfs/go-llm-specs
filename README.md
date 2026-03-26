# go-llm-specs

构建 Golang 生态中面向 LLM 的静态模型元数据注册表。

[English](./README_EN.md) | [中文](./README.md)

[![Daily Model Sync](https://github.com/kingfs/go-llm-specs/actions/workflows/daily-update.yml/badge.svg)](https://github.com/kingfs/go-llm-specs/actions/workflows/daily-update.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/kingfs/go-llm-specs.svg)](https://pkg.go.dev/github.com/kingfs/go-llm-specs)

## 项目定位

`go-llm-specs` 维护一份静态、类型安全、可直接编译进 Go 二进制的 LLM 模型注册表。

- 上游主数据源当前为 OpenRouter。
- 本地 `models/**/*.yaml` 负责人工修正、补充、别名和中文描述。
- 生成结果写入 `models_gen.go`，运行时不需要网络请求。
- `cmd/translator` 负责对 `models/` 下缺失 `description_cn` 的文件做增量翻译。

AI 协作说明见 [AGENTS.md](./AGENTS.md)，开发流程说明见 [docs/DEVELOPMENT.md](./docs/DEVELOPMENT.md)。

## 核心能力

- `Get` / `GetMany`：按 ID 或别名快速查找模型。
- `Query`：基于位掩码能力的链式过滤。
- `Search`：按 ID、名称、别名做模糊搜索。
- `models/` 覆盖机制：本地 YAML 显式填写的字段优先于上游数据。
- 代码生成：将最终注册表固化到 `models_gen.go`，方便其他 Go 项目直接依赖。

## 安装

```bash
go get github.com/kingfs/go-llm-specs
```

## 快速使用

### 基础获取

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

### 链式查询

```go
models := llmspecs.Query().
	Provider("Anthropic").
	Has(llmspecs.ModalityImageIn).
	Has(llmspecs.CapFunctionCall).
	List()
```

### 模糊搜索

```go
results := llmspecs.Search("claude", 5)
```

更多示例见 [examples/basic/main.go](./examples/basic/main.go)。

## 数据目录

```text
.
├── cmd/
│   ├── generator/      # 从上游抓取并生成静态注册表
│   └── translator/     # 对 models/ 做增量翻译
├── data/
│   └── models.json     # 上游原始缓存
├── docs/
├── models/             # 人工维护的 YAML 模型定义
├── models_gen.go       # 生成文件，不要手改
└── Taskfile.yml        # go-task 统一入口
```

## 开发命令

项目使用 `go-task` 作为统一入口。

```bash
task fmt
task lint
task test
task build
task generator
task translator
task sync
```

传递额外参数时使用 `--`：

```bash
task generator -- -fetch-only
task translator -- -provider OpenAI -limit 20
task run -- go test ./...
```

## Generator

`cmd/generator` 的职责是：

1. 从上游拉取模型列表及描述。
2. 合并 `models/` 中的本地覆盖。
3. 更新 `models/**/*.yaml`。
4. 生成 `models_gen.go` 供其他项目直接使用。

常用命令：

```bash
task generator
task generator -- -fetch-only
task generator -- -sync-registry=false
```

常用参数：

- `-source`：上游来源，当前支持 `openrouter`
- `-api-url`：上游接口地址
- `-models-dir`：本地 YAML 目录
- `-cache-path`：原始上游 JSON 缓存路径
- `-output-go`：生成的 Go 文件路径
- `-fetch-only`：只抓取上游并刷新缓存，不写 `models/` 和 `models_gen.go`

## Translator

`cmd/translator` 默认只翻译满足以下条件的 YAML：

- 有 `description`
- 缺少 `description_cn`

常用命令：

```bash
export LLM_API_KEY="sk-..."
task translator
task translator -- -provider OpenAI -limit 50
task translator -- -dry-run -id-prefix qwen/
```

环境变量：

- `LLM_API_KEY`：必填
- `LLM_BASE_URL`：可选，默认 `https://api.openai.com/v1`
- `LLM_MODEL`：可选，默认 `gpt-4o-mini`

## 本地模型覆盖示例

```yaml
id: openai/text-embedding-3-large
name: "OpenAI: Text Embedding 3 Large"
provider: OpenAI
description_cn: "OpenAI 最强大的嵌入模型。"
features:
  - CapEmbedding
  - ModalityTextIn
context_length: 8192
aliases:
  - text-embedding-3-large
```

支持的 capability 常量见 [capability.go](./capability.go)。

## 工作流

典型维护流程：

1. 运行 `task generator` 拉取上游并刷新本地注册表。
2. 如需中文描述，运行 `task translator`。
3. 再次运行 `task generator`，把翻译后的 `description_cn` 编译进 `models_gen.go`。
4. 运行 `task ci` 验证格式、静态检查、测试和构建。

## 许可证

Apache 2.0 License

# go-llm-specs

面向 Go 应用的 LLM 模型元数据注册表：把模型 ID、供应商、上下文长度、输入输出模态、工具调用、JSON mode、别名、标签和中英文描述编译进你的程序。

[English](./README_EN.md) | [中文](./README.md)

[![Daily Model Sync](https://github.com/kingfs/go-llm-specs/actions/workflows/daily-update.yml/badge.svg)](https://github.com/kingfs/go-llm-specs/actions/workflows/daily-update.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/kingfs/go-llm-specs.svg)](https://pkg.go.dev/github.com/kingfs/go-llm-specs)

## 为什么需要它

如果你的产品里需要接入多个 LLM 供应商，通常很快会遇到这些重复工作：

- 用户输入 `gpt4t`、`claude sonnet`、`qwen3-32b` 时，你需要把它们解析成稳定的模型 ID。
- 模型选择器、管理后台、计费配置、路由策略需要展示模型名称、供应商、上下文窗口和能力标签。
- 调用模型前需要判断它是否支持图片输入、函数调用、结构化输出、Embedding、Rerank、TTS 或 ASR。
- 你不希望每次启动服务都请求外部接口，也不希望在业务代码里维护一堆易过期的模型常量。

`go-llm-specs` 把这些信息整理成一个静态、类型安全、可直接依赖的 Go 包。运行时只做内存查询，不访问网络，适合放进 API 服务、Agent 平台、模型网关、控制台、CLI 工具和内部运维系统。

## 你能得到什么

- 静态注册表：当前包含 800+ 个模型，数据随项目自动同步并生成到 `models_gen.go`。
- 统一模型卡片：快速拿到 ID、名称、供应商、系列、摘要、标签、上下文长度、最大输出和能力位。
- 别名解析：按模型 ID 或别名查询，别名大小写不敏感。
- 能力过滤：用 Go 常量筛选 Vision、Tool Use、JSON mode、Embedding、Rerank 等模型能力。
- 模糊搜索：在模型 ID、名称、系列、标签和别名中搜索，适合构建模型选择器。
- 零运行时依赖：注册表编译进二进制，服务启动和查询都不依赖外部模型列表接口。
- 中英文描述：适合直接在中文或英文产品界面里渲染模型说明。

## 安装

```bash
go get github.com/kingfs/go-llm-specs
```

## 快速开始

```go
package main

import (
	"fmt"

	llmspecs "github.com/kingfs/go-llm-specs"
)

func main() {
	model, ok := llmspecs.Get("gpt4t")
	if !ok {
		return
	}

	fmt.Println(model.ID())              // openai/gpt-4-turbo
	fmt.Println(model.Provider())        // OpenAI
	fmt.Println(model.ContextLength())   // 上下文窗口
	fmt.Println(model.Features().String()) // TextIn|TextOut|...
}
```

更多可运行示例见 [examples/basic/main.go](./examples/basic/main.go)。

## 常见用法

### 构建模型选择器

```go
for _, model := range llmspecs.Search("claude sonnet", 10) {
	card := model.Card()
	fmt.Printf("%s: %s [%s]\n", card.Provider, card.Name, card.ID)
}
```

### 筛选支持图片和工具调用的模型

```go
models := llmspecs.Query().
	Has(llmspecs.ModalityImageIn).
	Has(llmspecs.CapFunctionCall).
	List()
```

### 只看某个供应商的模型

```go
anthropicVisionModels := llmspecs.Query().
	Provider("Anthropic").
	Has(llmspecs.ModalityImageIn).
	List()
```

### 校验业务配置里的模型名

```go
configured := []string{"gpt4t", "qwen3-32b", "not-exist"}
validModels := llmspecs.GetMany(configured)
```

### 按标签组织模型

```go
reasoningModels := llmspecs.Query().
	Tag(string(llmspecs.TagReasoning)).
	List()

for _, tag := range llmspecs.KnownTags() {
	fmt.Println(tag.Category, tag.Name, tag.Label)
}
```

## API 概览

| API | 说明 |
| --- | --- |
| `Total()` | 返回注册表模型数量 |
| `Get(idOrAlias)` | 通过模型 ID 或别名获取模型 |
| `GetMany(idsOrAliases)` | 批量获取模型，未命中的条目会被跳过 |
| `Search(query, limit)` | 在 ID、名称、系列、摘要、标签和别名中模糊搜索 |
| `Query()` | 创建链式查询器 |
| `KnownTags()` | 返回稳定标签目录，便于下游渲染和分组 |
| `Model.Card()` | 返回适合 UI 展示的轻量结构体 |

`Model` 暴露的核心字段包括：

- `ID()`、`Name()`、`Provider()`、`Family()`、`Series()`
- `Description()`、`DescriptionCN()`、`Summary()`
- `ContextLength()`、`MaxOutput()`
- `Features()`、`HasCapability()`、`Tags()`、`HasTag()`、`Aliases()`

能力常量定义在 [capability.go](./capability.go)，标签常量定义在 [tag.go](./tag.go)。

## 适合放在哪里

- 模型网关：根据用户选择、能力要求或供应商策略路由模型。
- Agent 平台：只展示支持工具调用、结构化输出或多模态输入的模型。
- SaaS 控制台：渲染模型列表、标签、上下文窗口和本地化说明。
- CLI / SDK：验证配置文件中的模型名，给出搜索和补全结果。
- 内部平台：用统一模型元数据替代散落在业务代码里的硬编码表。

## 数据来源与更新

项目当前主要从 OpenRouter 同步模型元数据，并通过 `models/**/*.yaml` 维护人工修正、补充字段、别名和中文描述。最终数据会生成到 `models_gen.go`，下游项目只需要正常引入 Go 包即可，不需要在运行时执行同步任务。

维护相关文件：

```text
.
├── cmd/
│   ├── generator/      # 同步上游数据并生成静态注册表
│   └── translator/     # 增量补充中文描述
├── data/
│   └── models.json     # 上游原始缓存
├── models/             # 人工维护的 YAML 模型定义
├── models_gen.go       # 生成文件，不要手改
└── Taskfile.yml        # go-task 统一入口
```

## 参与贡献

如果你发现模型信息缺失、别名不方便、能力标签不准确，欢迎提交 PR。常见维护流程：

```bash
task generator
task test
```

常用命令：

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

本地覆盖模型信息时，请修改 `models/**/*.yaml`，不要手改 `models_gen.go`。更完整的维护说明见 [docs/DEVELOPMENT.md](./docs/DEVELOPMENT.md)，AI 协作说明见 [AGENTS.md](./AGENTS.md)。

## 许可证

Apache 2.0 License

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

- 静态注册表：包含数百个模型，数据随项目自动同步并生成到 `models_gen.go`。
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

项目以模型厂商公开宣称的模型参数为事实范围，不记录具体部署实例的限制。OpenRouter 继续作为高覆盖率的主要发现入口，厂商官方页面和已订阅的官方 Hugging Face 组织用于补全和发现。`models/**/*.yaml` 是最高优先级、持续累计的人工事实目录：自动任务只填充空字段，任何已有值都不会因 provenance 或上游变化而被覆盖，也不会因上游下架而删除。

维护相关文件：

```text
.
├── cmd/
│   ├── generator/      # 同步上游数据并生成静态注册表
│   ├── translator/     # 增量补充中文描述
│   ├── enricher/       # 从结构化上游补充丰富信息
│   ├── catalogsync/    # 审计厂商归属并发现官方 HF 新增候选
│   ├── codexgen/       # 生成 Codex models.json
│   └── suggestionctl/ # 审核并应用有证据的 AI 建议
├── data/
│   └── models.json     # 上游原始缓存
├── models/             # 人工维护的 YAML 模型定义
├── providers/          # 厂商官方入口与可订阅组织目录
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
task cardextract -- -model qwen/qwen3.6-27b -ai-model <local-model>
task suggestion -- list
task codexsuggest -- -model qwen/qwen3.6-27b
task codexsuggest -- -allowlist codex-models.yaml -report .cache/codex-selection.json
task codexsuggest -- -since 180d -serving-provider openrouter -report .cache/codex-recent.json
task enrich
task catalog-audit
task catalog-discover
task catalog-promote
task codexgen
task releasecheck
task sync
```

每日 Actions 只请求一次 OpenRouter，并完整分页检查订阅的官方 Hugging Face 组织。HF 新发现先作为不参与 Go 注册表生成的候选 YAML；结构化补全和官方模型卡高置信字段齐备后才自动晋升。相同输入生成完全相同的代码，只有实际模型事实变化才提交并按 releasecheck 发布版本。

本地覆盖模型信息时，请修改 `models/**/*.yaml`，不要手改 `models_gen.go`。更完整的维护说明见 [docs/DEVELOPMENT.md](./docs/DEVELOPMENT.md)，AI 协作说明见 [AGENTS.md](./AGENTS.md)。

新发现或显式选择的模型可以使用 schema v2 保存来源可追溯的 OpenRouter、Hugging Face、官方链接与身份映射。完整事实边界和增量流程见 [模型目录架构](./docs/MODEL_CATALOG.md)；Codex 导出细节见 [Codex metadata pipeline](./docs/CODEX_METADATA_PIPELINE.md)。

## Codex 第三方模型目录

`third-party-models.json` 可供 Codex 使用；当配置的模型名与目录中的 `slug` 一致时，Codex 不再回退到 fallback metadata，因而可消除对应的 metadata warning。`model_catalog_json` 会替换而非追加 Codex 内置目录，因此推荐使用安装脚本：它会下载最新 release、导出本机 Codex 的内置模型、按本机 schema 合并并验证目录，然后备份和更新 `~/.codex/config.toml`。

```bash
curl -fsSL https://raw.githubusercontent.com/kingfs/go-llm-specs/master/scripts/install-codex-catalog.sh | sh
```

在仓库内也可以运行：

```bash
task codexinstall
```

默认输出为 `~/.codex/models.json`。可以用 `--config` 和 `--output` 指定其他位置。脚本只修改顶层 `model_catalog_json`；更新已有配置前会创建 `config.toml.bak`。若本机 Codex schema 与 release 产物仍不兼容，脚本会在改动配置前停止，并输出 Codex 的实际解析错误。

需要手工合并时，应始终使用同一台机器、同一 Codex 版本导出的内置目录：

```bash
codex debug models --bundled > bundled-models.json
task codexgen -- -bundled-catalog bundled-models.json -output merged-models.json
```

指定一批实际部署的模型时，创建清单（slug 必须与 API 返回/接受的模型名一致）：

```yaml
models:
  - id: qwen/qwen3.6-27b
    slugs: [qwen3.6-27b]
  - id: deepseek/deepseek-v3.2
    slugs: [deepseek-v3.2]
```

```bash
task codexsuggest -- -allowlist codex-models.yaml -report .cache/codex-selection.json
task suggestion -- list
task suggestion -- -fields codex.enabled,codex.slugs,codex.shell_type,codex.apply_patch_tool_type,codex.supports_parallel_tool_calls,codex.input_modalities apply data/suggestions/<provider>/<model>.codex.json
task codexgen
task codexcheck
```

OpenRouter 用户也可以按真实发布时间选择最近半年候选：

```bash
task codexsuggest -- -since 180d -serving-provider openrouter \
  -report .cache/codex-recent.json
```

这里的“最近”只负责筛选候选；非 schema v2、非 chat/tool、缺少文本输入输出或上下文信息的记录会写入 skipped 报告。对于 vLLM/SGLang 或其他供应商，不应假定 OpenRouter ID 就是 serving slug，请先把候选整理成上述显式清单。审核并 apply 后，下一次 `task codexgen` 会把所有合格且已启用的模型打包到同一个目录。不能直接把注册表中的全部模型导出，因为其中还包括 embedding、rerank、音频模型以及未确认服务名/工具策略的记录。

Release catalog 还通过 [`data/codex/default-open-models.yaml`](./data/codex/default-open-models.yaml) 默认收录以下开放权重家族：Qwen 3.5 及以后、DeepSeek V3/R1 及以后、GLM-5 及以后、Kimi K2.7 及以后。除静态能力检查外，型号必须具有已登记厂商、属于该厂商配置组织的 Hugging Face 身份、精确模型卡链接和固定 revision；普通历史收录记录不会进入 Codex catalog。API 专有型号、路由别名及非 agent 模型也不会仅凭名称被收录。

## 许可证

Apache 2.0 License

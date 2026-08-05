# 在 Codex 中使用 models.json

`go-llm-specs` 的 Release 提供经过校验的第三方模型目录。Codex 的 `model_catalog_json` 会替换内置模型目录，因此不能直接把第三方文件写进配置；需要先与当前 Codex 版本自带的目录合并。

## 推荐安装方式

在 macOS 或 Linux 上运行：

```bash
curl -fsSL https://raw.githubusercontent.com/kingfs/go-llm-specs/master/scripts/install-codex-catalog.sh | sh
```

在本仓库中也可以运行：

```bash
task codexinstall
```

安装脚本会：

1. 从本机 Codex 导出与当前版本兼容的内置模型目录。
2. 下载最新 Release 中的 `third-party-models.json`。
3. 合并内置模型和第三方模型，并检查重复 slug。
4. 使用本机 Codex CLI 验证合并结果。
5. 写入 `~/.codex/models.json`，并更新 `~/.codex/config.toml` 中的 `model_catalog_json`。

修改已有配置前，脚本会备份为 `~/.codex/config.toml.bak`。它只修改顶层 `model_catalog_json`，不会改动 provider、profile 或其他 Codex 设置。

## 指定其他路径

```bash
./scripts/install-codex-catalog.sh \
  --config /path/to/config.toml \
  --output /path/to/models.json
```

也可以使用 `CODEX_BIN`、`CODEX_HOME`、`CODEX_CONFIG_FILE` 和 `CODEX_CATALOG_FILE` 环境变量适配自定义安装。

## 验证安装

```bash
codex debug models
```

输出中应同时包含 Codex 内置模型和已启用的第三方模型。若 API 实际接受的模型名与目录中的 `slug` 不一致，仍需在模型服务或本仓库元数据中登记正确的 serving slug。

## 更新和恢复

重新运行安装命令即可下载最新 Release 并重建目录。需要恢复原配置时：

```bash
cp ~/.codex/config.toml.bak ~/.codex/config.toml
```

删除 `model_catalog_json` 配置后，Codex 会重新使用自身的内置模型目录。

## 手工合并

如果不使用安装脚本，应始终从同一台机器、同一个 Codex 版本导出内置目录：

```bash
codex debug models --bundled > bundled-models.json
task codexgen -- \
  -bundled-catalog bundled-models.json \
  -output merged-models.json
```

然后在 `~/.codex/config.toml` 顶层配置：

```toml
model_catalog_json = "/absolute/path/to/merged-models.json"
```

不要只配置 Release 中的 `third-party-models.json`，否则 Codex 内置模型会从目录中消失。

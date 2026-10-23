# Kami

完整产品名为 **Kami · 墨**，项目与应用使用 **Kami**。Kami 意为纸，与「墨」共同表达纸与文字的意象。仓库：[Umupa/Kami](https://github.com/Umupa/Kami)。

我自己的桌面 AI 工具箱：一个常驻菜单栏的 macOS 原生应用。⌥空格打开面板，双击 ⌥ 带着选中的文字打开，⌥S 截图识别文字；用自己配置的大模型完成翻译、润色（杂乱时顺带整理）、提示词梳理和快速问答，结果以 Markdown 流式呈现，可以继续追问。

一期提供翻译、润色、提示词梳理和多轮问答，二期加入笔记。用 Go 和 [MyGo](https://github.com/egoist/mygo) 编写；设置全在一个带 schema 的配置文件里，交给 agent 配置。

## 状态

一期功能已实现，等待统一实测：面板与多轮对话、四个内置模式、三种模型格式、带入选区、截图识别、Operator（在面板里输入 `/operator` 查看和修改设置）、配置接口（`config.json` + `check`）。

## 构建与开发

| 命令 | 作用 |
| --- | --- |
| `scripts/build.sh` | 构建 `build/darwin-arm64/Kami.app`，用本机签名身份 `Kami Dev` 签名；第一次运行时自动创建该身份（需要输入一次系统密码） |
| `scripts/dev.sh` | 带热重载的开发运行，同样签名。它是另一个应用 `Kami Dev`，权限与数据目录和正式构建分开；没有模型时显示首次使用引导，`KAMI_DEMO=1 scripts/dev.sh` 改用演示回复 |
| `scripts/reset-permissions.sh [--dev]` | 清除 Kami 的辅助功能、输入监控、屏幕录制记录，用于授权错乱时 |

签名固定后，重新构建不会丢失系统权限。`scripts/build.sh --ad-hoc` 只用于临时产物，它的权限在重新构建后失效。

**权限总是要重新授予？** 系统设置里的开关可能属于之前的某次构建，所以显示开启却不生效。运行 `scripts/reset-permissions.sh`，或在系统设置里用「−」移除 Kami 后重新添加。输入监控和屏幕录制授权后，从菜单栏选「重新启动 Kami」才会生效。

**API Key 在哪里？** 在 `~/Library/Application Support/Kami/config.json` 里每个模型的 `apiKey` 字段，文件权限 0600，不进钥匙串（见 [D12](docs/decisions.md)）。

## 文档

| 文档 | 内容 |
| --- | --- |
| [项目目标与最终效果](docs/vision.md) | 为什么做、做成什么样、一天里怎么用它 |
| [需求](docs/requirements.md) | 场景、功能需求与优先级、非功能指标、不做的事、待定问题 |
| [技术栈](docs/tech-stack.md) | 每一层用什么、版本、为什么、风险与退路 |
| [架构与核心原理](docs/architecture.md) | 模块划分、数据流、模式、提示词合约、入口流程、配置接口 |
| [路线图](docs/roadmap.md) | 里程碑、每个阶段的交付与验收、日常使用验收 |
| [决策记录](docs/decisions.md) | 已经做出的关键决定及其理由 |

参与开发前先读 [AGENTS.md](AGENTS.md)。

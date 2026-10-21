# Gloss

项目与应用统一命名为 **Gloss**，仓库：[Umupa/Gloss](https://github.com/Umupa/Gloss)。

我自己的桌面 AI 工具箱：一个常驻菜单栏的 macOS 原生应用。⌥空格打开面板，双击 ⌥ 带着选中的文字打开，⌥S 截图识别文字；用自己配置的大模型完成翻译、润色（杂乱时顺带整理）、提示词梳理和快速问答，结果以 Markdown 流式呈现，可以继续追问。

一期提供翻译、润色、提示词梳理和多轮问答，二期加入笔记。用 Go 和 [MyGo](https://github.com/egoist/mygo) 编写；设置全在一个带 schema 的配置文件里，交给 agent 配置。

## 状态

一期功能已实现，等待统一实测：面板与多轮对话、四个内置模式、三种模型格式、带入选区、截图识别、Operator（在面板里输入 `/operator` 查看和修改设置）、配置接口（`config.json` + `check`）。

构建：先运行一次 `scripts/dev-cert.sh` 创建本机签名身份，之后 `scripts/build.sh`，产物在 `build/darwin-arm64/Gloss.app`。没有固定签名时构建直接失败，避免重新构建后陷入系统权限反复申请；仅需临时产物时可显式运行 `scripts/build.sh --ad-hoc`，该产物的权限不保证在重建后保留。

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

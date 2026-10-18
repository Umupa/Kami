# AGENTS.md

所有参与开发 cida-next 的人与 agent 的工作规则。项目目标见 [docs/vision.md](docs/vision.md)，需求见 [docs/requirements.md](docs/requirements.md)。

## 目录

| 路径 | 内容 |
| --- | --- |
| `docs/` | 目标、需求、技术栈、架构、路线图、决策记录 |
| `internal/` | 应用代码，模块划分见 [docs/architecture.md](docs/architecture.md) §一（M0 建立） |
| `main.go` | 入口：带子命令（`key`、`check`）时是命令行，否则启动应用（M0 建立） |

## 文档先行

- `docs/requirements.md` 是需求的唯一来源，`docs/architecture.md` 是结构的唯一来源。改行为或结构的变更，在同一个提交里更新对应文档。
- 新的关键决定写进 `docs/decisions.md`；推翻旧决定时改原条目，不保留过时版本。
- 引用辞达（`Xuanwo/cida`）的规范时写明文件与章节，例如「同辞达 panel.md §一」。辞达是 Apache-2.0：可以参考思路，移植代码时保留它的版权与许可声明。

## 语言

- 代码、注释、标识符、提交信息用英文。
- 文档用简体中文；界面文字用简体中文。

## 代码

- 业务核心（`session`、`mode`、`llm`、`config`）不依赖 UI 和系统框架，必须能无窗口测试。
- purego 调用只出现在 `internal/macos`，其他包通过它暴露的 Go 接口使用系统能力。
- 每个系统调用都有超时；任何失败都不能导致崩溃。
- 注释解释代码表达不了的约束，不写历史。

## 验证

- 每次改动都要通过：`gofmt -l .` 无输出、`go vet ./...`、`go tool mygo vet .`、`go test ./...`。
- 改了面板、Markdown 渲染或主题时，更新视觉快照并检查 PNG。
- 系统集成测试用 `//go:build integration`，只做只读调用。

## 不要碰开发者的屏幕

开发者在这台 Mac 上工作，测试和调试时：

- 不发送合成的键盘、鼠标事件；模拟 ⌘C 只能由用户按下快捷键触发；双击 ⌥ 的事件监听只读，不拦截、不改写事件。
- 不截取实时屏幕，不录屏。
- 不写系统剪贴板；剪贴板相关测试用 `NSPasteboard pasteboardWithUniqueName` 创建的私有剪贴板。
- 不激活、不前置其他应用的窗口。
- 开发期全局快捷键用 ⌃⌥ 组合，不与正在运行的辞达冲突。
- 不改开发者真实的 `config.json` 和钥匙串条目；测试用临时目录和独立的钥匙串服务名。

## 提交

- 提交信息以类型开头：`feat:`、`fix:`、`docs:`、`test:`、`refactor:`、`build:`、`ci:`，正文说明为什么要改。
- 不提交任何密钥。API Key 只在钥匙串里；签名证书与发布凭据只在本机钥匙串或 CI secrets 里。
